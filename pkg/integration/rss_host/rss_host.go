package rss_host

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/feeds"
	"github.com/mule-ai/mule/pkg/types"
)

// RSSHost represents the RSS host integration
type RSSHost struct {
	config   *Config
	logger   logr.Logger
	feed     *feeds.Feed
	items    []*feeds.Item
	mutex    sync.RWMutex
	channel  chan any
	triggers map[string]chan any
}

// New creates a new RSS host integration instance
func New(config *Config, logger logr.Logger) *RSSHost {
	if config == nil {
		config = DefaultConfig()
	}

	// Set defaults
	if config.Title == "" {
		config.Title = "Mule RSS Feed"
	}
	if config.Description == "" {
		config.Description = "RSS feed hosted by Mule AI"
	}
	if config.Link == "" {
		config.Link = "http://localhost:8083/rss"
	}
	if config.Author == "" {
		config.Author = "Mule AI"
	}
	if config.MaxItems == 0 {
		config.MaxItems = 100
	}
	if config.Path == "" {
		config.Path = "/rss"
	}
	if config.IndexPath == "" {
		config.IndexPath = "/rss-index"
	}

	r := &RSSHost{
		config:   config,
		logger:   logger,
		items:    make([]*feeds.Item, 0),
		channel:  make(chan any, 100), // Buffered channel to prevent blocking
		triggers: make(map[string]chan any),
	}

	r.feed = &feeds.Feed{
		Title:       config.Title,
		Link:        &feeds.Link{Href: config.Link},
		Description: config.Description,
		Author:      &feeds.Author{Name: config.Author, Email: ""},
		Created:     time.Now(),
	}

	logger.Info("RSS host integration created", "title", config.Title)
	go r.receiveTriggers()
	return r
}

// Name returns the name of the integration
func (r *RSSHost) Name() string {
	return "rss_host"
}

// GetChannel returns the channel for internal triggers
func (r *RSSHost) GetChannel() chan any {
	return r.channel
}

// RegisterTrigger registers a channel for a specific trigger
func (r *RSSHost) RegisterTrigger(trigger string, data any, channel chan any) {
	triggerKey := trigger
	if dataStr, ok := data.(string); ok && dataStr != "" {
		triggerKey = trigger + dataStr
	}

	r.triggers[triggerKey] = channel
	r.logger.Info("Registered trigger", "key", triggerKey)
}

// Call is a generic method for extensions
func (r *RSSHost) Call(name string, data any) (any, error) {
	switch name {
	case "addItem":
		// Handle addItem event
		var item *feeds.Item

		// The data might be a string (JSON) or a TriggerSettings object
		switch dataVal := data.(type) {
		case *types.TriggerSettings:
			// Handle TriggerSettings
			switch itemData := dataVal.Data.(type) {
			case *feeds.Item:
				item = itemData
			case string:
				// Try to parse the string as JSON
				var itemMap map[string]interface{}
				if err := json.Unmarshal([]byte(itemData), &itemMap); err != nil {
					r.logger.Error(err, "Failed to parse JSON data for addItem", "data", itemData)
					return nil, fmt.Errorf("failed to parse JSON data: %w", err)
				}

				// Convert the map to a feeds.Item
				item = &feeds.Item{
					Title:       getStringValue(itemMap, "title"),
					Description: getStringValue(itemMap, "description"),
					Link:        &feeds.Link{Href: getStringValue(itemMap, "link")},
					Author:      &feeds.Author{Name: getStringValue(itemMap, "author")},
				}

				// Parse publish date if available
				if publishDateStr, ok := itemMap["publishDate"].(string); ok {
					if publishDate, err := time.Parse(time.RFC3339, publishDateStr); err == nil {
						item.Created = publishDate
					}
				}

				// Set ID if available
				if id, ok := itemMap["id"].(string); ok {
					item.Id = id
				}
			default:
				return nil, fmt.Errorf("unsupported data type for addItem: %T", dataVal.Data)
			}
		case string:
			// Handle JSON string directly
			var itemMap map[string]interface{}
			if err := json.Unmarshal([]byte(dataVal), &itemMap); err != nil {
				r.logger.Error(err, "Failed to parse JSON data for addItem", "data", dataVal)
				return nil, fmt.Errorf("failed to parse JSON data: %w", err)
			}

			// Convert the map to a feeds.Item
			item = &feeds.Item{
				Title:       getStringValue(itemMap, "title"),
				Description: getStringValue(itemMap, "description"),
				Link:        &feeds.Link{Href: getStringValue(itemMap, "link")},
				Author:      &feeds.Author{Name: getStringValue(itemMap, "author")},
			}

			// Parse publish date if available
			if publishDateStr, ok := itemMap["publishDate"].(string); ok {
				if publishDate, err := time.Parse(time.RFC3339, publishDateStr); err == nil {
					item.Created = publishDate
				}
			}

			// Set ID if available
			if id, ok := itemMap["id"].(string); ok {
				item.Id = id
			}
		default:
			return nil, fmt.Errorf("unsupported data type for addItem: %T", data)
		}

		r.AddItem(item)
		return "Item added successfully", nil // Return success message for workflow compatibility
	default:
		return nil, fmt.Errorf("method '%s' not implemented", name)
	}
}

// GetChatHistory returns empty string as RSS host doesn't maintain chat history
func (r *RSSHost) GetChatHistory(channelID string, limit int) (string, error) {
	// RSS host doesn't maintain chat history
	return "", nil
}

// ClearChatHistory does nothing as RSS host doesn't maintain chat history
func (r *RSSHost) ClearChatHistory(channelID string) error {
	// RSS host doesn't maintain chat history
	return nil
}

// AddItem adds a new item to the RSS feed
func (r *RSSHost) AddItem(item *feeds.Item) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	// Add the new item at the beginning (most recent first)
	r.items = append([]*feeds.Item{item}, r.items...)

	// Limit to MaxItems
	if len(r.items) > r.config.MaxItems {
		r.items = r.items[:r.config.MaxItems]
	}

	r.logger.Info("Added item to RSS feed", "title", item.Title, "item_count", len(r.items))
}

// GetItems returns the current items in the feed
func (r *RSSHost) GetItems() []*feeds.Item {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// Return a copy to prevent external modification
	items := make([]*feeds.Item, len(r.items))
	copy(items, r.items)
	return items
}

// HandleRSS serves the RSS feed
func (r *RSSHost) HandleRSS(w http.ResponseWriter, req *http.Request) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// Update feed items
	r.feed.Items = r.items

	rss, err := r.feed.ToRss()
	if err != nil {
		r.logger.Error(err, "Failed to generate RSS")
		http.Error(w, "Failed to generate RSS", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/rss+xml")
	if _, err := w.Write([]byte(rss)); err != nil {
		r.logger.Error(err, "Failed to write RSS response")
	}
}

// HandleIndex serves a simple index page
func (r *RSSHost) HandleIndex(w http.ResponseWriter, req *http.Request) {
	// Get items to display
	items := r.GetItems()

	// Build items list HTML
	itemsHTML := ""
	if len(items) > 0 {
		itemsHTML = "<h2>Recent Items</h2><ul>"
		for _, item := range items {
			itemsHTML += fmt.Sprintf("<li><a href=\"%s\">%s</a></li>", item.Link.Href, item.Title)
		}
		itemsHTML += "</ul>"
	} else {
		itemsHTML = "<p>No items available.</p>"
	}

	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
	<title>%s</title>
</head>
<body>
	<h1>%s</h1>
	<p>%s</p>
	%s
	<p><a href="%s">RSS Feed</a></p>
	<p><a href="%s-atom">Atom Feed</a></p>
</body>
</html>`, r.config.Title, r.config.Title, r.config.Description, itemsHTML, r.config.Path, r.config.Path)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte(html)); err != nil {
		r.logger.Error(err, "Failed to write HTML response")
	}
}

// HandleAtom serves the Atom feed
func (r *RSSHost) HandleAtom(w http.ResponseWriter, req *http.Request) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// Update feed items
	r.feed.Items = r.items

	atom, err := r.feed.ToAtom()
	if err != nil {
		r.logger.Error(err, "Failed to generate Atom")
		http.Error(w, "Failed to generate Atom", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/atom+xml")
	if _, err := w.Write([]byte(atom)); err != nil {
		r.logger.Error(err, "Failed to write Atom response")
	}
}

// receiveTriggers listens on the internal channel for actions to perform
func (r *RSSHost) receiveTriggers() {
	for trigger := range r.channel {
		triggerSettings, ok := trigger.(*types.TriggerSettings)
		if !ok {
			r.logger.Error(fmt.Errorf("trigger is not a TriggerSettings"), "Trigger is not a TriggerSettings")
			continue
		}
		if triggerSettings.Integration != "rss_host" {
			r.logger.Error(fmt.Errorf("trigger integration is not rss_host"), "Trigger integration is not rss_host")
			continue
		}

		switch triggerSettings.Event {
		case "addItem":
			item, ok := triggerSettings.Data.(*feeds.Item)
			if !ok {
				r.logger.Error(fmt.Errorf("trigger data is not a feeds.Item"), "Trigger data is not a feeds.Item")
				continue
			}
			r.AddItem(item)
		default:
			r.logger.Error(fmt.Errorf("trigger event not supported: %s", triggerSettings.Event), "Unsupported trigger event")
		}
	}
}

// getStringValue extracts a string value from a map, returning empty string if not found or not a string
func getStringValue(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}
