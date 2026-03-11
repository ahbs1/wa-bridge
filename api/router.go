package api

import (
	"net/http"
	"wa-bridge/auth"
	"wa-bridge/whatsapp"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func SetupRouter(manager *whatsapp.Manager) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// Static files
	r.Static("/public", "./public")
	r.StaticFile("/", "./public/index.html")
	r.StaticFile("/login", "./public/login.html")
	r.StaticFile("/login.html", "./public/login.html")
	r.StaticFile("/style.css", "./public/style.css")
	r.StaticFile("/app.js", "./public/app.js")

	// Auth middleware
	r.Use(auth.Middleware())

	// Health check
	r.GET("/health", func(c *gin.Context) {
		sessions := manager.List()
		connected := 0
		for _, s := range sessions {
			if s["status"] == whatsapp.StatusConnected {
				connected++
			}
		}
		c.JSON(200, gin.H{
			"status": "ok",
			"sessions": gin.H{
				"total":     len(sessions),
				"connected": connected,
			},
		})
	})

	// Auth routes
	authGroup := r.Group("/api/auth")
	{
		authGroup.POST("/login", auth.LoginHandler)
		authGroup.POST("/register", auth.RegisterHandler)
		authGroup.GET("/check", auth.CheckHandler)
	}

	// WebSocket
	r.GET("/ws", func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		Hub.Add(conn)
		// Send current sessions
		conn.WriteJSON(map[string]interface{}{
			"type":     "sessions",
			"sessions": manager.List(),
		})
		// Keep alive
		go func() {
			defer Hub.Remove(conn)
			for {
				if _, _, err := conn.ReadMessage(); err != nil {
					break
				}
			}
		}()
	})

	// Chatwoot webhook (no auth, session specific)
	r.POST("/webhook/chatwoot/:session", ChatwootWebhook)

	// Session routes
	sessGroup := r.Group("/api/sessions")
	{
		sessGroup.POST("", CreateSession)
		sessGroup.GET("", ListSessions)
		sessGroup.GET("/:id", GetSession)
		sessGroup.GET("/:id/qr", GetSessionQR)
		sessGroup.DELETE("/:id", DeleteSession)
		sessGroup.POST("/:id/restart", RestartSession)
	}

	// Message routes
	msgGroup := r.Group("/api/:session/messages")
	{
		msgGroup.POST("/send-text", SendText)
		msgGroup.POST("/send-image", SendImage)
		msgGroup.POST("/send-video", SendVideo)
		msgGroup.POST("/send-document", SendDocument)
		msgGroup.POST("/send-voice", SendVoice)
		msgGroup.POST("/send-location", SendLocation)
		msgGroup.POST("/send-contact", SendContact)
		msgGroup.POST("/send-poll", SendPoll)
		msgGroup.PUT("/edit", EditMessage)
		msgGroup.DELETE("/delete", DeleteMessage)
		msgGroup.POST("/react", ReactMessage)
	}

	// Status routes
	statusGroup := r.Group("/api/:session/status")
	{
		statusGroup.POST("/send-text", SendStatusText)
	}

	// Media routes
	mediaGroup := r.Group("/api/:session/media")
	{
		mediaGroup.POST("/convert/voice", ConvertVoice)
	}

	return r
}
