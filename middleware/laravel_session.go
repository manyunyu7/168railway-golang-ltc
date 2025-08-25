package middleware

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	
	"github.com/modernland/golang-live-tracking/models"
)

// UserSession represents the custom user_sessions table structure
type UserSession struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	UserID       uint      `json:"user_id"`
	SessionID    string    `json:"session_id"`
	UserAgent    string    `json:"user_agent"`
	IPAddress    *string   `json:"ip_address"`
	LastActiveAt time.Time `json:"last_active_at"`
	CreatedAt    *time.Time `json:"created_at"`
	UpdatedAt    *time.Time `json:"updated_at"`
}

func (UserSession) TableName() string {
	return "user_sessions"
}

// LaravelSessionAuth validates Laravel session cookies using the user_sessions table
func LaravelSessionAuth(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		fmt.Printf("DEBUG: Attempting Laravel session authentication\n")
		
		// Try multiple common Laravel session cookie names
		sessionCookieNames := []string{
			"laravel_session",
			"168railway_session", 
			"trainradar_session",
			"XSRF-TOKEN",
		}
		
		var sessionID string
		var cookieName string
		
		// Try to find a valid session cookie
		for _, name := range sessionCookieNames {
			if cookie, err := c.Cookie(name); err == nil && cookie != "" {
				// Decode URL-encoded cookie if needed
				if decoded, err := url.QueryUnescape(cookie); err == nil {
					sessionID = decoded
				} else {
					sessionID = cookie
				}
				cookieName = name
				fmt.Printf("DEBUG: Found session cookie '%s': %s...\n", name, sessionID[:min(20, len(sessionID))])
				break
			}
		}
		
		if sessionID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "Laravel session cookie required",
				"expected_cookies": sessionCookieNames,
			})
			c.Abort()
			return
		}

		// Decode Laravel session cookie to get actual session ID
		actualSessionID, err := decodeLaravelSessionCookie(sessionID)
		if err != nil {
			fmt.Printf("DEBUG: Failed to decode session cookie: %v\n", err)
			// Try using the cookie value directly as session ID
			actualSessionID = sessionID
		}

		fmt.Printf("DEBUG: Looking up session ID: %s\n", actualSessionID)

		// Look up session in user_sessions table
		var userSession UserSession
		result := db.Where("session_id = ?", actualSessionID).
			Where("last_active_at > ?", time.Now().Add(-24*time.Hour)). // Active within 24 hours
			First(&userSession)
			
		if result.Error != nil {
			fmt.Printf("DEBUG: Session not found in user_sessions: %v\n", result.Error)
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "Invalid or expired Laravel session",
				"session_id": actualSessionID,
			})
			c.Abort()
			return
		}

		// Get user from database
		var user models.User
		result = db.First(&user, userSession.UserID)
		if result.Error != nil {
			fmt.Printf("DEBUG: User not found for session: %v\n", result.Error)
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "User not found for session",
			})
			c.Abort()
			return
		}

		fmt.Printf("DEBUG: Laravel session authenticated user %d (%s) via cookie '%s'\n", 
			user.ID, user.Name, cookieName)

		// Store user in context
		c.Set("user", user)
		c.Next()
	}
}

// decodeLaravelSessionCookie attempts to decode Laravel session cookie
func decodeLaravelSessionCookie(cookieValue string) (string, error) {
	// Laravel session cookies are typically base64 encoded
	// Format: base64(session_id.payload.signature)
	
	// Try base64 decoding first
	decoded, err := base64.StdEncoding.DecodeString(cookieValue)
	if err == nil {
		decodedStr := string(decoded)
		// Look for session ID pattern in decoded data
		if len(decodedStr) > 10 {
			return decodedStr, nil
		}
	}
	
	// If base64 decode fails or result is too short, try URL decode
	urlDecoded, err := url.QueryUnescape(cookieValue)
	if err == nil && urlDecoded != cookieValue {
		return urlDecoded, nil
	}
	
	// Return original value if no decoding works
	return cookieValue, nil
}

// FlexibleAuth supports both Sanctum tokens and Laravel sessions
func FlexibleAuth(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		fmt.Printf("DEBUG: FlexibleAuth - checking authentication methods\n")
		
		// Try Sanctum first (for mobile apps)
		authHeader := c.GetHeader("Authorization")
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			fmt.Printf("DEBUG: Using Sanctum authentication\n")
			// Use Sanctum authentication
			sanctumAuth := NewAuthMiddleware(db).SanctumAuth()
			sanctumAuth(c)
			return
		}

		// Try Laravel session (for web apps)
		fmt.Printf("DEBUG: Trying Laravel session authentication\n")
		laravelAuth := LaravelSessionAuth(db)
		laravelAuth(c)
	}
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}