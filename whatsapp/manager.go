package whatsapp

import (
	"encoding/json"
	"fmt"
	"sync"
	"wa-bridge/config"
	"wa-bridge/database"
)

type Manager struct {
	sessions map[string]*WhatsAppSession
	mu       sync.RWMutex
	EventCh  chan SessionEvent
}

var GlobalManager *Manager

func NewManager() *Manager {
	m := &Manager{
		sessions: make(map[string]*WhatsAppSession),
		EventCh:  make(chan SessionEvent, 100),
	}
	GlobalManager = m
	return m
}

func (m *Manager) Create(id string, opts map[string]interface{}) (*WhatsAppSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sessions[id]; exists {
		return nil, fmt.Errorf("session '%s' already exists", id)
	}

	session, err := NewSession(id, m.EventCh)
	if err != nil {
		return nil, err
	}

	// Apply filters
	session.IgnoreGroups = getBoolOpt(opts, "ignoreGroups", config.C.IgnoreGroups)
	session.IgnoreBroadcast = getBoolOpt(opts, "ignoreBroadcast", config.C.IgnoreBroadcast)
	session.IgnoreStatus = getBoolOpt(opts, "ignoreStatus", config.C.IgnoreStatus)
	session.IgnoreChannels = getBoolOpt(opts, "ignoreChannels", config.C.IgnoreChannels)
	session.ChatwootInboxID = getStringOpt(opts, "chatwootInboxId", config.C.ChatwootInboxID)

	m.sessions[id] = session

	// Save to DB
	cfgJSON, _ := json.Marshal(opts)
	database.DB.Exec(
		"INSERT OR REPLACE INTO wa_sessions (id, config, status) VALUES (?, ?, ?)",
		id, string(cfgJSON), "connecting",
	)

	// Start session
	if err := session.Start(); err != nil {
		return nil, err
	}

	fmt.Printf("[Manager] Session '%s' created\n", id)
	return session, nil
}

func (m *Manager) Get(id string) *WhatsAppSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

func (m *Manager) List() []map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]map[string]interface{}, 0, len(m.sessions))
	for _, s := range m.sessions {
		result = append(result, s.GetInfo())
	}
	return result
}

func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session, exists := m.sessions[id]
	if !exists {
		return fmt.Errorf("session '%s' not found", id)
	}

	session.Logout()
	delete(m.sessions, id)
	database.DB.Exec("DELETE FROM wa_sessions WHERE id = ?", id)

	fmt.Printf("[Manager] Session '%s' deleted\n", id)
	return nil
}

func (m *Manager) Restart(id string) error {
	m.mu.RLock()
	session, exists := m.sessions[id]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("session '%s' not found", id)
	}
	return session.Restart()
}

func (m *Manager) RestoreAll() {
	rows, err := database.DB.Query("SELECT id, config FROM wa_sessions")
	if err != nil {
		fmt.Println("[Manager] No sessions to restore")
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		var id, cfgStr string
		if err := rows.Scan(&id, &cfgStr); err != nil {
			continue
		}

		var opts map[string]interface{}
		json.Unmarshal([]byte(cfgStr), &opts)

		if _, err := m.Create(id, opts); err != nil {
			fmt.Printf("[Manager] Failed to restore '%s': %v\n", id, err)
		} else {
			count++
		}
	}
	fmt.Printf("[Manager] Restored %d session(s)\n", count)
}

func getBoolOpt(opts map[string]interface{}, key string, fallback bool) bool {
	if opts == nil {
		return fallback
	}
	if v, ok := opts[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return fallback
}

func getStringOpt(opts map[string]interface{}, key string, fallback string) string {
	if opts == nil {
		return fallback
	}
	if v, ok := opts[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return fallback
}
