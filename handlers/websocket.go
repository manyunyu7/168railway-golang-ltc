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
	// Generate initial data from database (no S3 trains-list.json dependency)
	trainsListData := h.generateInitialDataFromDatabase()

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

	// Get active sessions directly from database (single source of truth)
	var sessions []models.LiveTrackingSession
	result := h.db.Where("status = ?", "active").Find(&sessions)
	if result.Error != nil {
		log.Printf("WebSocket: Failed to get active sessions: %v", result.Error)
		return
	}

	if len(sessions) == 0 {
		// No active sessions - send empty update
		message := WebSocketMessage{
			Type: "train_updates",
			Data: []TrainUpdate{},
		}
		h.broadcastToClients(message)
		return
	}

	// Group sessions by train number
	trainSessions := make(map[string][]models.LiveTrackingSession)
	for _, session := range sessions {
		trainSessions[session.TrainNumber] = append(trainSessions[session.TrainNumber], session)
	}

	// Prepare train updates from database sessions
	var updates []TrainUpdate
	for trainNumber, trainSessionList := range trainSessions {
		// Get detailed train data from S3
		fileName := fmt.Sprintf("trains/train-%s.json", trainNumber)
		trainData, err := h.s3.GetTrainData(fileName)
		if err != nil {
			// If S3 file doesn't exist but we have active sessions, create basic update from database
			log.Printf("WebSocket: S3 file missing for train %s, creating from database", trainNumber)
			update := h.createUpdateFromDatabaseSessions(trainNumber, trainSessionList)
			if update != nil {
				updates = append(updates, *update)
			}
			continue
		}

		// Filter passengers to only include those with active database sessions
		var activePassengers []models.Passenger
		for _, session := range trainSessionList {
			// Find passenger data for this session
			for _, passenger := range trainData.Passengers {
				if passenger.UserID == session.UserID {
					// Update passenger status from session heartbeat
					timeSinceHeartbeat := time.Now().Sub(session.LastHeartbeat)
					if timeSinceHeartbeat <= 2*time.Minute { // 2 minutes tolerance
						passenger.Status = "active"
						activePassengers = append(activePassengers, passenger)
					}
					break
				}
			}
		}

		// Skip trains with no active passengers
		if len(activePassengers) == 0 {
			continue
		}

		// Calculate average position from active passengers
		var totalLat, totalLng float64
		for _, passenger := range activePassengers {
			totalLat += passenger.Lat
			totalLng += passenger.Lng
		}
		avgPosition := models.Position{
			Lat: totalLat / float64(len(activePassengers)),
			Lng: totalLng / float64(len(activePassengers)),
		}

		update := TrainUpdate{
			TrainNumber:     trainNumber,
			PassengerCount:  len(activePassengers),
			AveragePosition: avgPosition,
			Passengers:      activePassengers,
			LastUpdate:      time.Now().Format(time.RFC3339),
			Status:          "active",
			Route:           trainData.Route,
			DataSource:      "database-driven-websocket",
		}

		updates = append(updates, update)
	}

	// Broadcast to all clients
	message := WebSocketMessage{
		Type: "train_updates",
		Data: updates,
	}
	h.broadcastToClients(message)

	if len(updates) > 0 {
		log.Printf("Broadcasted database-driven updates for %d trains to %d clients", len(updates), len(h.clients))
	}
}

// Helper method to broadcast messages to all clients
func (h *WebSocketHandler) broadcastToClients(message WebSocketMessage) {
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
}

// Helper method to create train update from database sessions when S3 file is missing
func (h *WebSocketHandler) createUpdateFromDatabaseSessions(trainNumber string, sessions []models.LiveTrackingSession) *TrainUpdate {
	if len(sessions) == 0 {
		return nil
	}

	// Create basic passengers list from sessions (without detailed GPS from S3)
	var passengers []models.Passenger
	for _, session := range sessions {
		// Check if session is recent (within 2 minutes)
		timeSinceHeartbeat := time.Now().Sub(session.LastHeartbeat)
		if timeSinceHeartbeat <= 2*time.Minute {
			passenger := models.Passenger{
				UserID:     session.UserID,
				UserType:   "authenticated",
				ClientType: "mobile", 
				SessionID:  session.SessionID,
				Status:     "active",
				Timestamp:  session.LastHeartbeat.UnixMilli(),
				// Note: No GPS coordinates available without S3 file
			}
			passengers = append(passengers, passenger)
		}
	}

	if len(passengers) == 0 {
		return nil
	}

	return &TrainUpdate{
		TrainNumber:    trainNumber,
		PassengerCount: len(passengers),
		Passengers:     passengers,
		LastUpdate:     time.Now().Format(time.RFC3339),
		Status:         "active",
		Route:          fmt.Sprintf("Route for train %s", trainNumber),
		DataSource:     "database-only-fallback",
	}
}

// generateInitialDataFromDatabase - Generate WebSocket initial data from database
func (h *WebSocketHandler) generateInitialDataFromDatabase() map[string]interface{} {
	now := time.Now()
	var activeTrains []interface{}
	
	// Get all active sessions from database
	var sessions []models.LiveTrackingSession
	result := h.db.Where("status = ?", "active").Find(&sessions)
	if result.Error != nil {
		log.Printf("WebSocket Initial: Failed to get active sessions: %v", result.Error)
		return map[string]interface{}{
			"trains":      []interface{}{},
			"total":       0,
			"lastUpdated": now.Format(time.RFC3339),
			"source":      "database-driven-websocket-initial",
		}
	}
	
	// Group sessions by train number
	trainSessions := make(map[string][]models.LiveTrackingSession)
	for _, session := range sessions {
		// Only include recent sessions (within 5 minutes)
		if time.Since(session.LastHeartbeat) <= 5*time.Minute {
			trainSessions[session.TrainNumber] = append(trainSessions[session.TrainNumber], session)
		}
	}
	
	// Build basic trains list (same logic as broadcastTrainUpdates but simplified)
	for trainNumber, trainSessionList := range trainSessions {
		if len(trainSessionList) == 0 {
			continue
		}
		
		activeTrains = append(activeTrains, map[string]interface{}{
			"trainId":        trainNumber,
			"passengerCount": len(trainSessionList),
			"lastUpdate":     now.Format(time.RFC3339),
			"status":         "active",
		})
	}
	
	return map[string]interface{}{
		"trains":      activeTrains,
		"total":       len(activeTrains),
		"lastUpdated": now.Format(time.RFC3339),
		"source":      "database-driven-websocket-initial",
	}
}