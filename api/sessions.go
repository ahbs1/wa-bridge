package api

import (
	"net/http"
	"wa-bridge/whatsapp"

	"github.com/gin-gonic/gin"
)

// POST /api/sessions
func CreateSession(c *gin.Context) {
	var req struct {
		ID              string `json:"id" binding:"required"`
		IgnoreGroups    *bool  `json:"ignoreGroups"`
		IgnoreBroadcast *bool  `json:"ignoreBroadcast"`
		IgnoreStatus    *bool  `json:"ignoreStatus"`
		IgnoreChannels  *bool  `json:"ignoreChannels"`
		ChatwootInboxID string `json:"chatwootInboxId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Session ID required"})
		return
	}

	opts := map[string]interface{}{}
	if req.IgnoreGroups != nil {
		opts["ignoreGroups"] = *req.IgnoreGroups
	}
	if req.IgnoreBroadcast != nil {
		opts["ignoreBroadcast"] = *req.IgnoreBroadcast
	}
	if req.IgnoreStatus != nil {
		opts["ignoreStatus"] = *req.IgnoreStatus
	}
	if req.IgnoreChannels != nil {
		opts["ignoreChannels"] = *req.IgnoreChannels
	}
	if req.ChatwootInboxID != "" {
		opts["chatwootInboxId"] = req.ChatwootInboxID
	}

	session, err := whatsapp.GlobalManager.Create(req.ID, opts)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": session.GetInfo()})
}

// GET /api/sessions
func ListSessions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"success": true, "data": whatsapp.GlobalManager.List()})
}

// GET /api/sessions/:id
func GetSession(c *gin.Context) {
	session := whatsapp.GlobalManager.Get(c.Param("id"))
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Session not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": session.GetInfo()})
}

// GET /api/sessions/:id/qr
func GetSessionQR(c *gin.Context) {
	session := whatsapp.GlobalManager.Get(c.Param("id"))
	if session == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Session not found"})
		return
	}
	info := session.GetInfo()
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"qr": info["qr"], "status": info["status"]}})
}

// DELETE /api/sessions/:id
func DeleteSession(c *gin.Context) {
	if err := whatsapp.GlobalManager.Delete(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "Session deleted"}})
}

// POST /api/sessions/:id/restart
func RestartSession(c *gin.Context) {
	if err := whatsapp.GlobalManager.Restart(c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"message": "Session restarted"}})
}
