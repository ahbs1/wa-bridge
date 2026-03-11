package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		path := c.Request.URL.Path

		// Skip auth for login, register, static files, health, webhook
		if path == "/api/auth/login" ||
			path == "/api/auth/register" ||
			path == "/health" ||
			strings.HasPrefix(path, "/webhook/") ||
			strings.HasPrefix(path, "/login") ||
			!strings.HasPrefix(path, "/api/") {
			c.Next()
			return
		}

		// Get token from header
		authHeader := c.GetHeader("Authorization")
		tokenStr := ""

		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenStr = authHeader[7:]
		}

		if tokenStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Authorization token required",
			})
			c.Abort()
			return
		}

		claims, err := ValidateToken(tokenStr)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"success": false,
				"error":   "Invalid or expired token",
			})
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Next()
	}
}
