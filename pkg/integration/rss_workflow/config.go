package rss_workflow

import "time"

// Config holds the configuration for the RSS workflow integration.
type Config struct {
	Enabled    bool          `json:"enabled,omitempty"`    // Whether the RSS workflow is enabled
	CacheTTL   time.Duration `json:"cacheTTL,omitempty"`   // Cache TTL duration (default: 6 hours)
	SearxngURL string        `json:"searxngURL,omitempty"` // Searxng URL for search queries
	AgentID    int           `json:"agentID,omitempty"`    // Agent ID to use for enhancements
}

// DefaultConfig returns default RSS workflow configuration.
func DefaultConfig() *Config {
	return &Config{
		Enabled:  true,
		CacheTTL: 6 * time.Hour, // 6 hours
	}
}
