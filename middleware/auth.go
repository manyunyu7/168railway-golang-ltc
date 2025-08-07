package middleware

import (
	"net/http"
	"strings"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/modernland/golang-live-tracking/models"
)

type AuthMiddleware struct {
	db *gorm.DB
}

func NewAuthMiddleware(db *gorm.DB) *AuthMiddleware {
	return &AuthMiddleware{db: db}
}

// SanctumAuth validates Laravel Sanctum tokens
func (am *AuthMiddleware) SanctumAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "Authentication required",
			})
			c.Abort()
			return
		}

		// Extract token from "Bearer <token>" format
		tokenParts := strings.Split(authHeader, " ")
		if len(tokenParts) != 2 || tokenParts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "Invalid authorization header format",
			})
			c.Abort()
			return
		}

		plainTextToken := tokenParts[1]
		

		// Laravel Sanctum stores the hash of the token part after the "|"
		// Token format: "id|token_string" -> we hash only the "token_string" part
		tokenSegments := strings.SplitN(plainTextToken, "|", 2)
		if len(tokenSegments) != 2 {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "Invalid token format",
			})
			c.Abort()
			return
		}
		
		tokenID := tokenSegments[0]
		tokenString := tokenSegments[1]
		tokenHash := sha256.Sum256([]byte(tokenString))
		hashedToken := fmt.Sprintf("%x", tokenHash)

		// Debug: Log token details
		fmt.Printf("DEBUG: Plain text token: %s\n", plainTextToken)
		fmt.Printf("DEBUG: Token ID: %s\n", tokenID)
		fmt.Printf("DEBUG: Token string (after |): %s\n", tokenString)
		fmt.Printf("DEBUG: Hashed token: %s\n", hashedToken)

		// Find the token in database - Laravel Sanctum matches both ID and hash
		var token models.PersonalAccessToken
		result := am.db.Where("id = ? AND token = ?", tokenID, hashedToken).First(&token)
		if result.Error != nil {
			fmt.Printf("DEBUG: Database query error: %v\n", result.Error)
			
			// Also try to find any token for debugging
			var allTokens []models.PersonalAccessToken
			am.db.Limit(5).Find(&allTokens)
			fmt.Printf("DEBUG: Sample tokens in DB:\n")
			for _, t := range allTokens {
				fmt.Printf("  ID: %d, Token: %s, TokenableID: %d\n", t.ID, t.Token, t.TokenableID)
			}
			
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "Invalid or expired token",
			})
			c.Abort()
			return
		}

		// Check if token is expired
		if token.ExpiresAt != nil && token.ExpiresAt.Before(time.Now()) {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "Token has expired",
			})
			c.Abort()
			return
		}

		// Update last used timestamp
		now := time.Now()
		token.LastUsedAt = &now
		am.db.Save(&token)

		// Get the user
		var user models.User
		if err := am.db.First(&user, token.TokenableID).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "User not found",
			})
			c.Abort()
			return
		}

		// Set user in context
		c.Set("user", user)
		c.Set("user_id", user.ID)
		
		c.Next()
	}
}

// GetUserFromContext retrieves the authenticated user from Gin context
func GetUserFromContext(c *gin.Context) (*models.User, bool) {
	user, exists := c.Get("user")
	if !exists {
		return nil, false
	}
	
	if authUser, ok := user.(models.User); ok {
		return &authUser, true
	}
	
	return nil, false
}

// GetUserIDFromContext retrieves the authenticated user ID from Gin context
func GetUserIDFromContext(c *gin.Context) (uint, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		return 0, false
	}
	
	if id, ok := userID.(uint); ok {
		return id, true
	}
	
	return 0, false
}