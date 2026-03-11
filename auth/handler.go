package auth

import (
	"net/http"
	"wa-bridge/database"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func RegisterHandler(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Username and password required"})
		return
	}

	// Check if any users exist (only allow if no users yet)
	var count int
	database.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if count > 0 {
		// If users exist, require auth (admin creates users)
		_, exists := c.Get("user_id")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Registration disabled. Login first."})
			return
		}
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to hash password"})
		return
	}

	result, err := database.DB.Exec("INSERT INTO users (username, password) VALUES (?, ?)", req.Username, string(hash))
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": "Username already exists"})
		return
	}

	id, _ := result.LastInsertId()
	token, _ := GenerateToken(id, req.Username)

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"data": gin.H{
			"token":    token,
			"username": req.Username,
		},
	})
}

func LoginHandler(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Username and password required"})
		return
	}

	var id int64
	var hash string
	err := database.DB.QueryRow("SELECT id, password FROM users WHERE username = ?", req.Username).Scan(&id, &hash)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Invalid username or password"})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Invalid username or password"})
		return
	}

	token, err := GenerateToken(id, req.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"token":    token,
			"username": req.Username,
		},
	})
}

func CheckHandler(c *gin.Context) {
	// Check if any user exists
	var count int
	database.DB.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"hasUsers":   count > 0,
		"needsSetup": count == 0,
	})
}
