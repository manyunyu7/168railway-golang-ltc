package handlers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/modernland/golang-live-tracking/middleware"
)

type SimpleLiveTrackingHandler struct {
	db *gorm.DB
}

func NewSimpleLiveTrackingHandler(db *gorm.DB) *SimpleLiveTrackingHandler {
	return &SimpleLiveTrackingHandler{
		db: db,
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

	fmt.Printf("DEBUG: User %d requested active session check (Redis-free)\n", user.ID)
	
	c.JSON(http.StatusOK, gin.H{
		"success":           true,
		"has_active_session": false,
		"message":           "Redis-free implementation - always returns no active session",
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
	fmt.Printf("DEBUG: Starting session %s for user %d, train %s\n", sessionID, user.ID, req.TrainNumber)

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"session_id": sessionID,
		"message":    "Mobile tracking session started successfully (Redis-free)",
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

	fmt.Printf("DEBUG: User %d updated location for session %s: (%.6f, %.6f)\n", 
		user.ID, req.SessionID, req.Latitude, req.Longitude)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Mobile location updated successfully (Redis-free)",
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