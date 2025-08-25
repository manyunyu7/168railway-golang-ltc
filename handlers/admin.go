package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/modernland/golang-live-tracking/models"
	"github.com/modernland/golang-live-tracking/middleware"
)

type AdminHandler struct {
	db *gorm.DB
}

func NewAdminHandler(db *gorm.DB) *AdminHandler {
	return &AdminHandler{db: db}
}

// GetAllSessions - Admin can view all live tracking sessions
func (h *AdminHandler) GetAllSessions(c *gin.Context) {
	user, _ := middleware.GetUserFromContext(c)
	fmt.Printf("DEBUG: Admin %s (ID: %d) requesting all sessions\n", user.Name, user.ID)

	// Parse query parameters
	status := c.DefaultQuery("status", "active") // active, inactive, terminated, completed, all
	limit := c.DefaultQuery("limit", "50")
	offset := c.DefaultQuery("offset", "0")

	// Convert to integers
	limitInt, _ := strconv.Atoi(limit)
	offsetInt, _ := strconv.Atoi(offset)

	// Build query
	query := h.db.Model(&models.LiveTrackingSession{}).
		Preload("User", func(db *gorm.DB) *gorm.DB {
			return db.Select("id, name, username, station_name")
		})

	// Filter by status
	if status != "all" {
		query = query.Where("status = ?", status)
	}

	// Count total
	var total int64
	query.Count(&total)

	// Get sessions with pagination
	var sessions []models.LiveTrackingSession
	result := query.Order("created_at DESC").
		Limit(limitInt).
		Offset(offsetInt).
		Find(&sessions)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to fetch sessions",
			"error":   result.Error.Error(),
		})
		return
	}

	// Format response
	var formattedSessions []map[string]interface{}
	for _, session := range sessions {
		formattedSession := map[string]interface{}{
			"id":            session.ID,
			"session_id":    session.SessionID,
			"user_id":       session.UserID,
			"user_name":     nil,
			"username":      nil,
			"station_name":  nil,
			"train_number":  session.TrainNumber,
			"status":        session.Status,
			"started_at":    session.StartedAt,
			"last_heartbeat": session.LastHeartbeat,
			"created_at":    session.CreatedAt,
		}

		// Add user info if loaded
		if session.User != nil {
			formattedSession["user_name"] = session.User.Name
			formattedSession["username"] = session.User.Username
			formattedSession["station_name"] = session.User.StationName
		}

		formattedSessions = append(formattedSessions, formattedSession)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"sessions": formattedSessions,
			"total":    total,
			"limit":    limitInt,
			"offset":   offsetInt,
			"status_filter": status,
		},
	})
}

// GetSessionsByUser - Admin can view sessions for a specific user
func (h *AdminHandler) GetSessionsByUser(c *gin.Context) {
	userID := c.Param("user_id")
	admin, _ := middleware.GetUserFromContext(c)

	fmt.Printf("DEBUG: Admin %s requesting sessions for user ID: %s\n", admin.Name, userID)

	userIDInt, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid user ID",
		})
		return
	}

	var sessions []models.LiveTrackingSession
	result := h.db.Where("user_id = ?", uint(userIDInt)).
		Order("created_at DESC").
		Limit(20).
		Find(&sessions)

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to fetch user sessions",
			"error":   result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"user_id":  uint(userIDInt),
			"sessions": sessions,
			"count":    len(sessions),
		},
	})
}

// TerminateSession - Admin can forcefully terminate any session
func (h *AdminHandler) TerminateSession(c *gin.Context) {
	sessionID := c.Param("session_id")
	admin, _ := middleware.GetUserFromContext(c)

	fmt.Printf("DEBUG: Admin %s terminating session: %s\n", admin.Name, sessionID)

	// Find the session
	var session models.LiveTrackingSession
	result := h.db.Where("session_id = ?", sessionID).First(&session)

	if result.Error != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Session not found",
		})
		return
	}

	// Check if already terminated
	if session.Status != "active" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Session is already " + session.Status,
			"current_status": session.Status,
		})
		return
	}

	// Update session status
	result = h.db.Model(&session).Update("status", "terminated")
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to terminate session",
			"error":   result.Error.Error(),
		})
		return
	}

	fmt.Printf("DEBUG: Admin %s successfully terminated session %s for user %d\n", 
		admin.Name, sessionID, session.UserID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Session terminated successfully",
		"data": gin.H{
			"session_id": sessionID,
			"user_id":    session.UserID,
			"train_number": session.TrainNumber,
			"previous_status": "active",
			"new_status": "terminated",
			"terminated_by": admin.ID,
			"terminated_at": time.Now(),
		},
	})
}

// TerminateUserSessions - Admin can terminate all sessions for a user
func (h *AdminHandler) TerminateUserSessions(c *gin.Context) {
	userID := c.Param("user_id")
	admin, _ := middleware.GetUserFromContext(c)

	fmt.Printf("DEBUG: Admin %s terminating all sessions for user ID: %s\n", admin.Name, userID)

	userIDInt, err := strconv.ParseUint(userID, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Invalid user ID",
		})
		return
	}

	// Update all active sessions for this user
	result := h.db.Model(&models.LiveTrackingSession{}).
		Where("user_id = ? AND status = ?", uint(userIDInt), "active").
		Update("status", "terminated")

	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to terminate user sessions",
			"error":   result.Error.Error(),
		})
		return
	}

	fmt.Printf("DEBUG: Admin %s terminated %d sessions for user %d\n", 
		admin.Name, result.RowsAffected, uint(userIDInt))

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "User sessions terminated successfully",
		"data": gin.H{
			"user_id": uint(userIDInt),
			"sessions_terminated": result.RowsAffected,
			"terminated_by": admin.ID,
			"terminated_at": time.Now(),
		},
	})
}

// GetActiveTrainsAdmin - Admin view of active trains with detailed info
func (h *AdminHandler) GetActiveTrainsAdmin(c *gin.Context) {
	admin, _ := middleware.GetUserFromContext(c)
	fmt.Printf("DEBUG: Admin %s requesting active trains overview\n", admin.Name)

	// Get all active sessions grouped by train
	var results []struct {
		TrainNumber    string `json:"train_number"`
		SessionCount   int    `json:"session_count"`
		UserIDs        string `json:"user_ids"`
		LastActivity   *time.Time `json:"last_activity"`
	}

	query := `
		SELECT 
			train_number,
			COUNT(*) as session_count,
			GROUP_CONCAT(user_id) as user_ids,
			MAX(last_heartbeat) as last_activity
		FROM live_tracking_sessions 
		WHERE status = 'active' 
		GROUP BY train_number 
		ORDER BY last_activity DESC
	`

	result := h.db.Raw(query).Scan(&results)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to fetch active trains data",
			"error":   result.Error.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"active_trains": results,
			"total_trains": len(results),
			"requested_by": admin.Name,
			"timestamp": time.Now(),
		},
	})
}