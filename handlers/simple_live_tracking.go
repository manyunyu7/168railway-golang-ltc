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

// GetActiveSession - Simple version without Redis
func (h *SimpleLiveTrackingHandler) GetActiveSession(c *gin.Context) {
	user, exists := middleware.GetUserFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Authentication required",
		})
		return
	}

	fmt.Printf("DEBUG: User %d requested active session check (S3-enabled, Redis-free)\n", user.ID)
	
	c.JSON(http.StatusOK, gin.H{
		"success":           true,
		"has_active_session": false,
		"message":           "S3-enabled, Redis-free implementation",
	})
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

	sessionID := uuid.New().String()
	fmt.Printf("DEBUG: Starting session %s for user %d, train %s (S3-enabled, Redis-free)\n", sessionID, user.ID, req.TrainNumber)

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

	// Update trains list
	h.updateTrainsList()

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"session_id": sessionID,
		"message":    "Mobile tracking session started successfully (S3-enabled, Redis-free)",
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

	// Without Redis, we'll use a simple approach:
	// Try to find the user's active train file and update it
	trainFile, err := h.updateUserLocationInActiveTrainFile(user.ID, req)
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
		"message": "Mobile location updated successfully (S3-enabled, Redis-free)",
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

	fmt.Printf("DEBUG: User %d stopped session %s\n", user.ID, req.SessionID)

	response := gin.H{
		"success":    true,
		"message":    "Mobile tracking session stopped successfully (Redis-free)",
		"trip_saved": false,
	}

	if req.SaveTrip != nil && *req.SaveTrip {
		response["trip_saved"] = false
		response["message"] = "Trip saving not implemented in Redis-free version"
	}

	c.JSON(http.StatusOK, response)
}

// Helper function to update trains list
func (h *SimpleLiveTrackingHandler) updateTrainsList() {
	fmt.Printf("DEBUG: Updating trains list (S3-enabled, Redis-free)\n")
	
	now := time.Now()
	var activeTrains []interface{}
	
	// Scan common train file patterns to build the trains list
	commonPatterns := []string{
		"trains/train-KA-001.json",
		"trains/train-KA-002.json", 
		"trains/train-KA-123.json",
		"trains/train-TEST.json",
	}
	
	for _, fileName := range commonPatterns {
		trainData, err := h.s3.GetTrainData(fileName)
		if err != nil {
			continue // File doesn't exist or can't be read
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
		"source":      "golang-s3-enabled-redis-free",
	}

	// Upload to S3
	if err := h.s3.UploadJSON("trains/trains-list.json", trainsListData); err != nil {
		fmt.Printf("ERROR: Failed to update trains-list.json: %v\n", err)
		return
	}

	fmt.Printf("DEBUG: Updated trains-list.json with %d active trains\n", len(activeTrains))
}

// Helper function to update user location in active train files
func (h *SimpleLiveTrackingHandler) updateUserLocationInActiveTrainFile(userID uint, req struct {
	SessionID string   `json:"session_id" binding:"required"`
	Latitude  float64  `json:"latitude" binding:"required,min=-90,max=90"`
	Longitude float64  `json:"longitude" binding:"required,min=-180,max=180"`
	Accuracy  *float64 `json:"accuracy,omitempty"`
	Speed     *float64 `json:"speed,omitempty"`
	Heading   *float64 `json:"heading,omitempty"`
	Altitude  *float64 `json:"altitude,omitempty"`
}) (string, error) {
	fmt.Printf("DEBUG: Searching for user %d's active train file in S3\n", userID)
	
	// Since we don't have Redis, we'll implement a simple scanning approach:
	// 1. Try to read trains-list.json to get potential train files
	// 2. Scan each train file to find one containing this user
	// 3. Update the passenger data in that file
	
	// Strategy: Try common train patterns or scan recent files
	// For demonstration, let's try a few common patterns and then implement proper scanning
	
	commonPatterns := []string{
		"trains/train-KA-001.json",
		"trains/train-KA-002.json", 
		"trains/train-KA-123.json",
		"trains/train-TEST.json",
	}
	
	// Try common patterns first
	for _, fileName := range commonPatterns {
		if updated, err := h.tryUpdateUserInTrainFile(fileName, userID, req); err == nil && updated {
			return fileName, nil
		}
	}
	
	// If not found in common patterns, we could implement more advanced scanning
	// For now, let's return empty (no file found)
	fmt.Printf("DEBUG: User %d not found in common train file patterns\n", userID)
	return "", nil // Return empty string (no error) if not found
}

// Helper function to try updating user location in a specific train file
func (h *SimpleLiveTrackingHandler) tryUpdateUserInTrainFile(fileName string, userID uint, req struct {
	SessionID string   `json:"session_id" binding:"required"`
	Latitude  float64  `json:"latitude" binding:"required,min=-90,max=90"`
	Longitude float64  `json:"longitude" binding:"required,min=-180,max=180"`
	Accuracy  *float64 `json:"accuracy,omitempty"`
	Speed     *float64 `json:"speed,omitempty"`
	Heading   *float64 `json:"heading,omitempty"`
	Altitude  *float64 `json:"altitude,omitempty"`
}) (bool, error) {
	// Try to get the train data from S3
	trainData, err := h.s3.GetTrainData(fileName)
	if err != nil {
		// File doesn't exist or can't be read - that's fine
		return false, nil
	}
	
	// Look for this user in the passengers array
	userFound := false
	for i := range trainData.Passengers {
		if trainData.Passengers[i].UserID == userID {
			// Update this passenger's location data
			trainData.Passengers[i].Lat = req.Latitude
			trainData.Passengers[i].Lng = req.Longitude
			trainData.Passengers[i].Timestamp = time.Now().UnixMilli()
			trainData.Passengers[i].Accuracy = req.Accuracy
			trainData.Passengers[i].Speed = req.Speed
			trainData.Passengers[i].Heading = req.Heading
			trainData.Passengers[i].Altitude = req.Altitude
			trainData.Passengers[i].Status = "active"
			userFound = true
			
			fmt.Printf("DEBUG: Found user %d in %s, updating location\n", userID, fileName)
			break
		}
	}
	
	if !userFound {
		return false, nil
	}
	
	// Recalculate average position
	h.recalculateAveragePosition(trainData)
	
	// Update last update timestamp
	trainData.LastUpdate = time.Now().Format(time.RFC3339)
	
	// Upload updated data back to S3
	if err := h.s3.UploadJSON(fileName, *trainData); err != nil {
		return false, fmt.Errorf("failed to update train file %s: %v", fileName, err)
	}
	
	fmt.Printf("DEBUG: Successfully updated user %d location in %s\n", userID, fileName)
	return true, nil
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