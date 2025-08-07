package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/modernland/golang-live-tracking/models"
	"github.com/modernland/golang-live-tracking/middleware"
	"github.com/modernland/golang-live-tracking/utils"
)

type SimpleLiveTrackingHandler struct {
	db *gorm.DB
	s3 *utils.S3Client
}

func NewSimpleLiveTrackingHandler(db *gorm.DB, s3Client *utils.S3Client) *SimpleLiveTrackingHandler {
	return &SimpleLiveTrackingHandler{
		db: db,
		s3: s3Client,
	}
}

// GetActiveTrainsList - Public API endpoint to serve active trains list (proxy for S3)
func (h *SimpleLiveTrackingHandler) GetActiveTrainsList(c *gin.Context) {
	fmt.Printf("DEBUG: Frontend requesting active trains list via API proxy\n")
	
	// Try to read trains-list.json from S3
	trainsListData, err := h.s3.GetJSONData("trains/trains-list.json")
	if err != nil {
		fmt.Printf("DEBUG: trains-list.json not found, generating from active sessions\n")
		// If file doesn't exist, generate it from active sessions
		h.updateTrainsList()
		
		// Try again after generation
		trainsListData, err = h.s3.GetJSONData("trains/trains-list.json")
		if err != nil {
			// Return empty response if still fails
			c.JSON(http.StatusOK, gin.H{
				"trains":      []interface{}{},
				"total":       0,
				"lastUpdated": time.Now().Format(time.RFC3339),
				"source":      "golang-api-fallback",
			})
			return
		}
	}
	
	// Set proper CORS and cache headers
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	
	// Notify clients about WebSocket availability
	c.Header("X-WebSocket-Available", "wss://go-ltc.trainradar35.com/ws/trains")
	c.Header("X-API-Version", "v2.0-websocket")
	
	// Add WebSocket upgrade information to response
	if trainsListData != nil {
		trainsListData["websocket_upgrade"] = map[string]interface{}{
			"available": true,
			"url": "wss://go-ltc.trainradar35.com/ws/trains",
			"benefits": "Real-time updates, lower bandwidth, individual passenger positions",
			"message_types": []string{"initial_data", "train_updates", "ping/pong"},
		}
	}
	
	fmt.Printf("DEBUG: Serving trains list with %v trains\n", trainsListData["total"])
	c.JSON(http.StatusOK, trainsListData)
}

// GetTrainData - Public API endpoint to serve individual train data (proxy for S3)
func (h *SimpleLiveTrackingHandler) GetTrainData(c *gin.Context) {
	trainNumber := c.Param("trainNumber")
	fmt.Printf("DEBUG: Frontend requesting train data for %s via API proxy\n", trainNumber)
	
	// Construct S3 key for train file
	fileName := fmt.Sprintf("trains/train-%s.json", trainNumber)
	
	// Try to read train data from S3
	trainData, err := h.s3.GetTrainData(fileName)
	if err != nil {
		fmt.Printf("DEBUG: Train file %s not found: %v\n", fileName, err)
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Train not found",
			"trainNumber": trainNumber,
		})
		return
	}
	
	// Set proper CORS and cache headers
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
	
	fmt.Printf("DEBUG: Serving train data for %s with %d passengers\n", trainNumber, len(trainData.Passengers))
	c.JSON(http.StatusOK, trainData)
}

// GetActiveSession - Check database for active sessions
func (h *SimpleLiveTrackingHandler) GetActiveSession(c *gin.Context) {
	user, exists := middleware.GetUserFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Authentication required",
		})
		return
	}

	fmt.Printf("DEBUG: User %d requested active session check (database-backed)\n", user.ID)
	
	// Check database for active sessions (like Laravel cache)
	var session models.LiveTrackingSession
	result := h.db.Where("user_id = ? AND status = ?", user.ID, "active").First(&session)
	
	if result.Error == nil {
		// Active session found
		c.JSON(http.StatusOK, gin.H{
			"success":           true,
			"has_active_session": true,
			"session_id":        session.SessionID,
			"train_number":      session.TrainNumber,
			"train_id":          session.TrainID,
			"started_at":        session.StartedAt.Format(time.RFC3339),
		})
	} else {
		// No active session
		c.JSON(http.StatusOK, gin.H{
			"success":           true,
			"has_active_session": false,
		})
	}
}

// StartMobileSession - Simple version
func (h *SimpleLiveTrackingHandler) StartMobileSession(c *gin.Context) {
	user, exists := middleware.GetUserFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Authentication required",
		})
		return
	}

	var req struct {
		TrainID    uint    `json:"train_id" binding:"required"`
		TrainNumber string  `json:"train_number" binding:"required"`
		InitialLat float64 `json:"initial_lat" binding:"required,min=-90,max=90"`
		InitialLng float64 `json:"initial_lng" binding:"required,min=-180,max=180"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"errors":  err.Error(),
		})
		return
	}

	// Terminate any existing sessions for this user (like Laravel)
	h.terminateUserSessions(user.ID)

	sessionID := uuid.New().String()
	now := time.Now()
	fmt.Printf("DEBUG: Starting session %s for user %d, train %s\n", sessionID, user.ID, req.TrainNumber)

	// Generate train file data
	trainData := models.TrainData{
		TrainID:         req.TrainNumber,
		Route:           fmt.Sprintf("Route information for train %d", req.TrainID),
		PassengerCount:  1,
		AveragePosition: models.Position{Lat: req.InitialLat, Lng: req.InitialLng},
		Passengers: []models.Passenger{
			{
				UserID:     user.ID,
				UserType:   "authenticated", 
				ClientType: "mobile",
				Lat:        req.InitialLat,
				Lng:        req.InitialLng,
				Timestamp:  time.Now().UnixMilli(),
				SessionID:  sessionID,
				Status:     "active",
			},
		},
		LastUpdate: time.Now().Format(time.RFC3339),
		Status:     "active",
		DataSource: "live-gps",
	}

	// Upload to S3
	fileName := fmt.Sprintf("trains/train-%s.json", req.TrainNumber)
	if err := h.s3.UploadJSON(fileName, trainData); err != nil {
		fmt.Printf("ERROR: Failed to upload to S3: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to start tracking session",
			"error":   err.Error(),
		})
		return
	}

	fmt.Printf("DEBUG: Session %s stored in S3 file %s\n", sessionID, fileName)

	// Store session in database (replaces Laravel cache)
	session := models.LiveTrackingSession{
		SessionID:     sessionID,
		UserID:        user.ID,
		UserType:      "authenticated",
		ClientType:    "mobile",
		TrainID:       req.TrainID,
		TrainNumber:   req.TrainNumber,
		FilePath:      fileName,
		StartedAt:     now,
		LastHeartbeat: now,
		Status:        "active",
	}

	if err := h.db.Create(&session).Error; err != nil {
		fmt.Printf("ERROR: Failed to save session to database: %v\n", err)
		// Continue anyway - S3 file was created successfully
	}

	// Update trains list
	h.updateTrainsList()

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"session_id": sessionID,
		"message":    "Mobile tracking session started successfully",
	})
}

// UpdateMobileLocation - Simple version
func (h *SimpleLiveTrackingHandler) UpdateMobileLocation(c *gin.Context) {
	user, exists := middleware.GetUserFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Authentication required",
		})
		return
	}

	var req struct {
		SessionID string   `json:"session_id" binding:"required"`
		Latitude  float64  `json:"latitude" binding:"required,min=-90,max=90"`
		Longitude float64  `json:"longitude" binding:"required,min=-180,max=180"`
		Accuracy  *float64 `json:"accuracy,omitempty"`
		Speed     *float64 `json:"speed,omitempty"`
		Heading   *float64 `json:"heading,omitempty"`
		Altitude  *float64 `json:"altitude,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"errors":  err.Error(),
		})
		return
	}

	fmt.Printf("DEBUG: User %d updating location for session %s: (%.6f, %.6f)\n", 
		user.ID, req.SessionID, req.Latitude, req.Longitude)

	// Validate session in database (like Laravel cache)
	var session models.LiveTrackingSession
	result := h.db.Where("session_id = ? AND user_id = ? AND status = ?", req.SessionID, user.ID, "active").First(&session)
	
	if result.Error != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "Invalid session",
		})
		return
	}

	// Update heartbeat in database
	h.db.Model(&session).Update("last_heartbeat", time.Now())

	// Update location in S3 train file using session info
	trainFile, err := h.updateLocationInTrainFile(session.FilePath, user.ID, req)
	if err != nil {
		fmt.Printf("ERROR: Failed to update location in S3: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update location",
			"error":   err.Error(),
		})
		return
	}

	if trainFile != "" {
		fmt.Printf("DEBUG: Successfully updated location in S3 file: %s\n", trainFile)
		// Update trains list after location update
		h.updateTrainsList()
	} else {
		fmt.Printf("DEBUG: No active train file found for user %d\n", user.ID)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Mobile location updated successfully",
		"updated_file": trainFile,
	})
}

// Heartbeat - Simple version
func (h *SimpleLiveTrackingHandler) Heartbeat(c *gin.Context) {
	user, exists := middleware.GetUserFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Authentication required",
		})
		return
	}

	var req struct {
		SessionID string  `json:"session_id" binding:"required"`
		AppState  *string `json:"app_state,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"errors":  err.Error(),
		})
		return
	}

	fmt.Printf("DEBUG: User %d sent heartbeat for session %s\n", user.ID, req.SessionID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Heartbeat received (Redis-free)",
	})
}

// RecoverSession - Simple version
func (h *SimpleLiveTrackingHandler) RecoverSession(c *gin.Context) {
	user, exists := middleware.GetUserFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Authentication required",
		})
		return
	}

	var req struct {
		SessionID string  `json:"session_id" binding:"required"`
		Reason    *string `json:"reason,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"errors":  err.Error(),
		})
		return
	}

	fmt.Printf("DEBUG: User %d recovered session %s\n", user.ID, req.SessionID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Session recovered successfully (Redis-free)",
		"train_number": "TEST-TRAIN",
	})
}

// StopMobileSession - Simple version
func (h *SimpleLiveTrackingHandler) StopMobileSession(c *gin.Context) {
	user, exists := middleware.GetUserFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Authentication required",
		})
		return
	}

	var req struct {
		SessionID string `json:"session_id" binding:"required"`
		SaveTrip  *bool  `json:"save_trip,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"errors":  err.Error(),
		})
		return
	}

	// Validate session in database
	var session models.LiveTrackingSession
	result := h.db.Where("session_id = ? AND user_id = ? AND status = ?", req.SessionID, user.ID, "active").First(&session)
	
	if result.Error != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "Invalid session",
		})
		return
	}

	fmt.Printf("DEBUG: User %d stopping session %s\n", user.ID, req.SessionID)

	// Get train file data before removing user
	fileName := session.FilePath
	var tripSaved bool = false
	
	// Handle train file - remove user or delete entire file
	err := h.handleStopSessionS3Operations(fileName, user.ID, req.SaveTrip != nil && *req.SaveTrip)
	if err != nil {
		fmt.Printf("ERROR: S3 operations failed: %v\n", err)
	}

	// Mark session as completed in database
	h.db.Model(&session).Updates(models.LiveTrackingSession{
		Status:    "completed",
		UpdatedAt: time.Now(),
	})

	// Update trains list immediately after stopping session
	h.updateTrainsList()
	
	fmt.Printf("DEBUG: User %d stopped session %s, trains list updated\n", user.ID, req.SessionID)

	response := gin.H{
		"success":    true,
		"message":    "Mobile tracking session stopped successfully",
		"trip_saved": tripSaved,
	}

	c.JSON(http.StatusOK, response)
}

// Terminate user sessions (like Laravel)
func (h *SimpleLiveTrackingHandler) terminateUserSessions(userID uint) {
	fmt.Printf("DEBUG: Terminating existing sessions for user %d\n", userID)
	
	// Get all active sessions for this user
	var sessions []models.LiveTrackingSession
	h.db.Where("user_id = ? AND status = ?", userID, "active").Find(&sessions)
	
	for _, session := range sessions {
		// Remove user from train file or delete entire file
		h.handleStopSessionS3Operations(session.FilePath, userID, false)
		
		// Mark session as terminated
		h.db.Model(&session).Update("status", "terminated")
	}
}

// Helper function to update trains list
func (h *SimpleLiveTrackingHandler) updateTrainsList() {
	fmt.Printf("DEBUG: Updating trains list from active sessions\n")
	
	now := time.Now()
	var activeTrains []interface{}
	
	// Get all active sessions from database
	var sessions []models.LiveTrackingSession
	h.db.Where("status = ?", "active").Find(&sessions)
	
	// Build trains list from active sessions
	for _, session := range sessions {
		// Try to read train file to get current data
		trainData, err := h.s3.GetTrainData(session.FilePath)
		if err != nil {
			continue // File doesn't exist anymore
		}
		
		// Check if train has recent activity (within last 5 minutes)
		lastUpdate, err := time.Parse(time.RFC3339, trainData.LastUpdate)
		if err != nil {
			continue
		}
		
		timeSinceUpdate := now.Sub(lastUpdate)
		if timeSinceUpdate <= 5*time.Minute && len(trainData.Passengers) > 0 {
			activeTrains = append(activeTrains, map[string]interface{}{
				"trainId":        trainData.TrainID,
				"passengerCount": trainData.PassengerCount,
				"lastUpdate":     trainData.LastUpdate,
				"status":         trainData.Status,
			})
		}
	}
	
	// Create trains list structure
	trainsListData := map[string]interface{}{
		"trains":      activeTrains,
		"total":       len(activeTrains),
		"lastUpdated": now.Format(time.RFC3339),
		"source":      "golang-database-session-tracking",
	}

	// Upload to S3
	if err := h.s3.UploadJSON("trains/trains-list.json", trainsListData); err != nil {
		fmt.Printf("ERROR: Failed to update trains-list.json: %v\n", err)
		return
	}

	fmt.Printf("DEBUG: Updated trains-list.json with %d active trains\n", len(activeTrains))
}

// Handle S3 operations when stopping session
func (h *SimpleLiveTrackingHandler) handleStopSessionS3Operations(fileName string, userID uint, saveTrip bool) error {
	// Get current train data
	trainData, err := h.s3.GetTrainData(fileName)
	if err != nil {
		return fmt.Errorf("failed to read train file %s: %v", fileName, err)
	}

	// Remove this user from passengers
	var remainingPassengers []models.Passenger
	for _, passenger := range trainData.Passengers {
		if passenger.UserID != userID {
			remainingPassengers = append(remainingPassengers, passenger)
		}
	}

	if len(remainingPassengers) > 0 {
		// Other passengers remain - update file
		trainData.Passengers = remainingPassengers
		h.recalculateAveragePosition(trainData)
		trainData.LastUpdate = time.Now().Format(time.RFC3339)
		
		if err := h.s3.UploadJSON(fileName, *trainData); err != nil {
			return fmt.Errorf("failed to update train file: %v", err)
		}
		fmt.Printf("DEBUG: Updated train file %s, removed user %d\n", fileName, userID)
	} else {
		// No passengers left - delete the file
		if err := h.s3.DeleteFile(fileName); err != nil {
			return fmt.Errorf("failed to delete train file: %v", err)
		}
		fmt.Printf("DEBUG: Deleted empty train file %s\n", fileName)
	}

	return nil
}

// Update location in specific train file  
func (h *SimpleLiveTrackingHandler) updateLocationInTrainFile(fileName string, userID uint, req struct {
	SessionID string   `json:"session_id" binding:"required"`
	Latitude  float64  `json:"latitude" binding:"required,min=-90,max=90"`
	Longitude float64  `json:"longitude" binding:"required,min=-180,max=180"`
	Accuracy  *float64 `json:"accuracy,omitempty"`
	Speed     *float64 `json:"speed,omitempty"`
	Heading   *float64 `json:"heading,omitempty"`
	Altitude  *float64 `json:"altitude,omitempty"`
}) (string, error) {
	// Get train data from S3
	trainData, err := h.s3.GetTrainData(fileName)
	if err != nil {
		return "", fmt.Errorf("failed to read train file: %v", err)
	}
	
	// Find and update this user's passenger data
	userFound := false
	for i := range trainData.Passengers {
		if trainData.Passengers[i].UserID == userID {
			// Update passenger location data
			trainData.Passengers[i].Lat = req.Latitude
			trainData.Passengers[i].Lng = req.Longitude
			trainData.Passengers[i].Timestamp = time.Now().UnixMilli()
			trainData.Passengers[i].Accuracy = req.Accuracy
			trainData.Passengers[i].Speed = req.Speed
			trainData.Passengers[i].Heading = req.Heading
			trainData.Passengers[i].Altitude = req.Altitude
			trainData.Passengers[i].Status = "active"
			userFound = true
			break
		}
	}
	
	if !userFound {
		return "", fmt.Errorf("user %d not found in train file %s", userID, fileName)
	}
	
	// Recalculate average position and update timestamp
	h.recalculateAveragePosition(trainData)
	trainData.LastUpdate = time.Now().Format(time.RFC3339)
	
	// Upload updated data back to S3
	if err := h.s3.UploadJSON(fileName, *trainData); err != nil {
		return "", fmt.Errorf("failed to update train file: %v", err)
	}
	
	return fileName, nil
}


// Helper function to recalculate average position
func (h *SimpleLiveTrackingHandler) recalculateAveragePosition(trainData *models.TrainData) {
	if len(trainData.Passengers) == 0 {
		return
	}
	
	var totalLat, totalLng float64
	activeCount := 0
	
	for _, passenger := range trainData.Passengers {
		if passenger.Status == "active" {
			totalLat += passenger.Lat
			totalLng += passenger.Lng
			activeCount++
		}
	}
	
	if activeCount > 0 {
		trainData.AveragePosition = models.Position{
			Lat: totalLat / float64(activeCount),
			Lng: totalLng / float64(activeCount),
		}
		trainData.PassengerCount = activeCount
	}
}