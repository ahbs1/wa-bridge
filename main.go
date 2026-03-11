package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"wa-bridge/api"
	"wa-bridge/config"
	"wa-bridge/database"
	"wa-bridge/whatsapp"
)

func main() {
	// Load config
	config.Load()

	// Init database
	if err := database.Init(); err != nil {
		fmt.Println("❌ Database error:", err)
		os.Exit(1)
	}
	defer database.Close()

	// Init session manager
	manager := whatsapp.NewManager()

	// Init WebSocket hub
	hub := api.NewWSHub()

	// Event loop: forward WA events → WebSocket
	go func() {
		for evt := range manager.EventCh {
			hub.Broadcast(map[string]interface{}{
				"type":    "event",
				"session": evt.SessionID,
				"event":   evt.Event,
				"data":    evt.Data,
			})

			// Also forward status updates specially
			if evt.Event == "session.status" {
				if data, ok := evt.Data.(map[string]interface{}); ok {
					hub.Broadcast(map[string]interface{}{
						"type":   "session_status",
						"id":     evt.SessionID,
						"status": data["status"],
					})
				}
			}

			// Forward QR
			if evt.Event == "qr" {
				if data, ok := evt.Data.(map[string]string); ok {
					hub.Broadcast(map[string]interface{}{
						"type": "qr",
						"id":   evt.SessionID,
						"qr":   data["qr"],
					})
				}
			}

			// Forward messages to Chatwoot
			if evt.Event == "message.received" {
				if data, ok := evt.Data.(map[string]interface{}); ok {
					// Get session-specific inbox ID
					inboxID := ""
					if s := manager.Get(evt.SessionID); s != nil {
						inboxID = s.ChatwootInboxID
					}
					go api.ForwardToChatwoot(evt.SessionID, inboxID, data)
				}
			}
		}
	}()

	// Setup router
	router := api.SetupRouter(manager)

	// Restore sessions
	go manager.RestoreAll()

	// Banner
	port := config.C.Port
	fmt.Println()
	fmt.Println("╔═══════════════════════════════════════════════════════════╗")
	fmt.Println("║              WA Bridge — GOWS Engine (Go)                ║")
	fmt.Println("╠═══════════════════════════════════════════════════════════╣")
	fmt.Printf("║  🌐 Dashboard:     http://localhost:%d                    ║\n", port)
	fmt.Printf("║  📡 API Base:      http://localhost:%d/api                ║\n", port)
	fmt.Printf("║  🔗 Chatwoot Hook: http://localhost:%d/webhook/chatwoot   ║\n", port)
	fmt.Printf("║  💚 Health:        http://localhost:%d/health              ║\n", port)
	fmt.Println("╠═══════════════════════════════════════════════════════════╣")
	fmt.Println("║  🔒 Auth: JWT (username/password)                        ║")
	fmt.Println("║  ⚡ Engine: GOWS (whatsmeow)                             ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		fmt.Println("\n[Server] Shutting down...")
		os.Exit(0)
	}()

	// Start server
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	if err := router.Run(addr); err != nil {
		fmt.Println("❌ Server error:", err)
		os.Exit(1)
	}
}
