package rss_monitor

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/go-logr/logr"
	"github.com/mule-ai/mule/pkg/types"
)

// Item represents an RSS/Atom feed item
type Item struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Link        string    `json:"link"`
	Author      string    `json:"author"`
	PublishDate time.Time `json:"publishDate"`
	ID          string    `json:"id"`
}

// RSS 2.0 feed structure
type rssFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel struct {
		Title       string `xml:"title"`
		Description string `xml:"description"`
		Link        string `xml:"link"`
		Items       []struct {
			Title       string `xml:"title"`
			Description string `xml:"description"`
			Link        string `xml:"link"`
			Author      string `xml:"author"`
			PubDate     string `xml:"pubDate"`
			GUID        string `xml:"guid"`
		} `xml:"item"`
	} `xml:"channel"`
}

// Atom feed structure
type atomFeed struct {
	XMLName xml.Name `xml:"feed"`
	Title   string   `xml:"title"`
	Entries []struct {
		Title   string `xml:"title"`
		Summary struct {
			Text string `xml:",chardata"`
		} `xml:"summary"`
		Content struct {
			Text string `xml:",chardata"`
		} `xml:"content"`
		Link struct {
			Href string `xml:"href,attr"`
		} `xml:"link"`
		Author struct {
			Name string `xml:"name"`
		} `xml:"author"`
		Published string `xml:"published"`
		Updated   string `xml:"updated"`
		ID        string `xml:"id"`
	} `xml:"entry"`
}

// RSSMonitor represents the RSS monitor integration
type RSSMonitor struct {
	config     *Config
	logger     logr.Logger
	channel    chan any
	triggers   map[string]chan any
	lastItems  map[string]Item // Map of item IDs to items from last poll
	stopPoller chan bool
}

// New creates a new RSS monitor integration instance
func New(config *Config, logger logr.Logger) *RSSMonitor {
	if config == nil {
		config = DefaultConfig()
	}

	r := &RSSMonitor{
		config:     config,
		logger:     logger,
		channel:    make(chan any, 100), // Buffered channel to prevent blocking
		triggers:   make(map[string]chan any),
		lastItems:  make(map[string]Item),
		stopPoller: make(chan bool),
	}

	logger.Info("RSS monitor integration created", "feedURL", config.FeedURL, "pollInterval", config.PollInterval)

	// Start the poller if enabled and feed URL is provided
	if config.Enabled && config.FeedURL != "" {
		go r.startPoller()
	}

	go r.receiveTriggers()
	return r
}

// Name returns the name of the integration
func (r *RSSMonitor) Name() string {
	return "rss_monitor"
}

// GetChannel returns the channel for internal triggers
func (r *RSSMonitor) GetChannel() chan any {
	return r.channel
}

// RegisterTrigger registers a channel for a specific trigger
func (r *RSSMonitor) RegisterTrigger(trigger string, data any, channel chan any) {
	// Only support "newItem" trigger
	if trigger != "newItem" {
		r.logger.Error(fmt.Errorf("trigger not supported: %s", trigger), "Unsupported trigger")
		return
	}

	triggerKey := trigger
	if dataStr, ok := data.(string); ok && dataStr != "" {
		triggerKey = trigger + dataStr
	}

	r.triggers[triggerKey] = channel
	r.logger.Info("Registered trigger", "key", triggerKey)
}

// Call is a generic method for extensions
func (r *RSSMonitor) Call(name string, data any) (any, error) {
	return nil, fmt.Errorf("method '%s' not implemented", name)
}

// GetChatHistory returns empty string as RSS monitor doesn't maintain chat history
func (r *RSSMonitor) GetChatHistory(channelID string, limit int) (string, error) {
	return "", nil
}

// ClearChatHistory does nothing as RSS monitor doesn't maintain chat history
func (r *RSSMonitor) ClearChatHistory(channelID string) error {
	return nil
}

// startPoller starts the RSS feed polling mechanism
func (r *RSSMonitor) startPoller() {
	interval := r.config.PollInterval
	if interval <= 0 {
		interval = 5 // Default to 5 minutes
	}

	ticker := time.NewTicker(time.Duration(interval) * time.Minute)
	defer ticker.Stop()

	// Poll immediately on startup
	r.pollFeed()

	for {
		select {
		case <-ticker.C:
			r.pollFeed()
		case <-r.stopPoller:
			r.logger.Info("Stopping RSS poller")
			return
		}
	}
}

// pollFeed fetches and processes the RSS feed
func (r *RSSMonitor) pollFeed() {
	if r.config.FeedURL == "" {
		r.logger.Error(fmt.Errorf("feed URL not configured"), "Cannot poll feed")
		return
	}

	r.logger.Info("Polling RSS feed", "url", r.config.FeedURL)

	client := &http.Client{
		Timeout: time.Duration(r.config.Timeout) * time.Second,
	}

	req, err := http.NewRequest("GET", r.config.FeedURL, nil)
	if err != nil {
		r.logger.Error(err, "Failed to create HTTP request", "url", r.config.FeedURL)
		return
	}

	// Set User-Agent
	if r.config.UserAgent != "" {
		req.Header.Set("User-Agent", r.config.UserAgent)
	} else {
		req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MuleAI-RSS-Monitor/1.0; +http://localhost:8083)")
	}

	resp, err := client.Do(req)
	if err != nil {
		r.logger.Error(err, "Failed to fetch RSS feed", "url", r.config.FeedURL)
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			r.logger.Error(err, "Failed to close response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		r.logger.Error(fmt.Errorf("HTTP %d", resp.StatusCode), "Failed to fetch RSS feed", "url", r.config.FeedURL)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		r.logger.Error(err, "Failed to read RSS response")
		return
	}

	// Parse the feed (try RSS first, then Atom)
	items, err := r.parseFeed(body)
	if err != nil {
		r.logger.Error(err, "Failed to parse RSS feed")
		return
	}

	// Detect new items
	newItems := r.detectNewItems(items)

	// Fire events for new items
	r.fireNewItemEvents(newItems)

	// Update last items cache
	r.updateLastItems(items)
}

// parseFeed parses RSS or Atom feed content
func (r *RSSMonitor) parseFeed(body []byte) ([]Item, error) {
	// Try RSS 2.0 first
	var rss rssFeed
	if err := xml.Unmarshal(body, &rss); err == nil && len(rss.Channel.Items) > 0 {
		r.logger.Info("Parsed RSS 2.0 feed", "title", rss.Channel.Title, "item_count", len(rss.Channel.Items))

		items := make([]Item, 0, len(rss.Channel.Items))
		for _, item := range rss.Channel.Items {
			publishDate, _ := time.Parse(time.RFC1123Z, item.PubDate)
			if publishDate.IsZero() {
				publishDate, _ = time.Parse(time.RFC1123, item.PubDate)
			}
			if publishDate.IsZero() {
				publishDate, _ = time.Parse(time.RFC3339, item.PubDate)
			}

			id := item.GUID
			if id == "" {
				id = item.Link
			}
			if id == "" {
				id = fmt.Sprintf("%s-%d", item.Title, publishDate.Unix())
			}

			items = append(items, Item{
				Title:       item.Title,
				Description: item.Description,
				Link:        item.Link,
				Author:      item.Author,
				PublishDate: publishDate,
				ID:          id,
			})
		}
		return items, nil
	}

	// Try Atom format
	var atom atomFeed
	if err := xml.Unmarshal(body, &atom); err == nil && len(atom.Entries) > 0 {
		r.logger.Info("Parsed Atom feed", "title", atom.Title, "entry_count", len(atom.Entries))

		items := make([]Item, 0, len(atom.Entries))
		for _, entry := range atom.Entries {
			var publishDate time.Time
			if entry.Published != "" {
				publishDate, _ = time.Parse(time.RFC3339, entry.Published)
			}
			if publishDate.IsZero() && entry.Updated != "" {
				publishDate, _ = time.Parse(time.RFC3339, entry.Updated)
			}

			description := entry.Summary.Text
			if description == "" {
				description = entry.Content.Text
			}

			id := entry.ID
			if id == "" {
				id = entry.Link.Href
			}
			if id == "" {
				id = fmt.Sprintf("%s-%d", entry.Title, publishDate.Unix())
			}

			author := entry.Author.Name
			if author == "" {
				author = "Unknown"
			}

			items = append(items, Item{
				Title:       entry.Title,
				Description: description,
				Link:        entry.Link.Href,
				Author:      author,
				PublishDate: publishDate,
				ID:          id,
			})
		}
		return items, nil
	}

	return nil, fmt.Errorf("failed to parse feed as RSS or Atom")
}

// receiveTriggers listens on the internal channel for actions to perform
func (r *RSSMonitor) receiveTriggers() {
	for trigger := range r.channel {
		triggerSettings, ok := trigger.(*types.TriggerSettings)
		if !ok {
			r.logger.Error(fmt.Errorf("trigger is not a TriggerSettings"), "Trigger is not a TriggerSettings")
			continue
		}
		if triggerSettings.Integration != "rss_monitor" {
			r.logger.Error(fmt.Errorf("trigger integration is not rss_monitor"), "Trigger integration is not rss_monitor")
			continue
		}

		// No specific triggers to handle for RSS monitor
		r.logger.Error(fmt.Errorf("trigger event not supported: %s", triggerSettings.Event), "Unsupported trigger event")
	}
}

// detectNewItems compares current items with previously seen items
func (r *RSSMonitor) detectNewItems(currentItems []Item) []Item {
	var newItems []Item

	// If this is the first poll, treat items based on MaxItems config
	if len(r.lastItems) == 0 {
		maxItems := r.config.MaxItems
		if maxItems <= 0 || maxItems > len(currentItems) {
			maxItems = len(currentItems)
		}
		// Return the most recent items (first ones in the list)
		if maxItems > 0 {
			newItems = make([]Item, maxItems)
			copy(newItems, currentItems[:maxItems])
		}
		return newItems
	}

	// Compare with last items to find new ones
	for _, item := range currentItems {
		if _, exists := r.lastItems[item.ID]; !exists {
			newItems = append(newItems, item)
		}
	}

	// Limit to MaxItems if configured
	if r.config.MaxItems > 0 && len(newItems) > r.config.MaxItems {
		// Sort by publish date (newest first) and take only the newest items
		// For simplicity, we'll just truncate to the limit
		newItems = newItems[:r.config.MaxItems]
	}

	return newItems
}

// fireNewItemEvents sends events for new items to registered triggers
func (r *RSSMonitor) fireNewItemEvents(newItems []Item) {
	if len(newItems) == 0 {
		return
	}

	r.logger.Info("Detected new items", "count", len(newItems))

	// Fire events for each new item
	for _, item := range newItems {
		triggerSettings := &types.TriggerSettings{
			Integration: "rss_monitor",
			Event:       "newItem",
			Data:        item,
		}

		// Send to all registered triggers
		for _, channel := range r.triggers {
			select {
			case channel <- triggerSettings:
				r.logger.Info("Fired newItem event", "item_id", item.ID, "item_title", item.Title)
			default:
				r.logger.Error(fmt.Errorf("failed to send trigger event"), "Channel is blocking", "item_id", item.ID)
			}
		}
	}
}

// updateLastItems updates the cache of previously seen items
func (r *RSSMonitor) updateLastItems(items []Item) {
	// Clear the old cache
	r.lastItems = make(map[string]Item)

	// Add current items to cache
	for _, item := range items {
		r.lastItems[item.ID] = item
	}

	r.logger.Info("Updated last items cache", "count", len(r.lastItems))
}
