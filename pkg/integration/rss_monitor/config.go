package rss_monitor

// Config holds the configuration for the RSS monitor integration.
type Config struct {
	Enabled      bool   `json:"enabled,omitempty"`      // Whether the RSS monitor is enabled
	FeedURL      string `json:"feedURL,omitempty"`      // URL of the RSS/Atom feed to monitor
	PollInterval int    `json:"pollInterval,omitempty"` // How often to check for new items (in minutes)
	MaxItems     int    `json:"maxItems,omitempty"`     // Maximum number of items to process per poll
	UserAgent    string `json:"userAgent,omitempty"`    // User agent string for HTTP requests
	Timeout      int    `json:"timeout,omitempty"`      // HTTP request timeout (in seconds)
}

// DefaultConfig returns default RSS monitor configuration.
func DefaultConfig() *Config {
	return &Config{
		Enabled:      true,
		PollInterval: 5,  // Check every 5 minutes
		MaxItems:     10, // Process up to 10 items per poll
		UserAgent:    "Mozilla/5.0 (compatible; MuleAI-RSS-Monitor/1.0; +http://localhost:8083)",
		Timeout:      30, // 30 second timeout
	}
}
