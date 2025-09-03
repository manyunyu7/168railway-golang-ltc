package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/modernland/golang-live-tracking/models"
)

type LaravelSessionMiddleware struct {
	db *gorm.DB
}

func NewLaravelSessionMiddleware(db *gorm.DB) *LaravelSessionMiddleware {
	return &LaravelSessionMiddleware{
		db: db,
	}
}

// AuthOrSession provides flexible authentication (Sanctum OR Laravel session OR anonymous)
func (m *LaravelSessionMiddleware) AuthOrSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try Sanctum authentication first (for mobile users)
		if authHeader := c.GetHeader("Authorization"); authHeader != "" {
			// Let Sanctum middleware handle this
			c.Next()
			return
		}

		// Try Laravel session authentication (for web users)
		user := m.validateLaravelSession(c)
		if user != nil {
			// Valid Laravel session
			c.Set("user", user)
			c.Set("user_type", "web_authenticated")
			c.Set("auth_method", "laravel_session")
			fmt.Printf("DEBUG: Web user authenticated via Laravel session: %d (%s)\n", user.ID, user.Name)
			c.Next()
			return
		}

		// No authentication - allow as anonymous web user with rate limiting
		c.Set("user_type", "web_anonymous") 
		c.Set("auth_method", "none")
		c.Set("rate_limit_key", c.ClientIP()) // Use IP for rate limiting
		fmt.Printf("DEBUG: Anonymous web user allowed from IP: %s\n", c.ClientIP())
		c.Next()
	}
}

// RequireAuthOrSession requires either Sanctum token OR Laravel session
func (m *LaravelSessionMiddleware) RequireAuthOrSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Try Sanctum first
		if authHeader := c.GetHeader("Authorization"); authHeader != "" {
			// Let Sanctum middleware handle validation
			c.Next()
			return
		}

		// Try Laravel session
		user := m.validateLaravelSession(c)
		if user != nil {
			c.Set("user", user)
			c.Set("user_type", "web_authenticated")
			c.Set("auth_method", "laravel_session")
			c.Next()
			return
		}

		// No valid authentication
		c.JSON(http.StatusUnauthorized, gin.H{
			"success": false,
			"message": "Authentication required - please login via mobile app or web",
		})
		c.Abort()
	}
}

// validateLaravelSession validates Laravel session cookie and returns user
func (m *LaravelSessionMiddleware) validateLaravelSession(c *gin.Context) *models.User {
	// Get all cookies and find Laravel session
	cookies := c.Request.Header.Get("Cookie")
	if cookies == "" {
		return nil
	}

	// Parse cookies to find Laravel session ID
	sessionID := m.extractSessionIDFromCookies(cookies)
	if sessionID == "" {
		return nil
	}

	fmt.Printf("DEBUG: Found Laravel session ID: %s\n", sessionID[:min(len(sessionID), 20)]+"...")

	// Look up session in database
	var session models.LaravelSession
	if err := m.db.Where("id = ?", sessionID).First(&session).Error; err != nil {
		fmt.Printf("DEBUG: Laravel session not found: %v\n", err)
		return nil
	}

	// Check if session is still active (within last 2 hours)
	lastActivity := time.Unix(session.LastActivity, 0)
	if time.Since(lastActivity) > 2*time.Hour {
		fmt.Printf("DEBUG: Laravel session expired (last activity: %v)\n", lastActivity)
		return nil
	}

	// If session has direct user_id, use it
	if session.UserID != nil {
		var user models.User
		if err := m.db.First(&user, *session.UserID).Error; err != nil {
			fmt.Printf("DEBUG: User %d from session not found: %v\n", *session.UserID, err)
			return nil
		}
		return &user
	}

	// Parse Laravel session payload to extract user login
	userID := m.parseUserFromPayload(session.Payload)
	if userID == nil {
		fmt.Printf("DEBUG: No user login found in Laravel session payload\n")
		return nil
	}

	// Fetch user from database
	var user models.User
	if err := m.db.First(&user, *userID).Error; err != nil {
		fmt.Printf("DEBUG: User %d from session payload not found: %v\n", *userID, err)
		return nil
	}

	return &user
}

// extractSessionIDFromCookies extracts Laravel session ID from cookies
func (m *LaravelSessionMiddleware) extractSessionIDFromCookies(cookies string) string {
	// Common Laravel session cookie names
	sessionNames := []string{
		"laravel_session",
		"168railway_session", // Custom session name
		"trainradar35_session",
	}

	// Parse cookie string
	for _, cookie := range strings.Split(cookies, ";") {
		cookie = strings.TrimSpace(cookie)
		parts := strings.SplitN(cookie, "=", 2)
		if len(parts) != 2 {
			continue
		}

		cookieName := strings.TrimSpace(parts[0])
		cookieValue := strings.TrimSpace(parts[1])

		// Check if this is a Laravel session cookie
		for _, sessionName := range sessionNames {
			if cookieName == sessionName {
				return cookieValue
			}
		}
	}

	return ""
}

// parseUserFromPayload extracts user ID from Laravel session payload
func (m *LaravelSessionMiddleware) parseUserFromPayload(payload string) *uint {
	// Laravel session payload is usually serialized PHP data or JSON
	// Look for user login patterns
	
	// Try to parse as JSON first
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &data); err == nil {
		// Look for Laravel Auth session keys
		authKeys := []string{
			"login_web_59ba36addc2b2f9401580f014c7f58ea4e30989d", // Default Laravel auth key
			"auth_user_id",
			"user_id", 
			"authenticated_user",
		}
		
		for _, key := range authKeys {
			if userID, exists := data[key]; exists {
				if id, ok := userID.(float64); ok {
					userIDUint := uint(id)
					return &userIDUint
				}
			}
		}
	}

	// Fallback: Simple string parsing for Laravel session format
	// Look for user ID patterns in serialized data
	if strings.Contains(payload, "login_web_") {
		// Laravel stores user login as: s:66:"login_web_59ba36addc2b2f9401580f014c7f58ea4e30989d";i:123;
		// This is a simplified parser - production would need proper PHP serialization parsing
		patterns := []string{
			`login_web_59ba36addc2b2f9401580f014c7f58ea4e30989d";i:`,
			`"auth_user_id";i:`,
			`"user_id";i:`,
		}

		for _, pattern := range patterns {
			if idx := strings.Index(payload, pattern); idx != -1 {
				start := idx + len(pattern)
				end := strings.Index(payload[start:], ";")
				if end != -1 {
					userIDStr := payload[start : start+end]
					if userID := parseUint(userIDStr); userID != nil {
						return userID
					}
				}
			}
		}
	}

	return nil
}

// parseUint safely parses string to uint
func parseUint(s string) *uint {
	var id uint
	if _, err := fmt.Sscanf(s, "%d", &id); err == nil {
		return &id
	}
	return nil
}

// min returns minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}