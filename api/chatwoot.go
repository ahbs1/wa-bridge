package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"wa-bridge/config"
	"wa-bridge/whatsapp"

	"github.com/gin-gonic/gin"
)

// ChatwootWebhook handles incoming webhooks from Chatwoot
func ChatwootWebhook(c *gin.Context) {
	var payload map[string]interface{}
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(400, gin.H{"success": false, "error": "Invalid payload"})
		return
	}

	event, _ := payload["event"].(string)
	msgType, _ := payload["message_type"].(string)

	if event != "message_created" || msgType != "outgoing" {
		c.JSON(200, gin.H{"status": "ignored"})
		return
	}

	// Check if private
	if prv, ok := payload["private"].(bool); ok && prv {
		c.JSON(200, gin.H{"status": "ignored_private"})
		return
	}

	conv, _ := payload["conversation"].(map[string]interface{})
	if conv == nil {
		c.JSON(200, gin.H{"status": "no_conversation"})
		return
	}

	// Extract phone
	phone := ""
	if meta, ok := conv["meta"].(map[string]interface{}); ok {
		if sender, ok := meta["sender"].(map[string]interface{}); ok {
			phone, _ = sender["phone_number"].(string)
		}
	}
	if phone == "" {
		c.JSON(200, gin.H{"status": "no_phone"})
		return
	}

	// Clean phone
	clean := ""
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			clean += string(r)
		}
	}

	// Find connected session
	sessionID := c.Param("session")
	var session *whatsapp.WhatsAppSession
	if sessionID != "" && sessionID != "default" {
		if s := whatsapp.GlobalManager.Get(sessionID); s != nil && s.Status == whatsapp.StatusConnected {
			session = s
		}
	}

	if session == nil {
		for _, info := range whatsapp.GlobalManager.List() {
			if info["status"] == whatsapp.StatusConnected {
				session = whatsapp.GlobalManager.Get(info["id"].(string))
				break
			}
		}
	}

	if session == nil {
		c.JSON(503, gin.H{"success": false, "error": "No connected session"})
		return
	}

	jid := parseJID(clean)
	content, _ := payload["content"].(string)

	// Handle attachments
	if attachments, ok := payload["attachments"].([]interface{}); ok && len(attachments) > 0 {
		for _, att := range attachments {
			attMap, _ := att.(map[string]interface{})
			dataURL, _ := attMap["data_url"].(string)
			fileType, _ := attMap["file_type"].(string)

			resp, err := http.Get(dataURL)
			if err != nil {
				continue
			}
			data, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			switch {
			case fileType == "image" || fileType == "":
				session.SendImage(jid, data, content, "image/jpeg")
			case fileType == "video":
				session.SendVideo(jid, data, content, "video/mp4")
			case fileType == "audio":
				session.SendAudio(jid, data, true)
			default:
				fname, _ := attMap["file_name"].(string)
				session.SendDocument(jid, data, fname, "application/octet-stream")
			}
		}
		c.JSON(200, gin.H{"status": "sent"})
		return
	}

	// Send text
	if content != "" {
		if _, err := session.SendText(jid, content); err != nil {
			c.JSON(500, gin.H{"success": false, "error": err.Error()})
			return
		}
	}

	c.JSON(200, gin.H{"status": "sent"})
}

// ForwardToChatwoot sends a WA message to Chatwoot
func ForwardToChatwoot(sessionID string, inboxID string, data map[string]interface{}) {
	cfg := config.C
	if cfg.ChatwootAPIURL == "" || cfg.ChatwootAPIKey == "" {
		return
	}

	// Use session-specific inbox or fallback to global
	if inboxID == "" {
		inboxID = cfg.ChatwootInboxID
	}
	if inboxID == "" {
		return
	}

	phone, _ := data["from"].(string)
	name, _ := data["name"].(string)
	preview, _ := data["preview"].(string)

	if phone == "" || preview == "" {
		return
	}

	baseURL := fmt.Sprintf("%s/api/v1/accounts/%s", cfg.ChatwootAPIURL, cfg.ChatwootAccountID)

	// Search contact
	contactID := searchOrCreateContact(baseURL, cfg.ChatwootAPIKey, inboxID, phone, name)
	if contactID == 0 {
		return
	}

	// Get or create conversation
	convID := getOrCreateConversation(baseURL, cfg.ChatwootAPIKey, inboxID, contactID)
	if convID == 0 {
		return
	}

	// Check for media
	mediaDataB64, _ := data["mediaData"].(string)
	mediaType, _ := data["mediaType"].(string)
	mimetype, _ := data["mimetype"].(string)
	filename, _ := data["filename"].(string)
	caption, _ := data["caption"].(string)

	if mediaDataB64 != "" && mediaType != "" {
		// Decode base64 media
		mediaBytes, err := base64Decode(mediaDataB64)
		if err != nil {
			// Fallback to text
			sendChatwootMessage(baseURL, cfg.ChatwootAPIKey, convID, preview)
			return
		}

		// Determine filename
		if filename == "" {
			ext := extensionFromMime(mimetype)
			filename = fmt.Sprintf("%s_%s%s", mediaType, phone, ext)
		}

		// Message content = caption or preview
		content := caption
		if content == "" {
			content = preview
		}

		// Send as multipart with attachment
		sendChatwootMediaMessage(baseURL, cfg.ChatwootAPIKey, convID, content, mediaBytes, filename, mimetype)
	} else {
		// Text only
		sendChatwootMessage(baseURL, cfg.ChatwootAPIKey, convID, preview)
	}
}

func searchOrCreateContact(baseURL, apiKey, inboxID, phone, name string) int {
	// Search
	req, _ := http.NewRequest("GET", baseURL+"/contacts/search?q="+phone, nil)
	req.Header.Set("api_access_token", apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	var searchResult map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&searchResult)

	if payload, ok := searchResult["payload"].([]interface{}); ok && len(payload) > 0 {
		if c, ok := payload[0].(map[string]interface{}); ok {
			if id, ok := c["id"].(float64); ok {
				return int(id)
			}
		}
	}

	// Create
	body, _ := json.Marshal(map[string]interface{}{
		"inbox_id":     inboxID,
		"name":         name,
		"phone_number": "+" + phone,
		"identifier":   phone,
	})
	req2, _ := http.NewRequest("POST", baseURL+"/contacts", bytes.NewReader(body))
	req2.Header.Set("api_access_token", apiKey)
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		return 0
	}
	defer resp2.Body.Close()

	var createResult map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&createResult)

	if payload, ok := createResult["payload"].(map[string]interface{}); ok {
		if contact, ok := payload["contact"].(map[string]interface{}); ok {
			if id, ok := contact["id"].(float64); ok {
				return int(id)
			}
		}
	}
	return 0
}

func getOrCreateConversation(baseURL, apiKey, inboxID string, contactID int) int {
	req, _ := http.NewRequest("GET", fmt.Sprintf("%s/contacts/%d/conversations", baseURL, contactID), nil)
	req.Header.Set("api_access_token", apiKey)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if payload, ok := result["payload"].([]interface{}); ok {
		for _, c := range payload {
			if conv, ok := c.(map[string]interface{}); ok {
				status, _ := conv["status"].(string)
				if status == "open" || status == "pending" {
					if id, ok := conv["id"].(float64); ok {
						return int(id)
					}
				}
			}
		}
	}

	// Create conversation
	body, _ := json.Marshal(map[string]interface{}{
		"contact_id": contactID,
		"inbox_id":   inboxID,
		"status":     "open",
	})
	req2, _ := http.NewRequest("POST", baseURL+"/conversations", bytes.NewReader(body))
	req2.Header.Set("api_access_token", apiKey)
	req2.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		return 0
	}
	defer resp2.Body.Close()

	var convResult map[string]interface{}
	json.NewDecoder(resp2.Body).Decode(&convResult)
	if id, ok := convResult["id"].(float64); ok {
		return int(id)
	}
	return 0
}

func sendChatwootMessage(baseURL, apiKey string, convID int, content string) {
	body, _ := json.Marshal(map[string]interface{}{
		"content":      content,
		"message_type": "incoming",
		"private":      false,
	})
	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/conversations/%d/messages", baseURL, convID), bytes.NewReader(body))
	req.Header.Set("api_access_token", apiKey)
	req.Header.Set("Content-Type", "application/json")
	http.DefaultClient.Do(req)
}

func sendChatwootMediaMessage(baseURL, apiKey string, convID int, content string, mediaData []byte, filename, mimetype string) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	// Content field
	w.WriteField("content", content)
	w.WriteField("message_type", "incoming")
	w.WriteField("private", "false")

	// Attachment file
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition", fmt.Sprintf(`form-data; name="attachments[]"; filename="%s"`, filename))
	if mimetype != "" {
		partHeader.Set("Content-Type", mimetype)
	}
	part, err := w.CreatePart(partHeader)
	if err != nil {
		return
	}
	part.Write(mediaData)
	w.Close()

	req, _ := http.NewRequest("POST", fmt.Sprintf("%s/conversations/%d/messages", baseURL, convID), &buf)
	req.Header.Set("api_access_token", apiKey)
	req.Header.Set("Content-Type", w.FormDataContentType())
	http.DefaultClient.Do(req)
}

func base64Decode(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

func extensionFromMime(mime string) string {
	mime = strings.ToLower(mime)
	switch {
	case strings.Contains(mime, "jpeg") || strings.Contains(mime, "jpg"):
		return ".jpg"
	case strings.Contains(mime, "png"):
		return ".png"
	case strings.Contains(mime, "gif"):
		return ".gif"
	case strings.Contains(mime, "webp"):
		return ".webp"
	case strings.Contains(mime, "mp4"):
		return ".mp4"
	case strings.Contains(mime, "ogg"):
		return ".ogg"
	case strings.Contains(mime, "opus"):
		return ".opus"
	case strings.Contains(mime, "mpeg") && strings.Contains(mime, "audio"):
		return ".mp3"
	case strings.Contains(mime, "pdf"):
		return ".pdf"
	case strings.Contains(mime, "doc"):
		return ".docx"
	default:
		return ".bin"
	}
}
