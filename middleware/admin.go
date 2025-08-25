package middleware

import (
	"net/http"
	"github.com/gin-gonic/gin"
)

// RequireAdmin checks if the authenticated user has admin role
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get user from context (set by auth middleware)
		user, exists := GetUserFromContext(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"message": "Authentication required",
			})
			c.Abort()
			return
		}

		// Check if user has admin role
		if user.Role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{
				"success": false,
				"message": "Admin access required",
				"error": "You do not have permission to access this resource",
			})
			c.Abort()
			return
		}

		// User is admin, continue
		c.Next()
	}
}