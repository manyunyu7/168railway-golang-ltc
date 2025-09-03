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
	UserID           uint    `json:"user_id"`
	Username         string  `json:"username"`
	Name             string  `json:"name"`
	Latitude         float64 `json:"latitude"`
	Longitude        float64 `json:"longitude"`
	LastUpdate       int64   `json:"last_update"` // Unix milliseconds
	IsActive         bool    `json:"is_active"`
	
	// Privacy settings
	HideLocation     bool    `json:"hide_location"`      // Hide location completely from public
	HideIdentity     bool    `json:"hide_identity"`      // Hide username/name, show as anonymous
}

// PublicSpotterLocation for public API responses (respects privacy settings)
type PublicSpotterLocation struct {
	UserID     *uint   `json:"user_id,omitempty"`      // Hidden if identity is hidden
	Username   string  `json:"username"`               // Shows "Anonymous User" if hidden
	Latitude   float64 `json:"latitude"`
	Longitude  float64 `json:"longitude"`
	LastUpdate int64   `json:"last_update"`
	IsActive   bool    `json:"is_active"`
}

// SpottersResponse for public API responses
type SpottersResponse struct {
	Spotters    []PublicSpotterLocation `json:"spotters"`
	Total       int                     `json:"total"`
	LastUpdated string                  `json:"last_updated"`
}

// AdminSpottersResponse for admin API responses (shows all data)
type AdminSpottersResponse struct {
	Spotters    []SpotterLocation `json:"spotters"`
	Total       int               `json:"total"`
	Hidden      int               `json:"hidden_from_public"` // Count of hidden spotters
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
		Latitude     float64 `json:"latitude" binding:"required,min=-90,max=90"`
		Longitude    float64 `json:"longitude" binding:"required,min=-180,max=180"`
		HideLocation bool    `json:"hide_location"` // Hide from public completely
		HideIdentity bool    `json:"hide_identity"` // Show as anonymous
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	fmt.Printf("DEBUG: User %d updating spotter location: (%.6f, %.6f), hide_location: %t, hide_identity: %t\n", 
		user.ID, req.Latitude, req.Longitude, req.HideLocation, req.HideIdentity)

	// Create spotter location data
	spotter := SpotterLocation{
		UserID:       user.ID,
		Username:     func() string {
			if user.Username != nil && *user.Username != "" {
				return *user.Username
			}
			return user.Name // Fallback to name if username is empty
		}(),
		Name:         user.Name,
		Latitude:     req.Latitude,
		Longitude:    req.Longitude,
		LastUpdate:   time.Now().UnixMilli(),
		IsActive:     true,
		HideLocation: req.HideLocation,
		HideIdentity: req.HideIdentity,
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
// Returns filtered results for public users, full results for admins
func (h *SpotterHandler) GetActiveSpotters(c *gin.Context) {
	fmt.Printf("DEBUG: Request for active spotters list\n")
	
	// Check if user is authenticated and has admin role
	user, exists := middleware.GetUserFromContext(c)
	isAdmin := exists && user.Role == "admin"
	
	if isAdmin {
		fmt.Printf("DEBUG: Admin user %d requesting spotter list\n", user.ID)
	}
	
	if h.redis == nil {
		// No Redis available, return empty list
		if isAdmin {
			c.JSON(http.StatusOK, AdminSpottersResponse{
				Spotters:    []SpotterLocation{},
				Total:       0,
				Hidden:      0,
				LastUpdated: time.Now().Format(time.RFC3339),
			})
		} else {
			c.JSON(http.StatusOK, SpottersResponse{
				Spotters:    []PublicSpotterLocation{},
				Total:       0,
				LastUpdated: time.Now().Format(time.RFC3339),
			})
		}
		return
	}

	// Return cached spotter list (updated every 30 seconds)
	spotters := h.getCachedSpotters()
	
	// Set CORS headers for frontend access
	c.Header("Access-Control-Allow-Origin", "*")
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	
	// Admin users get full unfiltered data
	if isAdmin {
		// Count hidden spotters for admin statistics
		hiddenCount := 0
		for _, spotter := range spotters {
			if spotter.HideLocation {
				hiddenCount++
			}
		}
		
		c.JSON(http.StatusOK, AdminSpottersResponse{
			Spotters:    spotters,
			Total:       len(spotters),
			Hidden:      hiddenCount,
			LastUpdated: time.Now().Format(time.RFC3339),
		})
		
		fmt.Printf("DEBUG: Returned %d total spotters to admin (%d hidden from public)\n", len(spotters), hiddenCount)
		return
	}
	
	// Public users get filtered data - respect privacy settings
	var publicSpotters []PublicSpotterLocation
	for _, spotter := range spotters {
		// Skip spotters who hide their location completely
		if spotter.HideLocation {
			continue
		}
		
		publicSpotter := PublicSpotterLocation{
			Latitude:   spotter.Latitude,
			Longitude:  spotter.Longitude,
			LastUpdate: spotter.LastUpdate,
			IsActive:   spotter.IsActive,
		}
		
		// Handle identity privacy
		if spotter.HideIdentity {
			publicSpotter.Username = "Anonymous User"
			// Don't include UserID for anonymous users
		} else {
			publicSpotter.UserID = &spotter.UserID
			publicSpotter.Username = spotter.Username
		}
		
		publicSpotters = append(publicSpotters, publicSpotter)
	}
	
	c.JSON(http.StatusOK, SpottersResponse{
		Spotters:    publicSpotters,
		Total:       len(publicSpotters),
		LastUpdated: time.Now().Format(time.RFC3339),
	})
	
	fmt.Printf("DEBUG: Returned %d public spotters (filtered from %d total)\n", len(publicSpotters), len(spotters))
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