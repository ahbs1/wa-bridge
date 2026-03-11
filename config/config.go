package config

import (
	"os"
	"strconv"
)

type Config struct {
	Port      int
	JWTSecret string

	ChatwootAPIURL   string
	ChatwootAPIKey   string
	ChatwootAccountID string
	ChatwootInboxID  string

	IgnoreBroadcast bool
	IgnoreStatus    bool
	IgnoreGroups    bool
	IgnoreChannels  bool
}

var C *Config

func Load() {
	loadEnvFile()
	C = &Config{
		Port:      getEnvInt("PORT", 3000),
		JWTSecret: getEnv("JWT_SECRET", "change_me"),

		ChatwootAPIURL:   getEnv("CHATWOOT_API_URL", ""),
		ChatwootAPIKey:   getEnv("CHATWOOT_API_KEY", ""),
		ChatwootAccountID: getEnv("CHATWOOT_ACCOUNT_ID", "1"),
		ChatwootInboxID:  getEnv("CHATWOOT_INBOX_ID", "1"),

		IgnoreBroadcast: getEnvBool("IGNORE_BROADCAST", true),
		IgnoreStatus:    getEnvBool("IGNORE_STATUS", true),
		IgnoreGroups:    getEnvBool("IGNORE_GROUPS", false),
		IgnoreChannels:  getEnvBool("IGNORE_CHANNELS", true),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		return v == "true" || v == "1" || v == "yes"
	}
	return fallback
}

func loadEnvFile() {
	data, err := os.ReadFile(".env")
	if err != nil {
		return
	}
	lines := splitLines(string(data))
	for _, line := range lines {
		line = trimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		idx := indexOf(line, '=')
		if idx < 0 {
			continue
		}
		key := trimSpace(line[:idx])
		val := trimSpace(line[idx+1:])
		if os.Getenv(key) == "" {
			os.Setenv(key, val)
		}
	}
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
