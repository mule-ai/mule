package config

import (
	"fmt"
	"os"
	"strconv"
)

// Config holds the application configuration
type Config struct {
	ServerPort     int
	SupabaseURL    string
	SupabaseKey    string
	MuleAPIURL     string
	MuleAPIToken   string
	WebhookSecret  string
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{}

	// Server configuration
	portStr := getEnv("SERVER_PORT", "8080")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("invalid SERVER_PORT: %w", err)
	}
	cfg.ServerPort = port

	// Supabase configuration
	cfg.SupabaseURL = getEnv("SUPABASE_URL", "")
	cfg.SupabaseKey = getEnv("SUPABASE_KEY", "")
	
	// Mule API configuration
	cfg.MuleAPIURL = getEnv("MULE_API_URL", "/v1/chat/completions")
	cfg.MuleAPIToken = getEnv("MULE_API_TOKEN", "")
	
	// Webhook configuration
	cfg.WebhookSecret = getEnv("WEBHOOK_SECRET", "")

	// Validate required configuration
	if cfg.SupabaseURL == "" {
		return nil, fmt.Errorf("SUPABASE_URL is required")
	}
	if cfg.SupabaseKey == "" {
		return nil, fmt.Errorf("SUPABASE_KEY is required")
	}

	return cfg, nil
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}