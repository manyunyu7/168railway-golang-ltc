package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/modernland/golang-live-tracking/models"
	"github.com/modernland/golang-live-tracking/middleware"
	"github.com/modernland/golang-live-tracking/utils"
)

type LiveTrackingHandler struct {
	db     *gorm.DB
	redis  *redis.Client
	s3     *utils.S3Client
}

func NewLiveTrackingHandler(db *gorm.DB, redisClient *redis.Client, s3Client *utils.S3Client) *LiveTrackingHandler {
	return &LiveTrackingHandler{
		db:    db,
		redis: redisClient,
		s3:    s3Client,
	}
}

// GetActiveSession - GET /api/mobile/live-tracking/active-session
func (h *LiveTrackingHandler) GetActiveSession(c *gin.Context) {
	user, exists := middleware.GetUserFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Authentication required",
		})
		return
	}

	// Skip Redis and return no active session for now
	// In a production setup, you could check database or S3 for active sessions
	fmt.Printf("DEBUG: User %d requested active session check (Redis disabled)\n", user.ID)
	
	c.JSON(http.StatusOK, gin.H{
		"success":           true,
		"has_active_session": false,
		"note":              "Redis disabled - no session tracking",
	})
}

// StartMobileSession - POST /api/mobile/live-tracking/start
func (h *LiveTrackingHandler) StartMobileSession(c *gin.Context) {
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

	// Skip terminating sessions since Redis is disabled
	sessionID := uuid.New().String()
	fmt.Printf("DEBUG: Starting session %s for user %d\n", sessionID, user.ID)

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
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to start tracking session",
		})
		return
	}

	// Skip Redis session storage - just log for now
	fmt.Printf("DEBUG: Session %s stored in S3 file %s\n", sessionID, fileName)

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"session_id": sessionID,
		"message":    "Mobile tracking session started successfully",
	})
}

// UpdateMobileLocation - POST /api/mobile/live-tracking/update
func (h *LiveTrackingHandler) UpdateMobileLocation(c *gin.Context) {
	user, exists := middleware.GetUserFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Authentication required",
		})
		return
	}

	var req struct {
		SessionID string   `json:"session_id" binding:"required,uuid"`
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

	ctx := context.Background()
	sessionKey := fmt.Sprintf("live_session_%s", req.SessionID)
	
	// Validate session
	sessionData, err := h.redis.HGetAll(ctx, sessionKey).Result()
	if err != nil || len(sessionData) == 0 {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "Invalid session",
		})
		return
	}

	userID, _ := strconv.ParseUint(sessionData["user_id"], 10, 32)
	if uint(userID) != user.ID {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "Invalid session",
		})
		return
	}

	// Update heartbeat
	h.redis.HSet(ctx, sessionKey, "last_heartbeat", time.Now().Format(time.RFC3339))

	// Get and update train file
	fileName := sessionData["file_path"]
	trainData, err := h.s3.GetTrainData(fileName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update location",
		})
		return
	}

	// Update passenger data
	passengerUpdated := false
	for i := range trainData.Passengers {
		if trainData.Passengers[i].UserID == user.ID {
			trainData.Passengers[i].Lat = req.Latitude
			trainData.Passengers[i].Lng = req.Longitude
			trainData.Passengers[i].Timestamp = time.Now().UnixMilli()
			trainData.Passengers[i].Accuracy = req.Accuracy
			trainData.Passengers[i].Speed = req.Speed
			trainData.Passengers[i].Heading = req.Heading
			trainData.Passengers[i].Altitude = req.Altitude
			trainData.Passengers[i].Status = "active"
			trainData.Passengers[i].ClientType = "mobile"
			passengerUpdated = true
			break
		}
	}

	// If passenger not found, add them
	if !passengerUpdated {
		trainData.Passengers = append(trainData.Passengers, models.Passenger{
			UserID:     user.ID,
			UserType:   "authenticated",
			ClientType: "mobile",
			Lat:        req.Latitude,
			Lng:        req.Longitude,
			Timestamp:  time.Now().UnixMilli(),
			SessionID:  req.SessionID,
			Accuracy:   req.Accuracy,
			Speed:      req.Speed,
			Heading:    req.Heading,
			Altitude:   req.Altitude,
			Status:     "active",
		})
	}

	// Filter active passengers and recalculate average
	h.filterActivePassengersAndRecalculate(trainData)
	trainData.LastUpdate = time.Now().Format(time.RFC3339)

	// Upload updated data
	if err := h.s3.UploadJSON(fileName, *trainData); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update location",
		})
		return
	}

	h.updateTrainsList()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Mobile location updated successfully",
	})
}

// Heartbeat - POST /api/mobile/live-tracking/heartbeat  
func (h *LiveTrackingHandler) Heartbeat(c *gin.Context) {
	user, exists := middleware.GetUserFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Authentication required",
		})
		return
	}

	var req struct {
		SessionID string  `json:"session_id" binding:"required,uuid"`
		AppState  *string `json:"app_state,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"errors":  err.Error(),
		})
		return
	}

	ctx := context.Background()
	sessionKey := fmt.Sprintf("live_session_%s", req.SessionID)
	
	// Validate session
	sessionData, err := h.redis.HGetAll(ctx, sessionKey).Result()
	if err != nil || len(sessionData) == 0 {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "Invalid session",
		})
		return
	}

	userID, _ := strconv.ParseUint(sessionData["user_id"], 10, 32)
	if uint(userID) != user.ID {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "Invalid session",
		})
		return
	}

	// Update heartbeat
	updateData := map[string]interface{}{
		"last_heartbeat": time.Now().Format(time.RFC3339),
	}
	if req.AppState != nil {
		updateData["app_state"] = *req.AppState
	}

	h.redis.HMSet(ctx, sessionKey, updateData)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Heartbeat received",
	})
}

// RecoverSession - POST /api/mobile/live-tracking/recover
func (h *LiveTrackingHandler) RecoverSession(c *gin.Context) {
	user, exists := middleware.GetUserFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Authentication required",
		})
		return
	}

	var req struct {
		SessionID string  `json:"session_id" binding:"required,uuid"`
		Reason    *string `json:"reason,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"errors":  err.Error(),
		})
		return
	}

	ctx := context.Background()
	sessionKey := fmt.Sprintf("live_session_%s", req.SessionID)
	
	// Validate session
	sessionData, err := h.redis.HGetAll(ctx, sessionKey).Result()
	if err != nil || len(sessionData) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Session not found or expired",
		})
		return
	}

	userID, _ := strconv.ParseUint(sessionData["user_id"], 10, 32)
	if uint(userID) != user.ID {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Session not found or expired",
		})
		return
	}

	// Update session as recovered
	updateData := map[string]interface{}{
		"last_heartbeat": time.Now().Format(time.RFC3339),
		"recovered_at":   time.Now().Format(time.RFC3339),
	}
	if req.Reason != nil {
		updateData["recovery_reason"] = *req.Reason
	}

	h.redis.HMSet(ctx, sessionKey, updateData)

	// Update passenger status to active in train file
	fileName := sessionData["file_path"]
	if trainData, err := h.s3.GetTrainData(fileName); err == nil {
		for i := range trainData.Passengers {
			if trainData.Passengers[i].UserID == user.ID {
				trainData.Passengers[i].Status = "active"
				break
			}
		}
		trainData.LastUpdate = time.Now().Format(time.RFC3339)
		h.s3.UploadJSON(fileName, *trainData)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"message":      "Session recovered successfully",
		"train_number": sessionData["train_number"],
	})
}

// StopMobileSession - POST /api/mobile/live-tracking/stop
func (h *LiveTrackingHandler) StopMobileSession(c *gin.Context) {
	user, exists := middleware.GetUserFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Authentication required",
		})
		return
	}

	var req struct {
		SessionID           string   `json:"session_id" binding:"required,uuid"`
		SaveTrip            *bool    `json:"save_trip,omitempty"`
		FromStationID       *uint    `json:"from_station_id,omitempty"`
		FromStationName     *string  `json:"from_station_name,omitempty"`
		ToStationID         *uint    `json:"to_station_id,omitempty"`
		ToStationName       *string  `json:"to_station_name,omitempty"`
		EndLat              *float64 `json:"end_lat,omitempty"`
		EndLng              *float64 `json:"end_lng,omitempty"`
		TotalDistanceKm     *float64 `json:"total_distance_km,omitempty"`
		MaxSpeedKmh         *float64 `json:"max_speed_kmh,omitempty"`
		AvgSpeedKmh         *float64 `json:"avg_speed_kmh,omitempty"`
		MaxElevationM       *float64 `json:"max_elevation_m,omitempty"`
		MinElevationM       *float64 `json:"min_elevation_m,omitempty"`
		ElevationGainM      *float64 `json:"elevation_gain_m,omitempty"`
		DurationSeconds     *int     `json:"duration_seconds,omitempty"`
		MaxSpeedLat         *float64 `json:"max_speed_lat,omitempty"`
		MaxSpeedLng         *float64 `json:"max_speed_lng,omitempty"`
		MaxElevationLat     *float64 `json:"max_elevation_lat,omitempty"`
		MaxElevationLng     *float64 `json:"max_elevation_lng,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"errors":  err.Error(),
		})
		return
	}

	ctx := context.Background()
	sessionKey := fmt.Sprintf("live_session_%s", req.SessionID)
	
	// Validate session
	sessionData, err := h.redis.HGetAll(ctx, sessionKey).Result()
	if err != nil || len(sessionData) == 0 {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "Invalid session",
		})
		return
	}

	userID, _ := strconv.ParseUint(sessionData["user_id"], 10, 32)
	if uint(userID) != user.ID {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "Invalid session",
		})
		return
	}

	fileName := sessionData["file_path"]
	var tripID *uint

	// Get tracking data before removing user
	if trainData, err := h.s3.GetTrainData(fileName); err == nil {
		var userTrackingData []models.Passenger
		
		// Extract this user's tracking data
		for _, passenger := range trainData.Passengers {
			if passenger.UserID == user.ID {
				userTrackingData = append(userTrackingData, passenger)
			}
		}

		// Save trip if requested
		if req.SaveTrip != nil && *req.SaveTrip && len(userTrackingData) > 0 {
			if savedTripID := h.saveMobileTrip(req, sessionData, userTrackingData, user); savedTripID != nil {
				tripID = savedTripID
			}
		}

		// Remove user from passengers
		var remainingPassengers []models.Passenger
		for _, passenger := range trainData.Passengers {
			if passenger.UserID != user.ID {
				remainingPassengers = append(remainingPassengers, passenger)
			}
		}

		if len(remainingPassengers) > 0 {
			trainData.Passengers = remainingPassengers
			h.filterActivePassengersAndRecalculate(trainData)
			trainData.LastUpdate = time.Now().Format(time.RFC3339)
			h.s3.UploadJSON(fileName, *trainData)
		} else {
			// No passengers left, delete file
			h.s3.DeleteFile(fileName)
		}
	}

	// Clear session caches
	h.redis.Del(ctx, sessionKey)
	userSessionsKey := fmt.Sprintf("user_sessions_%d", user.ID)
	h.redis.Del(ctx, userSessionsKey)

	h.updateTrainsList()

	response := gin.H{
		"success":    true,
		"message":    "Mobile tracking session stopped successfully",
		"trip_saved": tripID != nil,
	}

	if tripID != nil {
		response["trip_id"] = *tripID
	}

	c.JSON(http.StatusOK, response)
}

// Helper methods would continue here...
// (terminateUserSessions, updateTrainsList, filterActivePassengersAndRecalculate, saveMobileTrip)

func (h *LiveTrackingHandler) terminateUserSessions(userID uint) {
	ctx := context.Background()
	userSessionsKey := fmt.Sprintf("user_sessions_%d", userID)
	
	sessionIDs, err := h.redis.SMembers(ctx, userSessionsKey).Result()
	if err != nil {
		return
	}

	for _, sessionID := range sessionIDs {
		sessionKey := fmt.Sprintf("live_session_%s", sessionID)
		sessionData, err := h.redis.HGetAll(ctx, sessionKey).Result()
		if err != nil {
			continue
		}

		fileName := sessionData["file_path"]
		if fileName != "" {
			if trainData, err := h.s3.GetTrainData(fileName); err == nil {
				var remainingPassengers []models.Passenger
				for _, passenger := range trainData.Passengers {
					if passenger.UserID != userID {
						remainingPassengers = append(remainingPassengers, passenger)
					}
				}

				if len(remainingPassengers) > 0 {
					trainData.Passengers = remainingPassengers
					trainData.PassengerCount = len(remainingPassengers)
					h.s3.UploadJSON(fileName, *trainData)
				} else {
					h.s3.DeleteFile(fileName)
				}
			}
		}

		h.redis.Del(ctx, sessionKey)
	}

	h.redis.Del(ctx, userSessionsKey)
}

func (h *LiveTrackingHandler) updateTrainsList() {
	// Read existing trains-list.json to preserve trains from other systems (Laravel)
	ctx := context.Background()
	existingTrains := []map[string]interface{}{}
	
	// Try to get existing trains-list.json
	if existingData, err := h.s3.GetTrainData("trains/trains-list.json"); err == nil {
		// Parse the existing trains-list.json structure
		if trainsData, ok := existingData.Passengers.([]interface{}); ok {
			for _, train := range trainsData {
				if trainMap, ok := train.(map[string]interface{}); ok {
					existingTrains = append(existingTrains, trainMap)
				}
			}
		}
	}

	// Alternative: Read trains-list.json directly as JSON
	if len(existingTrains) == 0 {
		if resp, err := http.Get(fmt.Sprintf("%s/trains/trains-list.json", h.s3.Endpoint)); err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				var existingList map[string]interface{}
				if json.Unmarshal(body, &existingList) == nil {
					if trains, ok := existingList["trains"].([]interface{}); ok {
						for _, train := range trains {
							if trainMap, ok := train.(map[string]interface{}); ok {
								existingTrains = append(existingTrains, trainMap)
							}
						}
					}
				}
			}
		}
	}

	// Get current active Golang sessions
	sessionKeys, err := h.redis.Keys(ctx, "live_session_*").Result()
	if err != nil {
		fmt.Printf("Error getting session keys for trains list update: %v\n", err)
		return
	}

	// Collect all trains (existing + current Golang sessions)
	allTrains := []map[string]interface{}{}
	processedTrains := make(map[string]bool)
	now := time.Now()

	// First, add trains from current Golang sessions
	for _, sessionKey := range sessionKeys {
		sessionData, err := h.redis.HGetAll(ctx, sessionKey).Result()
		if err != nil || len(sessionData) == 0 {
			continue
		}

		fileName := sessionData["file_path"]
		trainNumber := sessionData["train_number"]
		
		// Skip if we already processed this train
		if processedTrains[trainNumber] {
			continue
		}

		// Get train data from S3
		trainData, err := h.s3.GetTrainData(fileName)
		if err != nil {
			continue
		}

		// Check if data is recent (within last 5 minutes)
		lastUpdate, err := time.Parse(time.RFC3339, trainData.LastUpdate)
		if err != nil {
			continue
		}

		timeSinceUpdate := now.Sub(lastUpdate)
		if timeSinceUpdate <= 5*time.Minute && trainData.DataSource == "live-gps" {
			allTrains = append(allTrains, map[string]interface{}{
				"trainId":        trainData.TrainID,
				"passengerCount": trainData.PassengerCount,
				"lastUpdate":     trainData.LastUpdate,
				"status":         trainData.Status,
			})
			processedTrains[trainNumber] = true
		}
	}

	// Then, add existing trains that are still active and not already processed
	for _, existingTrain := range existingTrains {
		trainId, ok := existingTrain["trainId"].(string)
		if !ok {
			continue
		}

		// Skip if we already processed this train from our sessions
		if processedTrains[trainId] {
			continue
		}

		// Check if the existing train is still active (within 5 minutes)
		lastUpdateStr, ok := existingTrain["lastUpdate"].(string)
		if !ok {
			continue
		}

		lastUpdate, err := time.Parse(time.RFC3339, lastUpdateStr)
		if err != nil {
			continue
		}

		timeSinceUpdate := now.Sub(lastUpdate)
		if timeSinceUpdate <= 5*time.Minute {
			allTrains = append(allTrains, existingTrain)
			processedTrains[trainId] = true
		}
	}

	// Create trains-list.json content with all trains
	trainsListData := map[string]interface{}{
		"trains":      allTrains,
		"total":       len(allTrains),
		"lastUpdated": now.Format(time.RFC3339),
		"source":      "live-tracking-system",
	}

	// Upload to S3
	if err := h.s3.UploadJSON("trains/trains-list.json", trainsListData); err != nil {
		fmt.Printf("Error updating trains-list.json: %v\n", err)
		return
	}

	fmt.Printf("Updated trains-list.json with %d trains (preserving existing trains)\n", len(allTrains))
}

func (h *LiveTrackingHandler) filterActivePassengersAndRecalculate(trainData *models.TrainData) {
	now := time.Now().UnixMilli()
	var activePassengers []models.Passenger

	for _, passenger := range trainData.Passengers {
		timeSinceUpdate := now - passenger.Timestamp
		isMobile := passenger.ClientType == "mobile"
		
		// Mobile: 8 minutes, Web: 2 minutes tolerance
		timeoutMs := int64(120000) // 2 minutes
		if isMobile {
			timeoutMs = 480000 // 8 minutes
		}

		if timeSinceUpdate <= timeoutMs {
			if isMobile && timeSinceUpdate > 120000 {
				passenger.Status = "disconnected"
			}
			activePassengers = append(activePassengers, passenger)
		}
	}

	// Recalculate average position
	if len(activePassengers) > 0 {
		var totalLat, totalLng float64
		activeCount := 0

		for _, passenger := range activePassengers {
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
		}

		trainData.PassengerCount = len(activePassengers)
	}

	trainData.Passengers = activePassengers
}

func (h *LiveTrackingHandler) saveMobileTrip(req struct {
	SessionID           string   `json:"session_id" binding:"required,uuid"`
	SaveTrip            *bool    `json:"save_trip,omitempty"`
	FromStationID       *uint    `json:"from_station_id,omitempty"`
	FromStationName     *string  `json:"from_station_name,omitempty"`
	ToStationID         *uint    `json:"to_station_id,omitempty"`
	ToStationName       *string  `json:"to_station_name,omitempty"`
	EndLat              *float64 `json:"end_lat,omitempty"`
	EndLng              *float64 `json:"end_lng,omitempty"`
	TotalDistanceKm     *float64 `json:"total_distance_km,omitempty"`
	MaxSpeedKmh         *float64 `json:"max_speed_kmh,omitempty"`
	AvgSpeedKmh         *float64 `json:"avg_speed_kmh,omitempty"`
	MaxElevationM       *float64 `json:"max_elevation_m,omitempty"`
	MinElevationM       *float64 `json:"min_elevation_m,omitempty"`
	ElevationGainM      *float64 `json:"elevation_gain_m,omitempty"`
	DurationSeconds     *int     `json:"duration_seconds,omitempty"`
	MaxSpeedLat         *float64 `json:"max_speed_lat,omitempty"`
	MaxSpeedLng         *float64 `json:"max_speed_lng,omitempty"`
	MaxElevationLat     *float64 `json:"max_elevation_lat,omitempty"`
	MaxElevationLng     *float64 `json:"max_elevation_lng,omitempty"`
}, sessionData map[string]string, userTrackingData []models.Passenger, user *models.User) *uint {
	
	if len(userTrackingData) == 0 {
		return nil
	}

	startedAt, _ := time.Parse(time.RFC3339, sessionData["started_at"])
	trainID, _ := strconv.ParseUint(sessionData["train_id"], 10, 32)
	
	startPoint := userTrackingData[0]
	endPoint := userTrackingData[len(userTrackingData)-1]

	endLat := endPoint.Lat
	endLng := endPoint.Lng
	if req.EndLat != nil {
		endLat = *req.EndLat
	}
	if req.EndLng != nil {
		endLng = *req.EndLng
	}

	// Convert tracking data to JSON
	trackingDataJSON, _ := json.Marshal(userTrackingData)
	
	// Extract route coordinates
	var routeCoords []map[string]interface{}
	for _, point := range userTrackingData {
		routeCoords = append(routeCoords, map[string]interface{}{
			"lat": point.Lat,
			"lng": point.Lng,
			"timestamp": point.Timestamp,
		})
	}
	routeCoordsJSON, _ := json.Marshal(routeCoords)

	trip := models.Trip{
		SessionID:           req.SessionID,
		UserID:              &user.ID,
		UserType:            "authenticated",
		TrainID:             uint(trainID),
		TrainName:           sessionData["train_number"],
		TrainNumber:         sessionData["train_number"],
		TotalDistanceKm:     getFloatOrDefault(req.TotalDistanceKm, 0),
		MaxSpeedKmh:         getFloatOrDefault(req.MaxSpeedKmh, 0),
		AvgSpeedKmh:         getFloatOrDefault(req.AvgSpeedKmh, 0),
		MaxElevationM:       getIntOrDefault(req.MaxElevationM, 0),
		MinElevationM:       getIntOrDefault(req.MinElevationM, 0),
		ElevationGainM:      getIntOrDefault(req.ElevationGainM, 0),
		DurationSeconds:     getIntFromIntPtr(req.DurationSeconds, 0),
		StartLatitude:       startPoint.Lat,
		StartLongitude:      startPoint.Lng,
		EndLatitude:         endLat,
		EndLongitude:        endLng,
		MaxSpeedLat:         req.MaxSpeedLat,
		MaxSpeedLng:         req.MaxSpeedLng,
		MaxElevationLat:     req.MaxElevationLat,
		MaxElevationLng:     req.MaxElevationLng,
		FromStationID:       req.FromStationID,
		FromStationName:     req.FromStationName,
		ToStationID:         req.ToStationID,
		ToStationName:       req.ToStationName,
		TrackingData:        string(trackingDataJSON),
		RouteCoordinates:    string(routeCoordsJSON),
		StartedAt:           startedAt,
		CompletedAt:         time.Now(),
	}

	if err := h.db.Create(&trip).Error; err != nil {
		return nil
	}

	return &trip.ID
}

func getFloatOrDefault(val *float64, def float64) float64 {
	if val != nil {
		return *val
	}
	return def
}

func getIntOrDefault(val *float64, def int) int {
	if val != nil {
		return int(*val)
	}
	return def
}

func getIntFromIntPtr(val *int, def int) int {
	if val != nil {
		return *val
	}
	return def
}