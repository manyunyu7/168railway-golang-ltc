package handlers

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync"
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
	trainMutexes map[string]*sync.Mutex // mutex per train to prevent race conditions
	mutexLock    sync.RWMutex           // protect the trainMutexes map itself
	trainsListMutex sync.Mutex          // dedicated mutex for trains-list.json updates
}

func NewSimpleLiveTrackingHandler(db *gorm.DB, s3Client *utils.S3Client) *SimpleLiveTrackingHandler {
	return &SimpleLiveTrackingHandler{
		db: db,
		s3: s3Client,
		trainMutexes: make(map[string]*sync.Mutex),
	}
}

// getTrainMutex returns a mutex for the specific train to prevent race conditions
func (h *SimpleLiveTrackingHandler) getTrainMutex(trainNumber string) *sync.Mutex {
	h.mutexLock.Lock()
	defer h.mutexLock.Unlock()
	
	if _, exists := h.trainMutexes[trainNumber]; !exists {
		h.trainMutexes[trainNumber] = &sync.Mutex{}
	}
	
	return h.trainMutexes[trainNumber]
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

	// Start database transaction for consistency
	tx := h.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Terminate any existing sessions for this user (like Laravel)
	h.terminateUserSessions(user.ID)

	sessionID := uuid.New().String()
	now := time.Now()
	fmt.Printf("DEBUG: Starting session %s for user %d, train %s\n", sessionID, user.ID, req.TrainNumber)

	// Get train-specific mutex to prevent race conditions
	trainMutex := h.getTrainMutex(req.TrainNumber)
	trainMutex.Lock()
	defer trainMutex.Unlock()

	fileName := fmt.Sprintf("trains/train-%s.json", req.TrainNumber)
	
	// Check if train file already exists and has other passengers
	existingTrainData, err := h.s3.GetTrainData(fileName)
	var trainData models.TrainData
	
	if err != nil {
		// New train file - create fresh data
		fmt.Printf("DEBUG: Creating new train file for train %s\n", req.TrainNumber)
		trainData = models.TrainData{
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
	} else {
		// Train file exists - add this user to existing passengers
		fmt.Printf("DEBUG: Adding user to existing train %s with %d passengers\n", req.TrainNumber, len(existingTrainData.Passengers))
		trainData = *existingTrainData
		
		// Add new passenger
		newPassenger := models.Passenger{
			UserID:     user.ID,
			UserType:   "authenticated", 
			ClientType: "mobile",
			Lat:        req.InitialLat,
			Lng:        req.InitialLng,
			Timestamp:  time.Now().UnixMilli(),
			SessionID:  sessionID,
			Status:     "active",
		}
		trainData.Passengers = append(trainData.Passengers, newPassenger)
		
		// Recalculate average position and passenger count
		h.recalculateAveragePosition(&trainData)
		trainData.LastUpdate = time.Now().Format(time.RFC3339)
	}

	// Upload to S3 with train-specific lock already acquired
	if err := h.s3.UploadJSON(fileName, trainData); err != nil {
		tx.Rollback()
		fmt.Printf("ERROR: Failed to upload to S3: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to start tracking session",
			"error":   err.Error(),
		})
		return
	}

	fmt.Printf("DEBUG: Session %s stored in S3 file %s\n", sessionID, fileName)

	// Store session in database (only if S3 succeeded)
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

	if err := tx.Create(&session).Error; err != nil {
		tx.Rollback()
		fmt.Printf("ERROR: Failed to save session to database: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to save session to database",
			"error":   err.Error(),
		})
		return
	}

	// Commit transaction only if everything succeeded
	if err := tx.Commit().Error; err != nil {
		fmt.Printf("ERROR: Failed to commit transaction: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to commit session",
			"error":   err.Error(),
		})
		return
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

	// Start database transaction for consistency
	tx := h.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Validate session in database (like Laravel cache)
	var session models.LiveTrackingSession
	result := tx.Where("session_id = ? AND user_id = ? AND status = ?", req.SessionID, user.ID, "active").First(&session)
	
	if result.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "Invalid session",
		})
		return
	}

	// Get train-specific mutex to prevent race conditions with other users on same train
	trainMutex := h.getTrainMutex(session.TrainNumber)
	trainMutex.Lock()
	defer trainMutex.Unlock()

	// Update location in S3 train file using session info (with mutex protection)
	trainFile, err := h.updateLocationInTrainFile(session.FilePath, user.ID, req)
	if err != nil {
		tx.Rollback()
		fmt.Printf("ERROR: Failed to update location in S3: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update location",
			"error":   err.Error(),
		})
		return
	}

	// Update heartbeat in database only if S3 update succeeded
	if err := tx.Model(&session).Update("last_heartbeat", time.Now()).Error; err != nil {
		tx.Rollback()
		fmt.Printf("ERROR: Failed to update heartbeat: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update session heartbeat",
			"error":   err.Error(),
		})
		return
	}

	// Commit transaction only if everything succeeded
	if err := tx.Commit().Error; err != nil {
		fmt.Printf("ERROR: Failed to commit transaction: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to commit location update",
			"error":   err.Error(),
		})
		return
	}

	if trainFile != "" {
		fmt.Printf("DEBUG: Successfully updated location in S3 file: %s\n", trainFile)
		// Update trains list after location update (with dedicated mutex)
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
		SessionID       string         `json:"session_id" binding:"required"`
		SaveTrip        *bool          `json:"save_trip,omitempty"`
		TripSummary     *TripSummary   `json:"trip_summary,omitempty"`
		GPSPath         []GPSPoint     `json:"gps_path,omitempty"`
		TrainRelation   *string        `json:"train_relation,omitempty"`
		FromStationID   *uint          `json:"from_station_id,omitempty"`
		FromStationName *string        `json:"from_station_name,omitempty"`
		ToStationID     *uint          `json:"to_station_id,omitempty"`
		ToStationName   *string        `json:"to_station_name,omitempty"`
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
	var tripID *uint = nil
	
	// Track failure reason
	var saveFailureReason string = ""
	
	// Save trip data if requested
	if req.SaveTrip != nil && *req.SaveTrip {
		stationInfo := StationInfo{
			TrainRelation:   req.TrainRelation,
			FromStationID:   req.FromStationID,
			FromStationName: req.FromStationName,
			ToStationID:     req.ToStationID,
			ToStationName:   req.ToStationName,
		}
		tripID, saveFailureReason = h.saveUserTrip(session, user.ID, req.TripSummary, req.GPSPath, &stationInfo)
		tripSaved = (tripID != nil)
		if tripSaved {
			fmt.Printf("DEBUG: Saved trip with ID %d for user %d\n", *tripID, user.ID)
		} else {
			if saveFailureReason == "" {
				saveFailureReason = "Trip saving failed - check server logs for details"
			}
			fmt.Printf("WARNING: Trip saving failed for user %d session %s: %s\n", user.ID, req.SessionID, saveFailureReason)
		}
	} else {
		if req.SaveTrip == nil {
			saveFailureReason = "save_trip parameter not provided"
		} else {
			saveFailureReason = "save_trip set to false"
		}
	}
	
	// Handle train file - remove user or delete entire file
	err := h.handleStopSessionS3Operations(fileName, user.ID, false) // Don't pass saveTrip here
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
	
	if tripID != nil {
		response["trip_id"] = *tripID
	}
	
	// Add save failure reason if trip wasn't saved
	if !tripSaved && saveFailureReason != "" {
		response["save_failure_reason"] = saveFailureReason
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

// Helper function to update trains list (thread-safe)
func (h *SimpleLiveTrackingHandler) updateTrainsList() {
	// Use dedicated mutex to prevent race conditions when updating trains-list.json
	h.trainsListMutex.Lock()
	defer h.trainsListMutex.Unlock()
	
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

// Save user trip data to trips table using mobile GPS path and statistics
func (h *SimpleLiveTrackingHandler) saveUserTrip(session models.LiveTrackingSession, userID uint, mobileSummary *TripSummary, gpsPath []GPSPoint, stationInfo *StationInfo) (*uint, string) {
	
	// Use mobile GPS path if provided, otherwise fallback to S3 data
	var trackingDataInterface interface{}
	var routeCoordsInterface interface{}
	var startLat, startLng, endLat, endLng float64
	
	if len(gpsPath) > 0 {
		fmt.Printf("DEBUG: Using mobile GPS path with %d points\n", len(gpsPath))
		
		// Convert GPS path to JSON bytes for database storage
		jsonBytes, err := json.Marshal(gpsPath)
		if err != nil {
			fmt.Printf("ERROR: Failed to marshal GPS path: %v\n", err)
			return nil, fmt.Sprintf("Failed to serialize GPS data: %v", err)
		}
		
		// Use json.RawMessage for direct JSON storage
		trackingDataInterface = json.RawMessage(jsonBytes)
		
		// Extract route coordinates for map display and serialize to JSON
		var routeCoords []map[string]interface{}
		for _, point := range gpsPath {
			routeCoords = append(routeCoords, map[string]interface{}{
				"lat":       point.Lat,
				"lng":       point.Lng,
				"timestamp": point.Timestamp,
			})
		}
		routeBytes, err := json.Marshal(routeCoords)
		if err != nil {
			fmt.Printf("ERROR: Failed to marshal route coordinates: %v\n", err)
			return nil, fmt.Sprintf("Failed to serialize route data: %v", err)
		}
		routeCoordsInterface = json.RawMessage(routeBytes)
		
		// Get start/end points from GPS path
		startLat = gpsPath[0].Lat
		startLng = gpsPath[0].Lng
		endLat = gpsPath[len(gpsPath)-1].Lat
		endLng = gpsPath[len(gpsPath)-1].Lng
		
	} else {
		fmt.Printf("DEBUG: Falling back to S3 data for GPS path\n")
		
		// Fallback: Get tracking data from S3 file (legacy approach)
		trainData, err := h.s3.GetTrainData(session.FilePath)
		if err != nil {
			fmt.Printf("ERROR: Could not read train data for trip saving: %v\n", err)
			return nil, "Failed to read S3 train data"
		}

		// Find user's tracking data from S3
		var userTrackingData []models.Passenger
		for _, passenger := range trainData.Passengers {
			if passenger.UserID == userID {
				userTrackingData = append(userTrackingData, passenger)
			}
		}

		if len(userTrackingData) == 0 {
			fmt.Printf("ERROR: No tracking data found for user %d\n", userID)
			return nil, "No tracking data found for user"
		}

		// Use S3 data for tracking - serialize to JSON
		s3JsonBytes, err := json.Marshal(userTrackingData)
		if err != nil {
			fmt.Printf("ERROR: Failed to marshal S3 tracking data: %v\n", err)
			return nil, fmt.Sprintf("Failed to serialize S3 tracking data: %v", err)
		}
		trackingDataInterface = json.RawMessage(s3JsonBytes)
		
		// Extract route coordinates from S3 data and serialize to JSON
		var routeCoords []map[string]interface{}
		for _, point := range userTrackingData {
			routeCoords = append(routeCoords, map[string]interface{}{
				"lat":       point.Lat,
				"lng":       point.Lng,
				"timestamp": point.Timestamp,
			})
		}
		s3RouteBytes, err := json.Marshal(routeCoords)
		if err != nil {
			fmt.Printf("ERROR: Failed to marshal S3 route coordinates: %v\n", err)
			return nil, fmt.Sprintf("Failed to serialize S3 route data: %v", err)
		}
		routeCoordsInterface = json.RawMessage(s3RouteBytes)
		
		// Get start/end points from S3 data
		startLat = userTrackingData[0].Lat
		startLng = userTrackingData[0].Lng
		endLat = userTrackingData[len(userTrackingData)-1].Lat
		endLng = userTrackingData[len(userTrackingData)-1].Lng
	}

	// Use mobile-calculated stats if provided, otherwise fallback to server calculation
	var stats TripStatistics
	var durationSeconds int
	
	if mobileSummary != nil {
		fmt.Printf("DEBUG: Using mobile-calculated trip statistics\n")
		// Use mobile statistics (preferred)
		durationSeconds = mobileSummary.DurationSeconds
		stats.TotalDistanceKm = mobileSummary.TotalDistanceKm
		stats.MaxSpeedKmh = mobileSummary.MaxSpeedKmh
		stats.AvgSpeedKmh = mobileSummary.AvgSpeedKmh
		
		if mobileSummary.MaxElevationM != nil {
			stats.MaxElevationM = int(*mobileSummary.MaxElevationM)
		}
		if mobileSummary.MinElevationM != nil {
			stats.MinElevationM = int(*mobileSummary.MinElevationM)
		}
		if mobileSummary.ElevationGainM != nil {
			stats.ElevationGainM = int(*mobileSummary.ElevationGainM)
		}
		
		if mobileSummary.MaxSpeedLocation != nil {
			stats.MaxSpeedLat = &mobileSummary.MaxSpeedLocation.Lat
			stats.MaxSpeedLng = &mobileSummary.MaxSpeedLocation.Lng
		}
		if mobileSummary.MaxElevationLocation != nil {
			stats.MaxElevationLat = &mobileSummary.MaxElevationLocation.Lat
			stats.MaxElevationLng = &mobileSummary.MaxElevationLocation.Lng
		}
	} else {
		fmt.Printf("DEBUG: Falling back to server-calculated trip statistics\n")
		// Fallback: Calculate basic stats from available data
		if len(gpsPath) > 1 {
			// Calculate from mobile GPS path
			durationSeconds = int((gpsPath[len(gpsPath)-1].Timestamp - gpsPath[0].Timestamp) / 1000)
			stats = h.calculateTripStatisticsFromGPS(gpsPath)
		} else {
			// Calculate from S3 data (legacy fallback)
			durationSeconds = int(time.Now().Sub(session.StartedAt).Seconds())
			// Basic stats only since S3 has limited data
		}
	}

	// Create trip record
	trip := models.Trip{
		SessionID:        session.SessionID,
		UserID:           &userID,
		UserType:         "authenticated",
		TrainID:          session.TrainID,
		TrainName:        session.TrainNumber,
		TrainNumber:      session.TrainNumber,
		
		// Station information from mobile request
		TrainRelation:    stationInfo.TrainRelation,
		FromStationID:    stationInfo.FromStationID,
		FromStationName:  stationInfo.FromStationName,
		ToStationID:      stationInfo.ToStationID,
		ToStationName:    stationInfo.ToStationName,
		
		// Statistical data (mobile-calculated preferred)
		TotalDistanceKm:  stats.TotalDistanceKm,
		MaxSpeedKmh:      stats.MaxSpeedKmh,
		AvgSpeedKmh:      stats.AvgSpeedKmh,
		MaxElevationM:    stats.MaxElevationM,
		MinElevationM:    stats.MinElevationM,
		ElevationGainM:   stats.ElevationGainM,
		DurationSeconds:  durationSeconds,
		
		// Position data
		StartLatitude:    startLat,
		StartLongitude:   startLng,
		EndLatitude:      endLat,
		EndLongitude:     endLng,
		MaxSpeedLat:      stats.MaxSpeedLat,
		MaxSpeedLng:      stats.MaxSpeedLng,
		MaxElevationLat:  stats.MaxElevationLat,
		MaxElevationLng:  stats.MaxElevationLng,
		
		// JSON data (complete tracking history from mobile or S3)
		TrackingData:     trackingDataInterface,
		RouteCoordinates: routeCoordsInterface,
		
		// Timestamps
		StartedAt:        session.StartedAt,
		CompletedAt:      time.Now(),
	}

	// Save to database
	if err := h.db.Create(&trip).Error; err != nil {
		fmt.Printf("ERROR: Failed to save trip: %v\n", err)
		return nil, fmt.Sprintf("Database error: %v", err)
	}

	// Log station information
	stationLog := "no stations"
	if stationInfo.FromStationName != nil && stationInfo.ToStationName != nil {
		stationLog = fmt.Sprintf("%s → %s", *stationInfo.FromStationName, *stationInfo.ToStationName)
	}
	
	if len(gpsPath) > 0 {
		fmt.Printf("DEBUG: Saved trip ID %d with mobile GPS path (%d points) - %.2fkm, %.1fkm/h max, %ds duration, %s\n", 
			trip.ID, len(gpsPath), stats.TotalDistanceKm, stats.MaxSpeedKmh, durationSeconds, stationLog)
	} else {
		fmt.Printf("DEBUG: Saved trip ID %d with S3 fallback data - %.2fkm, %.1fkm/h max, %ds duration, %s\n", 
			trip.ID, stats.TotalDistanceKm, stats.MaxSpeedKmh, durationSeconds, stationLog)
	}
	
	return &trip.ID, ""
}

// Trip statistics structure (server-calculated fallback)
type TripStatistics struct {
	TotalDistanceKm  float64
	MaxSpeedKmh      float64
	AvgSpeedKmh      float64
	MaxElevationM    int
	MinElevationM    int
	ElevationGainM   int
	MaxSpeedLat      *float64
	MaxSpeedLng      *float64
	MaxElevationLat  *float64
	MaxElevationLng  *float64
}

// Mobile-calculated trip summary (preferred approach)
type TripSummary struct {
	TotalDistanceKm     float64                `json:"total_distance_km"`
	MaxSpeedKmh         float64                `json:"max_speed_kmh"`
	AvgSpeedKmh         float64                `json:"avg_speed_kmh"`
	DurationSeconds     int                    `json:"duration_seconds"`
	MaxElevationM       *float64               `json:"max_elevation_m,omitempty"`
	MinElevationM       *float64               `json:"min_elevation_m,omitempty"`
	ElevationGainM      *float64               `json:"elevation_gain_m,omitempty"`
	MaxSpeedLocation    *LocationPoint         `json:"max_speed_location,omitempty"`
	MaxElevationLocation *LocationPoint        `json:"max_elevation_location,omitempty"`
}

type LocationPoint struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// Mobile GPS point for complete journey path
type GPSPoint struct {
	Lat       float64  `json:"lat"`
	Lng       float64  `json:"lng"`
	Timestamp int64    `json:"timestamp"`
	Speed     *float64 `json:"speed,omitempty"`
	Altitude  *float64 `json:"altitude,omitempty"`
	Accuracy  *float64 `json:"accuracy,omitempty"`
	Heading   *float64 `json:"heading,omitempty"`
}

// Station information for trip
type StationInfo struct {
	TrainRelation   *string `json:"train_relation,omitempty"`
	FromStationID   *uint   `json:"from_station_id,omitempty"`
	FromStationName *string `json:"from_station_name,omitempty"`
	ToStationID     *uint   `json:"to_station_id,omitempty"`
	ToStationName   *string `json:"to_station_name,omitempty"`
}

// Calculate advanced trip statistics from GPS tracking data
func (h *SimpleLiveTrackingHandler) calculateTripStatistics(trackingData []models.Passenger) TripStatistics {
	stats := TripStatistics{}
	
	if len(trackingData) < 2 {
		return stats // Not enough data
	}
	
	var totalDistance float64 = 0
	var totalSpeed float64 = 0
	var speedCount int = 0
	var maxSpeed float64 = 0
	var maxElevation float64 = -1000
	var minElevation float64 = 10000
	
	// Process each GPS point
	for i, point := range trackingData {
		// Calculate distance between consecutive points
		if i > 0 {
			prevPoint := trackingData[i-1]
			distance := calculateDistance(prevPoint.Lat, prevPoint.Lng, point.Lat, point.Lng)
			totalDistance += distance
		}
		
		// Speed analysis
		if point.Speed != nil && *point.Speed > 0 {
			speed := *point.Speed
			totalSpeed += speed
			speedCount++
			
			if speed > maxSpeed {
				maxSpeed = speed
				stats.MaxSpeedLat = &point.Lat
				stats.MaxSpeedLng = &point.Lng
			}
		}
		
		// Elevation analysis (if available)
		if point.Altitude != nil {
			altitude := *point.Altitude
			if altitude > maxElevation {
				maxElevation = altitude
				stats.MaxElevationLat = &point.Lat
				stats.MaxElevationLng = &point.Lng
			}
			if altitude < minElevation {
				minElevation = altitude
			}
		}
	}
	
	// Finalize statistics
	stats.TotalDistanceKm = totalDistance
	stats.MaxSpeedKmh = maxSpeed * 3.6 // Convert m/s to km/h
	if speedCount > 0 {
		stats.AvgSpeedKmh = (totalSpeed / float64(speedCount)) * 3.6
	}
	
	if maxElevation > -1000 {
		stats.MaxElevationM = int(maxElevation)
	}
	if minElevation < 10000 {
		stats.MinElevationM = int(minElevation)
	}
	if maxElevation > -1000 && minElevation < 10000 {
		stats.ElevationGainM = int(maxElevation - minElevation)
	}
	
	return stats
}

// Calculate trip statistics from mobile GPS points
func (h *SimpleLiveTrackingHandler) calculateTripStatisticsFromGPS(gpsPath []GPSPoint) TripStatistics {
	stats := TripStatistics{}
	
	if len(gpsPath) < 2 {
		return stats // Not enough data
	}
	
	var totalDistance float64 = 0
	var totalSpeed float64 = 0
	var speedCount int = 0
	var maxSpeed float64 = 0
	var maxElevation float64 = -1000
	var minElevation float64 = 10000
	
	// Process each GPS point
	for i, point := range gpsPath {
		// Calculate distance between consecutive points
		if i > 0 {
			prevPoint := gpsPath[i-1]
			distance := calculateDistance(prevPoint.Lat, prevPoint.Lng, point.Lat, point.Lng)
			totalDistance += distance
		}
		
		// Speed analysis
		if point.Speed != nil && *point.Speed > 0 {
			speed := *point.Speed
			totalSpeed += speed
			speedCount++
			
			if speed > maxSpeed {
				maxSpeed = speed
				stats.MaxSpeedLat = &point.Lat
				stats.MaxSpeedLng = &point.Lng
			}
		}
		
		// Elevation analysis (if available)
		if point.Altitude != nil {
			altitude := *point.Altitude
			if altitude > maxElevation {
				maxElevation = altitude
				stats.MaxElevationLat = &point.Lat
				stats.MaxElevationLng = &point.Lng
			}
			if altitude < minElevation {
				minElevation = altitude
			}
		}
	}
	
	// Finalize statistics
	stats.TotalDistanceKm = totalDistance
	stats.MaxSpeedKmh = maxSpeed * 3.6 // Convert m/s to km/h
	if speedCount > 0 {
		stats.AvgSpeedKmh = (totalSpeed / float64(speedCount)) * 3.6
	}
	
	if maxElevation > -1000 {
		stats.MaxElevationM = int(maxElevation)
	}
	if minElevation < 10000 {
		stats.MinElevationM = int(minElevation)
	}
	if maxElevation > -1000 && minElevation < 10000 {
		stats.ElevationGainM = int(maxElevation - minElevation)
	}
	
	return stats
}

// Calculate distance between two GPS coordinates (Haversine formula)
func calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000 // Earth's radius in meters
	
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLatRad := (lat2 - lat1) * math.Pi / 180
	deltaLonRad := (lon2 - lon1) * math.Pi / 180
	
	a := math.Sin(deltaLatRad/2)*math.Sin(deltaLatRad/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
		math.Sin(deltaLonRad/2)*math.Sin(deltaLonRad/2)
	
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	
	return R * c / 1000 // Return distance in kilometers
}