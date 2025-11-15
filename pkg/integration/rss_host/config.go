package rss_host

// Config holds the configuration for the RSS host integration.
type Config struct {
	Enabled     bool   `json:"enabled,omitempty"`     // Whether the RSS host is enabled
	Title       string `json:"title,omitempty"`       // RSS feed title
	Description string `json:"description,omitempty"` // RSS feed description
	Link        string `json:"link,omitempty"`        // RSS feed link
	Author      string `json:"author,omitempty"`      // RSS feed author
	MaxItems    int    `json:"maxItems,omitempty"`    // Maximum number of items to keep in feed
	Path        string `json:"path,omitempty"`        // URL path for RSS feed (default: /rss)
	IndexPath   string `json:"indexPath,omitempty"`   // URL path for web interface (default: /rss-index)
}

// DefaultConfig returns default RSS host configuration.
func DefaultConfig() *Config {
	return &Config{
		Enabled:     true,
		Title:       "Mule RSS Feed",
		Description: "RSS feed hosted by Mule AI",
		Link:        "http://localhost:8083/rss",
		Author:      "Mule AI",
		MaxItems:    100,
		Path:        "/rss",
		IndexPath:   "/rss-index",
	}
}
