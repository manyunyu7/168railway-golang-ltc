package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"

	"github.com/modernland/golang-live-tracking/middleware"
)

// SpotterLocation represents a user's location while viewing the map
type SpotterLocation struct {
	UserID     uint    `json:"user_id"`
	Username   string  `json:"username"`
	Name       string  `json:"name"`
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	LastUpdate int64   `json:"last_update"` // Unix milliseconds
	IsActive   bool    `json:"is_active"`
}

// SpottersResponse for API responses
type SpottersResponse struct {
	Spotters    []SpotterLocation `json:"spotters"`
	Total       int               `json:"total"`
	LastUpdated string            `json:"last_updated"`
}

// SpotterHandler handles train spotter location tracking
type SpotterHandler struct {
	db          *gorm.DB
	redis       *redis.Client
	cache       []SpotterLocation
	cacheMutex  sync.RWMutex
	lastCacheUpdate time.Time
}

// NewSpotterHandler creates a new spotter location handler
func NewSpotterHandler(db *gorm.DB, redisClient *redis.Client) *SpotterHandler {
	handler := &SpotterHandler{
		db:    db,
		redis: redisClient,
		cache: make([]SpotterLocation, 0),
	}
	
	// Start cache updater if Redis is available
	if redisClient != nil {
		fmt.Printf("INFO: Starting spotter location cache updater\n")
		go handler.startCacheUpdater()
	}
	
	return handler
}

// UpdateSpotterLocation handles POST /api/spotters/heartbeat
func (h *SpotterHandler) UpdateSpotterLocation(c *gin.Context) {
	user, exists := middleware.GetUserFromContext(c)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Authentication required - please provide Sanctum token",
		})
		return
	}

	var req struct {
		Latitude  float64 `json:"latitude" binding:"required,min=-90,max=90"`
		Longitude float64 `json:"longitude" binding:"required,min=-180,max=180"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	fmt.Printf("DEBUG: User %d updating spotter location: (%.6f, %.6f)\n", 
		user.ID, req.Latitude, req.Longitude)

	// Create spotter location data
	spotter := SpotterLocation{
		UserID:     user.ID,
		Username:   func() string {
			if user.Username != nil {
				return *user.Username
			}
			return user.Name
		}(),
		Name:       user.Name,
		Latitude:   req.Latitude,
		Longitude:  req.Longitude,
		LastUpdate: time.Now().UnixMilli(),
		IsActive:   true,
	}

	// Store in Redis with 5-minute expiration
	if err := h.storeSpotterLocation(spotter); err != nil {
		fmt.Printf("ERROR: Failed to store spotter location: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to update location",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Spotter location updated",
	})
}

// GetActiveSpotters handles GET /api/spotters/active
func (h *SpotterHandler) GetActiveSpotters(c *gin.Context) {
	fmt.Printf("DEBUG: Request for active spotters list\n")
	
	if h.redis == nil {
		// No Redis available, return empty list
		c.JSON(http.StatusOK, SpottersResponse{
			Spotters:    []SpotterLocation{},
			Total:       0,
			LastUpdated: time.Now().Format(time.RFC3339),
		})
		return
	}

	// Return cached spotter list (updated every 30 seconds)
	spotters := h.getCachedSpotters()
	
	// Set CORS headers for frontend access
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	
	c.JSON(http.StatusOK, SpottersResponse{
		Spotters:    spotters,
		Total:       len(spotters),
		LastUpdated: time.Now().Format(time.RFC3339),
	})
	
	fmt.Printf("DEBUG: Returned %d active spotters\n", len(spotters))
}

// storeSpotterLocation stores spotter data in Redis
func (h *SpotterHandler) storeSpotterLocation(spotter SpotterLocation) error {
	if h.redis == nil {
		return fmt.Errorf("Redis not available")
	}
	
	ctx := context.Background()
	key := fmt.Sprintf("spotter_location:%d", spotter.UserID)
	
	data, err := json.Marshal(spotter)
	if err != nil {
		return fmt.Errorf("failed to marshal spotter data: %v", err)
	}
	
	// Store with 5-minute expiration (auto-cleanup)
	if err := h.redis.Set(ctx, key, data, 5*time.Minute).Err(); err != nil {
		return fmt.Errorf("failed to store in Redis: %v", err)
	}
	
	return nil
}

// startCacheUpdater runs background cache updates every 30 seconds
func (h *SpotterHandler) startCacheUpdater() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	fmt.Printf("INFO: Spotter cache updater started (30-second interval)\n")
	
	for {
		select {
		case <-ticker.C:
			h.updateSpotterCache()
		}
	}
}

// updateSpotterCache refreshes the cached spotter list from Redis
func (h *SpotterHandler) updateSpotterCache() {
	if h.redis == nil {
		return
	}
	
	ctx := context.Background()
	
	// Get all spotter location keys
	keys, err := h.redis.Keys(ctx, "spotter_location:*").Result()
	if err != nil {
		fmt.Printf("ERROR: Failed to get spotter keys: %v\n", err)
		return
	}
	
	var spotters []SpotterLocation
	
	// Fetch each spotter's data
	for _, key := range keys {
		data, err := h.redis.Get(ctx, key).Result()
		if err != nil {
			continue // Skip expired or invalid keys
		}
		
		var spotter SpotterLocation
		if err := json.Unmarshal([]byte(data), &spotter); err != nil {
			continue // Skip malformed data
		}
		
		// Only include recent spotters (within 5 minutes)
		if time.Since(time.UnixMilli(spotter.LastUpdate)) <= 5*time.Minute {
			spotters = append(spotters, spotter)
		}
	}
	
	// Update cache with write lock
	h.cacheMutex.Lock()
	h.cache = spotters
	h.lastCacheUpdate = time.Now()
	h.cacheMutex.Unlock()
	
	fmt.Printf("DEBUG: Updated spotter cache with %d active spotters\n", len(spotters))
}

// getCachedSpotters returns the cached spotter list
func (h *SpotterHandler) getCachedSpotters() []SpotterLocation {
	h.cacheMutex.RLock()
	defer h.cacheMutex.RUnlock()
	
	// Return copy of cache to avoid race conditions
	result := make([]SpotterLocation, len(h.cache))
	copy(result, h.cache)
	
	return result
}