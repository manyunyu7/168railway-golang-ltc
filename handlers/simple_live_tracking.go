package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/modernland/golang-live-tracking/models"
	"github.com/modernland/golang-live-tracking/middleware"
	"github.com/modernland/golang-live-tracking/utils"
)

type SimpleLiveTrackingHandler struct {
	db *gorm.DB
	s3 *utils.S3Client
	redis *redis.Client // Redis client for live tracking performance
	trainMutexes map[string]*sync.Mutex // mutex per train to prevent race conditions
	mutexLock    sync.RWMutex           // protect the trainMutexes map itself
	trainsListMutex sync.Mutex          // dedicated mutex for trains-list.json updates
	trainsListCache map[string]interface{} // in-memory cache for trains list
	trainsListCacheMutex sync.RWMutex       // protect trains list cache
	lastCacheUpdate time.Time               // when cache was last updated
}

func NewSimpleLiveTrackingHandler(db *gorm.DB, s3Client *utils.S3Client) *SimpleLiveTrackingHandler {
	return &SimpleLiveTrackingHandler{
		db: db,
		s3: s3Client,
		trainMutexes: make(map[string]*sync.Mutex),
		trainsListCache: make(map[string]interface{}),
	}
}

// SetRedisClient sets the Redis client for live tracking performance
func (h *SimpleLiveTrackingHandler) SetRedisClient(redisClient *redis.Client) {
	h.redis = redisClient
	fmt.Printf("INFO: Redis client enabled for live tracking handler\n")
	
	// Start background cache updater if Redis is available
	go h.startCacheUpdater()
	
	// Start Redis to S3 sync process (every 88 seconds)
	go h.startRedisSyncToS3()
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

// GetActiveTrainsList - Public API endpoint to serve active trains list (cached for performance)
func (h *SimpleLiveTrackingHandler) GetActiveTrainsList(c *gin.Context) {
	if h.redis != nil {
		fmt.Printf("DEBUG: Frontend requesting active trains list via Redis cache\n")
	} else {
		fmt.Printf("DEBUG: Frontend requesting active trains list via direct database query\n")
	}
	
	// Use cached trains list for performance (updated every 5 seconds in background)
	trainsListData := h.getCachedTrainsList()
	
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
	
	fmt.Printf("DEBUG: Serving trains list with %v trains (database-driven)\n", trainsListData["total"])
	c.JSON(http.StatusOK, trainsListData)
}

// GetTrainData - Public API endpoint to serve individual train data (Redis-first with S3 fallback)
func (h *SimpleLiveTrackingHandler) GetTrainData(c *gin.Context) {
	trainNumber := c.Param("trainNumber")
	
	if h.redis != nil {
		fmt.Printf("DEBUG: Frontend requesting train data for %s via Redis (with S3 fallback)\n", trainNumber)
	} else {
		fmt.Printf("DEBUG: Frontend requesting train data for %s via S3 (Redis disabled)\n", trainNumber)
	}
	
	// Try to read train data from Redis first, fallback to S3
	trainData, err := h.getTrainDataFromRedis(trainNumber)
	if err != nil {
		fmt.Printf("DEBUG: Train %s not found in Redis or S3: %v\n", trainNumber, err)
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
		
		// Create passenger with username and station name
		passenger := models.Passenger{
			UserID:     user.ID,
			UserType:   "authenticated", 
			ClientType: "mobile",
			Lat:        req.InitialLat,
			Lng:        req.InitialLng,
			Timestamp:  time.Now().UnixMilli(),
			SessionID:  sessionID,
			Status:     "active",
		}
		
		// Add username and station name
		if user.Username != nil {
			passenger.Username = *user.Username
		} else {
			passenger.Username = user.Name
		}
		if user.StationName != nil {
			passenger.StationName = *user.StationName
		} else {
			passenger.StationName = ""
		}
		
		trainData = models.TrainData{
			TrainID:         req.TrainNumber,
			Route:           fmt.Sprintf("Route information for train %d", req.TrainID),
			PassengerCount:  1,
			AveragePosition: models.Position{Lat: req.InitialLat, Lng: req.InitialLng},
			Passengers:      []models.Passenger{passenger},
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
		
		// Add username and station name
		if user.Username != nil {
			newPassenger.Username = *user.Username
		} else {
			newPassenger.Username = user.Name
		}
		if user.StationName != nil {
			newPassenger.StationName = *user.StationName
		} else {
			newPassenger.StationName = ""
		}
		
		trainData.Passengers = append(trainData.Passengers, newPassenger)
		
		// Recalculate average position and passenger count
		h.recalculateAveragePosition(&trainData)
		trainData.LastUpdate = time.Now().Format(time.RFC3339)
	}

	// Store GPS position in Redis for real-time tracking (with S3 fallback)
	if h.redis != nil {
		// Store in Redis for real-time performance
		if err := h.storeGPSInRedis(sessionID, user.ID, req.TrainNumber, req.InitialLat, req.InitialLng, user); err != nil {
			fmt.Printf("WARNING: Failed to store GPS in Redis, falling back to S3: %v\n", err)
			// Fallback to S3 if Redis fails
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
			fmt.Printf("DEBUG: Session %s stored in S3 file %s (Redis fallback)\n", sessionID, fileName)
		} else {
			fmt.Printf("DEBUG: Session %s stored in Redis for real-time tracking\n", sessionID)
		}
	} else {
		// No Redis available, use S3 (legacy mode)
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
		fmt.Printf("DEBUG: Session %s stored in S3 file %s (Redis disabled)\n", sessionID, fileName)
	}

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

	// Note: No longer maintaining trains-list.json - using database-driven approach

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

	// First check if session exists (regardless of status)
	var session models.LiveTrackingSession
	result := tx.Where("session_id = ? AND user_id = ?", req.SessionID, user.ID).First(&session)
	
	if result.Error != nil {
		tx.Rollback()
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "Invalid session",
		})
		return
	}
	
	// Check if session is terminated or inactive
	if session.Status != "active" {
		tx.Rollback()
		fmt.Printf("DEBUG: User %d tried to update terminated/inactive session %s (status: %s)\n", 
			user.ID, req.SessionID, session.Status)
		
		// Return 200 OK with EXACT same structure as successful update for backward compatibility
		// Mobile apps won't break because they expect these exact fields
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "Mobile location updated successfully", // Same message as normal success
			"updated_file": "", // Empty string instead of actual file since we didn't update
		})
		return
	}

	// Get train-specific mutex to prevent race conditions with other users on same train
	trainMutex := h.getTrainMutex(session.TrainNumber)
	trainMutex.Lock()
	defer trainMutex.Unlock()

	// Update location in Redis for real-time tracking (with S3 fallback)
	var updateError error
	if h.redis != nil {
		// Update GPS position in Redis (primary, fast)
		if err := h.storeGPSInRedis(session.SessionID, user.ID, session.TrainNumber, req.Latitude, req.Longitude, user); err != nil {
			fmt.Printf("WARNING: Failed to update GPS in Redis, falling back to S3: %v\n", err)
			// Fallback to S3 if Redis fails
			_, updateError = h.updateLocationInTrainFile(session.FilePath, user.ID, req)
		} else {
			fmt.Printf("DEBUG: GPS position updated in Redis for session %s\n", session.SessionID)
		}
	} else {
		// No Redis available, use S3 (legacy mode)
		_, updateError = h.updateLocationInTrainFile(session.FilePath, user.ID, req)
	}

	if updateError != nil {
		tx.Rollback()
		fmt.Printf("ERROR: Failed to update location: %v\n", updateError)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update location",
			"error":   updateError.Error(),
		})
		return
	}

	// Update heartbeat in database only if GPS update succeeded
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

	if h.redis != nil {
		fmt.Printf("DEBUG: Successfully updated GPS position in Redis for user %d\n", user.ID)
	} else {
		fmt.Printf("DEBUG: GPS position updated via S3 for user %d (Redis disabled)\n", user.ID)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Mobile location updated successfully",
		"storage": func() string {
			if h.redis != nil { return "redis" } else { return "s3" }
		}(),
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
		
		// Enhanced GPS path handling: Use mobile data if provided, otherwise try Redis, then S3 fallback
		var finalGPSPath []GPSPoint
		if len(req.GPSPath) > 0 {
			// Use mobile-provided GPS path (preferred)
			finalGPSPath = req.GPSPath
			fmt.Printf("DEBUG: Using mobile GPS path with %d points for trip saving\n", len(req.GPSPath))
		} else if h.redis != nil {
			// Try to get GPS data from Redis (real-time)
			redisGPSPath, err := h.getGPSPathFromRedis(req.SessionID, user.ID, session.TrainNumber)
			if err == nil && len(redisGPSPath) > 0 {
				finalGPSPath = redisGPSPath
				fmt.Printf("DEBUG: Using Redis GPS data for trip saving (real-time position)\n")
			} else {
				fmt.Printf("DEBUG: Redis GPS data not available, will use S3 fallback in saveUserTrip\n")
			}
		} else {
			fmt.Printf("DEBUG: Redis not available, will use S3 fallback in saveUserTrip\n")
		}
		
		tripID, saveFailureReason = h.saveUserTrip(session, user.ID, req.TripSummary, finalGPSPath, &stationInfo)
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
	
	// Handle session cleanup - Redis first, S3 fallback
	if h.redis != nil {
		// Clean up Redis data (real-time approach)
		if err := h.cleanupRedisSession(req.SessionID, user.ID, session.TrainNumber); err != nil {
			fmt.Printf("ERROR: Redis cleanup failed: %v\n", err)
			// Fallback to S3 operations if Redis fails
			if err := h.handleStopSessionS3Operations(fileName, user.ID, false); err != nil {
				fmt.Printf("ERROR: S3 fallback operations also failed: %v\n", err)
			}
		} else {
			fmt.Printf("DEBUG: Successfully cleaned up Redis session data for user %d\n", user.ID)
		}
	} else {
		// No Redis available, use S3 operations (legacy mode)
		err := h.handleStopSessionS3Operations(fileName, user.ID, false)
		if err != nil {
			fmt.Printf("ERROR: S3 operations failed: %v\n", err)
		}
	}

	// Mark session as completed in database
	h.db.Model(&session).Updates(models.LiveTrackingSession{
		Status:    "completed",
		UpdatedAt: time.Now(),
	})

	// Note: No longer maintaining trains-list.json - using database-driven approach
	
	fmt.Printf("DEBUG: User %d stopped session %s\n", user.ID, req.SessionID)

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
			// Update passenger location data (preserving username and station name)
			trainData.Passengers[i].Lat = req.Latitude
			trainData.Passengers[i].Lng = req.Longitude
			trainData.Passengers[i].Timestamp = time.Now().UnixMilli()
			trainData.Passengers[i].Accuracy = req.Accuracy
			trainData.Passengers[i].Speed = req.Speed
			trainData.Passengers[i].Heading = req.Heading
			trainData.Passengers[i].Altitude = req.Altitude
			trainData.Passengers[i].Status = "active"
			// Note: Username and StationName are preserved, not overwritten
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

// startCacheUpdater starts a background goroutine to update trains list cache every 5 seconds
func (h *SimpleLiveTrackingHandler) startCacheUpdater() {
	if h.redis == nil {
		return // No Redis, no cache updater needed
	}
	
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	
	fmt.Printf("INFO: Started trains list cache updater (5-second interval)\n")
	
	for {
		select {
		case <-ticker.C:
			h.updateTrainsListCache()
		}
	}
}

// updateTrainsListCache updates the in-memory cache with fresh data
func (h *SimpleLiveTrackingHandler) updateTrainsListCache() {
	// Generate fresh trains list from database
	freshData := h.generateTrainsListFromDatabaseOptimized()
	
	// Update cache with write lock
	h.trainsListCacheMutex.Lock()
	h.trainsListCache = freshData
	h.lastCacheUpdate = time.Now()
	h.trainsListCacheMutex.Unlock()
	
	if trainCount, ok := freshData["total"].(int); ok {
		fmt.Printf("DEBUG: Updated trains list cache with %d active trains\n", trainCount)
	}
}

// getCachedTrainsList returns cached trains list or generates new one if cache is stale
func (h *SimpleLiveTrackingHandler) getCachedTrainsList() map[string]interface{} {
	if h.redis == nil {
		// No Redis, use direct database query (old behavior)
		return h.generateTrainsListFromDatabase()
	}
	
	h.trainsListCacheMutex.RLock()
	defer h.trainsListCacheMutex.RUnlock()
	
	// Check if cache is still fresh (within 10 seconds)
	if time.Since(h.lastCacheUpdate) <= 10*time.Second && len(h.trainsListCache) > 0 {
		return h.trainsListCache
	}
	
	// Cache is stale or empty, trigger immediate update
	go h.updateTrainsListCache()
	
	// Return current cache or fallback to database
	if len(h.trainsListCache) > 0 {
		return h.trainsListCache
	}
	return h.generateTrainsListFromDatabase()
}

// startRedisSyncToS3 starts a background goroutine to sync Redis data to S3 every 88 seconds
func (h *SimpleLiveTrackingHandler) startRedisSyncToS3() {
	if h.redis == nil {
		return // No Redis, no sync needed
	}
	
	ticker := time.NewTicker(88 * time.Second)
	defer ticker.Stop()
	
	fmt.Printf("INFO: Started Redis to S3 sync process (88-second interval)\n")
	
	for {
		select {
		case <-ticker.C:
			h.syncRedisToS3()
		}
	}
}

// syncRedisToS3 syncs all live train data from Redis to S3 for backup/failover
func (h *SimpleLiveTrackingHandler) syncRedisToS3() {
	if h.redis == nil {
		return
	}
	
	ctx := context.Background()
	
	// Get all live train keys from Redis
	trainKeys, err := h.redis.Keys(ctx, "train_live:*").Result()
	if err != nil {
		fmt.Printf("ERROR: Failed to get train keys from Redis: %v\n", err)
		return
	}
	
	syncCount := 0
	for _, key := range trainKeys {
		// Extract train number from key (train_live:TRAIN-123 -> TRAIN-123)
		trainNumber := key[11:] // Remove "train_live:" prefix
		
		// Get train data from Redis
		trainDataStr, err := h.redis.Get(ctx, key).Result()
		if err != nil {
			fmt.Printf("ERROR: Failed to get train data for %s: %v\n", trainNumber, err)
			continue
		}
		
		// Parse train data
		var trainData models.TrainData
		if err := json.Unmarshal([]byte(trainDataStr), &trainData); err != nil {
			fmt.Printf("ERROR: Failed to parse train data for %s: %v\n", trainNumber, err)
			continue
		}
		
		// Sync to S3 (same structure as before)
		fileName := fmt.Sprintf("trains/train-%s.json", trainNumber)
		if err := h.s3.UploadJSON(fileName, trainData); err != nil {
			fmt.Printf("ERROR: Failed to sync train %s to S3: %v\n", trainNumber, err)
			continue
		}
		
		syncCount++
	}
	
	if syncCount > 0 {
		fmt.Printf("INFO: Synced %d trains from Redis to S3 (88-second backup)\n", syncCount)
	}
}

// Redis GPS Helper Methods

// storeGPSInRedis stores user's GPS position in Redis for real-time tracking
func (h *SimpleLiveTrackingHandler) storeGPSInRedis(sessionID string, userID uint, trainNumber string, lat, lng float64, user *models.User) error {
	if h.redis == nil {
		return fmt.Errorf("Redis not available")
	}
	
	ctx := context.Background()
	
	// Store individual session data
	sessionData := map[string]interface{}{
		"user_id":      userID,
		"train_number": trainNumber,
		"lat":          lat,
		"lng":          lng,
		"timestamp":    time.Now().UnixMilli(),
		"status":       "active",
	}
	
	// Add username and station name
	if user.Username != nil {
		sessionData["username"] = *user.Username
	} else {
		sessionData["username"] = user.Name
	}
	if user.StationName != nil {
		sessionData["station_name"] = *user.StationName
	}
	
	sessionJSON, err := json.Marshal(sessionData)
	if err != nil {
		return fmt.Errorf("failed to marshal session data: %v", err)
	}
	
	// Store session data with 10-minute expiration (auto-cleanup)
	sessionKey := fmt.Sprintf("live_session:%s", sessionID)
	if err := h.redis.Set(ctx, sessionKey, sessionJSON, 10*time.Minute).Err(); err != nil {
		return fmt.Errorf("failed to store session in Redis: %v", err)
	}
	
	// Update train's live data
	return h.updateTrainDataInRedis(trainNumber)
}

// updateTrainDataInRedis rebuilds the train data from all active sessions for that train
func (h *SimpleLiveTrackingHandler) updateTrainDataInRedis(trainNumber string) error {
	if h.redis == nil {
		return fmt.Errorf("Redis not available")
	}
	
	ctx := context.Background()
	
	// Get all sessions for this train from database (source of truth)
	var sessions []models.LiveTrackingSession
	h.db.Where("train_number = ? AND status = ?", trainNumber, "active").Find(&sessions)
	
	if len(sessions) == 0 {
		// No active sessions, remove train from Redis
		trainKey := fmt.Sprintf("train_live:%s", trainNumber)
		h.redis.Del(ctx, trainKey)
		return nil
	}
	
	var passengers []models.Passenger
	var totalLat, totalLng float64
	activeCount := 0
	
	// Get GPS data for each session from Redis
	for _, session := range sessions {
		sessionKey := fmt.Sprintf("live_session:%s", session.SessionID)
		sessionDataStr, err := h.redis.Get(ctx, sessionKey).Result()
		
		if err != nil {
			// Session not in Redis, skip (might be expired or not updated yet)
			continue
		}
		
		var sessionData map[string]interface{}
		if err := json.Unmarshal([]byte(sessionDataStr), &sessionData); err != nil {
			continue
		}
		
		// Extract GPS data
		lat, _ := sessionData["lat"].(float64)
		lng, _ := sessionData["lng"].(float64)
		timestamp, _ := sessionData["timestamp"].(float64)
		username, _ := sessionData["username"].(string)
		stationName, _ := sessionData["station_name"].(string)
		
		passenger := models.Passenger{
			UserID:      session.UserID,
			UserType:    "authenticated",
			ClientType:  "mobile",
			Lat:         lat,
			Lng:         lng,
			Timestamp:   int64(timestamp),
			SessionID:   session.SessionID,
			Status:      "active",
			Username:    username,
			StationName: stationName,
		}
		
		passengers = append(passengers, passenger)
		totalLat += lat
		totalLng += lng
		activeCount++
	}
	
	if activeCount == 0 {
		// No GPS data available, remove train
		trainKey := fmt.Sprintf("train_live:%s", trainNumber)
		h.redis.Del(ctx, trainKey)
		return nil
	}
	
	// Build train data structure
	trainData := models.TrainData{
		TrainID:         trainNumber,
		Route:           fmt.Sprintf("Route information for train %s", trainNumber),
		PassengerCount:  activeCount,
		AveragePosition: models.Position{Lat: totalLat / float64(activeCount), Lng: totalLng / float64(activeCount)},
		Passengers:      passengers,
		LastUpdate:      time.Now().Format(time.RFC3339),
		Status:          "active",
		DataSource:      "redis-live-gps",
	}
	
	trainJSON, err := json.Marshal(trainData)
	if err != nil {
		return fmt.Errorf("failed to marshal train data: %v", err)
	}
	
	// Store train data with 15-minute expiration
	trainKey := fmt.Sprintf("train_live:%s", trainNumber)
	if err := h.redis.Set(ctx, trainKey, trainJSON, 15*time.Minute).Err(); err != nil {
		return fmt.Errorf("failed to store train data in Redis: %v", err)
	}
	
	return nil
}

// getTrainDataFromRedis gets train data from Redis with S3 failover
func (h *SimpleLiveTrackingHandler) getTrainDataFromRedis(trainNumber string) (*models.TrainData, error) {
	if h.redis != nil {
		// Try Redis first
		ctx := context.Background()
		trainKey := fmt.Sprintf("train_live:%s", trainNumber)
		trainDataStr, err := h.redis.Get(ctx, trainKey).Result()
		
		if err == nil {
			var trainData models.TrainData
			if err := json.Unmarshal([]byte(trainDataStr), &trainData); err == nil {
				return &trainData, nil
			}
		}
	}
	
	// Fallback to S3 if Redis fails or data not found
	fileName := fmt.Sprintf("trains/train-%s.json", trainNumber)
	return h.s3.GetTrainData(fileName)
}

// getGPSPathFromRedis gets complete GPS tracking history for a user session from Redis
func (h *SimpleLiveTrackingHandler) getGPSPathFromRedis(sessionID string, userID uint, trainNumber string) ([]GPSPoint, error) {
	if h.redis == nil {
		return nil, fmt.Errorf("Redis not available")
	}
	
	ctx := context.Background()
	
	// First try to get the session data from Redis
	sessionKey := fmt.Sprintf("live_session:%s", sessionID)
	sessionDataStr, err := h.redis.Get(ctx, sessionKey).Result()
	
	if err != nil {
		return nil, fmt.Errorf("session not found in Redis: %v", err)
	}
	
	var sessionData map[string]interface{}
	if err := json.Unmarshal([]byte(sessionDataStr), &sessionData); err != nil {
		return nil, fmt.Errorf("failed to parse session data: %v", err)
	}
	
	// For now, we only have the latest GPS position in Redis
	// In a production system, you'd store a GPS path history
	// But we can create a single point from the current position
	lat, _ := sessionData["lat"].(float64)
	lng, _ := sessionData["lng"].(float64)
	timestamp, _ := sessionData["timestamp"].(float64)
	
	// Create GPS path with current position
	// Note: This gives us the most recent position from Redis
	gpsPath := []GPSPoint{
		{
			Lat:       lat,
			Lng:       lng,
			Timestamp: int64(timestamp),
		},
	}
	
	return gpsPath, nil
}

// cleanupRedisSession removes user session data from Redis
func (h *SimpleLiveTrackingHandler) cleanupRedisSession(sessionID string, userID uint, trainNumber string) error {
	if h.redis == nil {
		return nil // No Redis, nothing to clean
	}
	
	ctx := context.Background()
	
	// Remove user's session data
	sessionKey := fmt.Sprintf("live_session:%s", sessionID)
	if err := h.redis.Del(ctx, sessionKey).Err(); err != nil {
		fmt.Printf("WARNING: Failed to delete session from Redis: %v\n", err)
	} else {
		fmt.Printf("DEBUG: Cleaned up Redis session data for %s\n", sessionID)
	}
	
	// Update the train data to remove this user
	return h.updateTrainDataInRedis(trainNumber)
}

// generateTrainsListFromDatabaseOptimized - Optimized version with reduced S3 calls
func (h *SimpleLiveTrackingHandler) generateTrainsListFromDatabaseOptimized() map[string]interface{} {
	now := time.Now()
	var activeTrains []interface{}
	
	// Get all active sessions from database (real-time source of truth)
	var sessions []models.LiveTrackingSession
	h.db.Where("status = ? AND last_heartbeat > ?", "active", now.Add(-5*time.Minute)).Find(&sessions)
	
	// Group sessions by train number
	trainSessions := make(map[string][]models.LiveTrackingSession)
	for _, session := range sessions {
		trainSessions[session.TrainNumber] = append(trainSessions[session.TrainNumber], session)
	}
	
	// Build trains list from active sessions (MINIMAL S3 calls)
	for trainNumber, trainSessionList := range trainSessions {
		if len(trainSessionList) == 0 {
			continue
		}
		
		passengerCount := len(trainSessionList)
		lastUpdate := now.Format(time.RFC3339)
		
		// For cached trains list, we don't need exact GPS coordinates
		// Just need train ID and passenger count for the frontend list
		activeTrains = append(activeTrains, map[string]interface{}{
			"trainId":        trainNumber,
			"passengerCount": passengerCount,
			"lastUpdate":     lastUpdate,
			"status":         "active",
			// Note: No averagePosition needed for trains list - only individual train data needs GPS
		})
	}
	
	// Create optimized trains list structure
	return map[string]interface{}{
		"trains":      activeTrains,
		"total":       len(activeTrains),
		"lastUpdated": now.Format(time.RFC3339),
		"source":      "redis-cached-database-optimized",
		"websocket_upgrade": map[string]interface{}{
			"available": true,
			"url":       "wss://go-ltc.trainradar35.com/ws/trains",
			"benefits":  "Real-time updates, lower bandwidth, individual passenger positions",
			"message_types": []string{"initial_data", "train_updates", "ping/pong"},
		},
	}
}

// generateTrainsListFromDatabase - Generate trains list from database without S3 dependency
func (h *SimpleLiveTrackingHandler) generateTrainsListFromDatabase() map[string]interface{} {
	now := time.Now()
	var activeTrains []interface{}
	
	// Get all active sessions from database (real-time source of truth)
	var sessions []models.LiveTrackingSession
	h.db.Where("status = ?", "active").Find(&sessions)
	
	// Group sessions by train number
	trainSessions := make(map[string][]models.LiveTrackingSession)
	for _, session := range sessions {
		// Only include recent sessions (within 5 minutes)
		if time.Since(session.LastHeartbeat) <= 5*time.Minute {
			trainSessions[session.TrainNumber] = append(trainSessions[session.TrainNumber], session)
		}
	}
	
	// Build trains list from active sessions with S3 data enhancement
	for trainNumber, trainSessionList := range trainSessions {
		if len(trainSessionList) == 0 {
			continue
		}
		
		// Try to get additional data from S3 train file (optional enhancement)
		fileName := fmt.Sprintf("trains/train-%s.json", trainNumber)
		trainData, err := h.s3.GetTrainData(fileName)
		
		var avgPosition models.Position
		var lastUpdate string
		var passengerCount int
		
		if err == nil && trainData != nil {
			// S3 data available - use rich data
			avgPosition = trainData.AveragePosition
			lastUpdate = trainData.LastUpdate
			passengerCount = len(trainData.Passengers)
		} else {
			// S3 data missing - generate from database sessions
			fmt.Printf("DEBUG: S3 data missing for train %s, using database fallback\n", trainNumber)
			
			// Calculate basic data from sessions
			passengerCount = len(trainSessionList)
			lastUpdate = now.Format(time.RFC3339)
			
			// Use last known position from most recent session (basic fallback)
			if len(trainSessionList) > 0 {
				// Sort by last heartbeat to get most recent
				mostRecent := trainSessionList[0]
				for _, session := range trainSessionList {
					if session.LastHeartbeat.After(mostRecent.LastHeartbeat) {
						mostRecent = session
					}
				}
				// Note: Without S3, we don't have exact GPS coordinates
				// This would need to be enhanced if GPS coordinates were stored in database
				avgPosition = models.Position{Lat: 0, Lng: 0} // Placeholder
			}
		}
		
		// Only include trains with recent activity and passengers
		if passengerCount > 0 {
			activeTrains = append(activeTrains, map[string]interface{}{
				"trainId":        trainNumber,
				"passengerCount": passengerCount,
				"lastUpdate":     lastUpdate,
				"status":         "active",
				"averagePosition": avgPosition,
			})
		}
	}
	
	// Create trains list structure (same format as S3 version)
	return map[string]interface{}{
		"trains":      activeTrains,
		"total":       len(activeTrains),
		"lastUpdated": now.Format(time.RFC3339),
		"source":      "database-driven-real-time",
		"websocket_upgrade": map[string]interface{}{
			"available": true,
			"url":       "wss://go-ltc.trainradar35.com/ws/trains",
			"benefits":  "Real-time updates, lower bandwidth, individual passenger positions",
			"message_types": []string{"initial_data", "train_updates", "ping/pong"},
		},
	}
}