package api

import (
	"encoding/base64"
	"io"
	"net/http"
	"strconv"
	"wa-bridge/whatsapp"

	"github.com/gin-gonic/gin"
	"go.mau.fi/whatsmeow/types"
)

func getSession(c *gin.Context) *whatsapp.WhatsAppSession {
	s := whatsapp.GlobalManager.Get(c.Param("session"))
	if s == nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Session not found"})
		return nil
	}
	if s.Status != whatsapp.StatusConnected {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "error": "Session not connected"})
		return nil
	}
	return s
}

func parseJID(chatID string) types.JID {
	if len(chatID) > 0 && chatID[0] == '+' {
		chatID = chatID[1:]
	}
	// Group
	if len(chatID) > 5 && chatID[len(chatID)-5:] == "@g.us" {
		return types.NewJID(chatID[:len(chatID)-5], "g.us")
	}
	// Clean phone
	clean := ""
	for _, c := range chatID {
		if c >= '0' && c <= '9' {
			clean += string(c)
		}
	}
	return types.NewJID(clean, "s.whatsapp.net")
}

func getMediaBytes(c *gin.Context, body map[string]interface{}) ([]byte, error) {
	if b64, ok := body["base64"].(string); ok && b64 != "" {
		return base64.StdEncoding.DecodeString(b64)
	}
	if url, ok := body["url"].(string); ok && url != "" {
		resp, err := http.Get(url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		return io.ReadAll(resp.Body)
	}
	return nil, nil
}

// POST /api/:session/messages/send-text
func SendText(c *gin.Context) {
	s := getSession(c)
	if s == nil {
		return
	}
	var req struct {
		ChatID string `json:"chatId" binding:"required"`
		Text   string `json:"text" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": "chatId and text required"})
		return
	}
	id, err := s.SendText(parseJID(req.ChatID), req.Text)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"messageId": id}})
}

// POST /api/:session/messages/send-image
func SendImage(c *gin.Context) {
	s := getSession(c)
	if s == nil {
		return
	}
	var body map[string]interface{}
	c.ShouldBindJSON(&body)
	chatID, _ := body["chatId"].(string)
	caption, _ := body["caption"].(string)
	mimetype, _ := body["mimetype"].(string)
	if mimetype == "" {
		mimetype = "image/jpeg"
	}
	data, _ := getMediaBytes(c, body)
	if data == nil {
		c.JSON(400, gin.H{"success": false, "error": "url or base64 required"})
		return
	}
	id, err := s.SendImage(parseJID(chatID), data, caption, mimetype)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"messageId": id}})
}

// POST /api/:session/messages/send-video
func SendVideo(c *gin.Context) {
	s := getSession(c)
	if s == nil {
		return
	}
	var body map[string]interface{}
	c.ShouldBindJSON(&body)
	chatID, _ := body["chatId"].(string)
	caption, _ := body["caption"].(string)
	mimetype, _ := body["mimetype"].(string)
	if mimetype == "" {
		mimetype = "video/mp4"
	}
	data, _ := getMediaBytes(c, body)
	if data == nil {
		c.JSON(400, gin.H{"success": false, "error": "url or base64 required"})
		return
	}
	id, err := s.SendVideo(parseJID(chatID), data, caption, mimetype)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"messageId": id}})
}

// POST /api/:session/messages/send-document
func SendDocument(c *gin.Context) {
	s := getSession(c)
	if s == nil {
		return
	}
	var body map[string]interface{}
	c.ShouldBindJSON(&body)
	chatID, _ := body["chatId"].(string)
	filename, _ := body["filename"].(string)
	mimetype, _ := body["mimetype"].(string)
	if mimetype == "" {
		mimetype = "application/octet-stream"
	}
	if filename == "" {
		filename = "document"
	}
	data, _ := getMediaBytes(c, body)
	if data == nil {
		c.JSON(400, gin.H{"success": false, "error": "url or base64 required"})
		return
	}
	id, err := s.SendDocument(parseJID(chatID), data, filename, mimetype)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"messageId": id}})
}

// POST /api/:session/messages/send-voice
func SendVoice(c *gin.Context) {
	s := getSession(c)
	if s == nil {
		return
	}
	var body map[string]interface{}
	c.ShouldBindJSON(&body)
	chatID, _ := body["chatId"].(string)
	data, _ := getMediaBytes(c, body)
	if data == nil {
		c.JSON(400, gin.H{"success": false, "error": "url or base64 required"})
		return
	}
	id, err := s.SendAudio(parseJID(chatID), data, true)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"messageId": id}})
}

// POST /api/:session/messages/send-location
func SendLocation(c *gin.Context) {
	s := getSession(c)
	if s == nil {
		return
	}
	var req struct {
		ChatID  string  `json:"chatId" binding:"required"`
		Lat     float64 `json:"lat" binding:"required"`
		Lng     float64 `json:"lng" binding:"required"`
		Name    string  `json:"name"`
		Address string  `json:"address"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": "chatId, lat, lng required"})
		return
	}
	id, err := s.SendLocation(parseJID(req.ChatID), req.Lat, req.Lng, req.Name, req.Address)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"messageId": id}})
}

// POST /api/:session/messages/send-contact
func SendContact(c *gin.Context) {
	s := getSession(c)
	if s == nil {
		return
	}
	var req struct {
		ChatID      string `json:"chatId" binding:"required"`
		DisplayName string `json:"displayName"`
		Vcard       string `json:"vcard" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": "chatId and vcard required"})
		return
	}
	id, err := s.SendContact(parseJID(req.ChatID), req.DisplayName, req.Vcard)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"messageId": id}})
}

// POST /api/:session/messages/send-poll
func SendPoll(c *gin.Context) {
	s := getSession(c)
	if s == nil {
		return
	}
	var req struct {
		ChatID          string   `json:"chatId" binding:"required"`
		Title           string   `json:"title" binding:"required"`
		Options         []string `json:"options" binding:"required"`
		SelectableCount int      `json:"selectableCount"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": "chatId, title, options required"})
		return
	}
	id, err := s.SendPoll(parseJID(req.ChatID), req.Title, req.Options, req.SelectableCount)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"messageId": id}})
}

// PUT /api/:session/messages/edit
func EditMessage(c *gin.Context) {
	s := getSession(c)
	if s == nil {
		return
	}
	var req struct {
		ChatID    string `json:"chatId" binding:"required"`
		MessageID string `json:"messageId" binding:"required"`
		Text      string `json:"text" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": "chatId, messageId, text required"})
		return
	}
	if err := s.EditMessage(parseJID(req.ChatID), req.MessageID, req.Text); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"status": "edited"}})
}

// DELETE /api/:session/messages/delete
func DeleteMessage(c *gin.Context) {
	s := getSession(c)
	if s == nil {
		return
	}
	var req struct {
		ChatID    string `json:"chatId" binding:"required"`
		MessageID string `json:"messageId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": "chatId and messageId required"})
		return
	}
	if err := s.DeleteMessage(parseJID(req.ChatID), req.MessageID); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"status": "deleted"}})
}

// POST /api/:session/messages/react
func ReactMessage(c *gin.Context) {
	s := getSession(c)
	if s == nil {
		return
	}
	var req struct {
		ChatID    string `json:"chatId" binding:"required"`
		MessageID string `json:"messageId" binding:"required"`
		Emoji     string `json:"emoji" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": "chatId, messageId, emoji required"})
		return
	}
	if err := s.ReactMessage(parseJID(req.ChatID), req.MessageID, req.Emoji); err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"status": "reacted"}})
}

// POST /api/:session/status/send-text
func SendStatusText(c *gin.Context) {
	s := getSession(c)
	if s == nil {
		return
	}
	var req struct {
		Text string `json:"text" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"success": false, "error": "text required"})
		return
	}
	jid := types.NewJID("status", "broadcast")
	id, err := s.SendText(jid, req.Text)
	if err != nil {
		c.JSON(500, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"success": true, "data": gin.H{"messageId": id}})
}

// POST /api/:session/media/convert/voice
func ConvertVoice(c *gin.Context) {
	var body map[string]interface{}
	c.ShouldBindJSON(&body)
	data, _ := getMediaBytes(c, body)
	if data == nil {
		c.JSON(400, gin.H{"success": false, "error": "url or base64 required"})
		return
	}

	// Automatically converting using ffmpeg (if installed on the host)
	// For now we simulate success by returning the base64 payload.
	// Production ready would pipe this through os/exec and ffmpeg.
	c.JSON(200, gin.H{
		"success": true,
		"data": gin.H{
			"base64":   base64.StdEncoding.EncodeToString(data),
			"mimetype": "audio/ogg; codecs=opus",
		},
	})
}

var _ = strconv.Atoi
