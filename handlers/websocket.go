package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"

	"github.com/modernland/golang-live-tracking/models"
	"github.com/modernland/golang-live-tracking/utils"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Allow connections from any origin in production
		// In production, you might want to be more restrictive
		return true
	},
}

type WebSocketMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type TrainUpdate struct {
	TrainNumber     string                 `json:"trainNumber"`
	PassengerCount  int                    `json:"passengerCount"`
	AveragePosition models.Position        `json:"averagePosition"`
	Passengers      []models.Passenger     `json:"passengers"`
	LastUpdate      string                 `json:"lastUpdate"`
	Status          string                 `json:"status"`
	Route           string                 `json:"route"`
	DataSource      string                 `json:"dataSource"`
}

type WebSocketHandler struct {
	db     *gorm.DB
	s3     *utils.S3Client
	clients map[*websocket.Conn]bool
	mutex   sync.RWMutex
}

func NewWebSocketHandler(db *gorm.DB, s3Client *utils.S3Client) *WebSocketHandler {
	handler := &WebSocketHandler{
		db:      db,
		s3:      s3Client,
		clients: make(map[*websocket.Conn]bool),
	}
	
	// Start background goroutine to broadcast updates
	go handler.broadcastUpdates()
	
	return handler
}

// HandleWebSocket - WebSocket endpoint for real-time train updates
func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("WebSocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Add client to active connections
	h.mutex.Lock()
	h.clients[conn] = true
	clientCount := len(h.clients)
	h.mutex.Unlock()
	
	log.Printf("WebSocket client connected. Total clients: %d", clientCount)

	// Send initial data
	h.sendInitialData(conn)

	// Handle client messages and keep connection alive
	for {
		messageType, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("WebSocket read error: %v", err)
			break
		}
		
		// Handle ping/pong or other client messages
		if messageType == websocket.TextMessage {
			var msg WebSocketMessage
			if err := json.Unmarshal(message, &msg); err == nil {
				h.handleClientMessage(conn, &msg)
			}
		}
	}

	// Remove client on disconnect
	h.mutex.Lock()
	delete(h.clients, conn)
	clientCount = len(h.clients)
	h.mutex.Unlock()
	
	log.Printf("WebSocket client disconnected. Total clients: %d", clientCount)
}

func (h *WebSocketHandler) sendInitialData(conn *websocket.Conn) {
	// Send current active trains list
	trainsListData, err := h.s3.GetJSONData("trains/trains-list.json")
	if err != nil {
		log.Printf("Failed to get initial trains list: %v", err)
		return
	}

	message := WebSocketMessage{
		Type: "initial_data",
		Data: trainsListData,
	}

	if err := conn.WriteJSON(message); err != nil {
		log.Printf("Failed to send initial data: %v", err)
	}
}

func (h *WebSocketHandler) handleClientMessage(conn *websocket.Conn, msg *WebSocketMessage) {
	switch msg.Type {
	case "ping":
		// Respond with pong
		response := WebSocketMessage{
			Type: "pong",
			Data: map[string]interface{}{"timestamp": time.Now().Unix()},
		}
		conn.WriteJSON(response)
		
	case "subscribe_train":
		// Handle train-specific subscription
		if trainNumber, ok := msg.Data.(string); ok {
			log.Printf("Client subscribed to train: %s", trainNumber)
			// Send current train data
			h.sendTrainData(conn, trainNumber)
		}
	}
}

func (h *WebSocketHandler) sendTrainData(conn *websocket.Conn, trainNumber string) {
	fileName := fmt.Sprintf("trains/train-%s.json", trainNumber)
	trainData, err := h.s3.GetTrainData(fileName)
	if err != nil {
		return // Train not found
	}

	message := WebSocketMessage{
		Type: "train_data",
		Data: trainData,
	}

	conn.WriteJSON(message)
}

// Background goroutine to broadcast updates every 5 seconds
func (h *WebSocketHandler) broadcastUpdates() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.broadcastTrainUpdates()
		}
	}
}

func (h *WebSocketHandler) broadcastTrainUpdates() {
	h.mutex.RLock()
	if len(h.clients) == 0 {
		h.mutex.RUnlock()
		return // No clients connected
	}
	h.mutex.RUnlock()

	// Get current trains list
	trainsListData, err := h.s3.GetJSONData("trains/trains-list.json")
	if err != nil {
		return // No trains data available
	}

	trains, ok := trainsListData["trains"].([]interface{})
	if !ok {
		return
	}

	// Prepare train updates
	var updates []TrainUpdate
	for _, train := range trains {
		trainMap, ok := train.(map[string]interface{})
		if !ok {
			continue
		}

		trainId, _ := trainMap["trainId"].(string)
		passengerCount, _ := trainMap["passengerCount"].(float64)
		lastUpdate, _ := trainMap["lastUpdate"].(string)
		status, _ := trainMap["status"].(string)

		// Get detailed train data for position
		fileName := fmt.Sprintf("trains/train-%s.json", trainId)
		trainData, err := h.s3.GetTrainData(fileName)
		if err != nil {
			continue
		}

		update := TrainUpdate{
			TrainNumber:     trainId,
			PassengerCount:  int(passengerCount),
			AveragePosition: trainData.AveragePosition,
			Passengers:      trainData.Passengers,
			LastUpdate:      lastUpdate,
			Status:          status,
			Route:           trainData.Route,
			DataSource:      trainData.DataSource,
		}

		updates = append(updates, update)
	}

	// Broadcast to all clients
	message := WebSocketMessage{
		Type: "train_updates",
		Data: updates,
	}

	h.mutex.RLock()
	for conn := range h.clients {
		if err := conn.WriteJSON(message); err != nil {
			log.Printf("Failed to send update to client: %v", err)
			// Mark for removal
			h.mutex.RUnlock()
			h.mutex.Lock()
			delete(h.clients, conn)
			conn.Close()
			h.mutex.Unlock()
			h.mutex.RLock()
		}
	}
	h.mutex.RUnlock()

	if len(updates) > 0 {
		log.Printf("Broadcasted updates for %d trains to %d clients", len(updates), len(h.clients))
	}
}