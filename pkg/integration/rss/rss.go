package rss

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/feeds"
	"github.com/mule-ai/mule/pkg/types"
)

// Config holds the configuration for the RSS integration.
type Config struct {
	Enabled     bool   `json:"enabled,omitempty"`
	Title       string `json:"title,omitempty"`       // RSS feed title
	Description string `json:"description,omitempty"` // RSS feed description
	Link        string `json:"link,omitempty"`        // RSS feed link
	Author      string `json:"author,omitempty"`      // RSS feed author
	MaxItems    int    `json:"maxItems,omitempty"`    // Maximum number of items to keep in feed
	Path        string `json:"path,omitempty"`        // URL path for RSS feed (default: /rss)
}

// RSS represents the RSS integration.
type RSS struct {
	config   *Config
	l        logr.Logger
	feed     *feeds.Feed
	items    []*feeds.Item
	mutex    sync.RWMutex
	channel  chan any
	triggers map[string]chan any
}

var (
	events = map[string]struct{}{
		"addItem": {},
	}
)

// New creates a new RSS integration instance.
func New(config *Config, l logr.Logger) *RSS {
	if config == nil {
		config = DefaultConfig()
	}

	// Set defaults
	if config.Title == "" {
		config.Title = "Mule Discord RSS Feed"
	}
	if config.Description == "" {
		config.Description = "RSS feed of Discord messages processed by Mule"
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

	r := &RSS{
		config:   config,
		l:        l,
		items:    make([]*feeds.Item, 0),
		channel:  make(chan any),
		triggers: make(map[string]chan any),
	}

	r.feed = &feeds.Feed{
		Title:       config.Title,
		Link:        &feeds.Link{Href: config.Link},
		Description: config.Description,
		Author:      &feeds.Author{Name: config.Author, Email: ""},
		Created:     time.Now(),
	}

	r.init()
	go r.receiveTriggers()
	return r
}

// DefaultConfig returns default RSS configuration.
func DefaultConfig() *Config {
	return &Config{
		Enabled:     true,
		Title:       "Mule Discord RSS Feed",
		Description: "RSS feed of Discord messages processed by Mule",
		Link:        "http://localhost:8083/rss",
		Author:      "Mule AI",
		MaxItems:    100,
		Path:        "/rss",
	}
}

// init initializes the RSS integration.
func (r *RSS) init() {
	if !r.config.Enabled {
		r.l.Info("RSS integration is disabled")
		return
	}

	r.l.Info("RSS integration initialized - handlers will be registered with main server", "path", r.config.Path, "url", r.config.Link)
}

// HandleRSS serves the RSS feed.
func (r *RSS) HandleRSS(w http.ResponseWriter, req *http.Request) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// Update feed items
	r.feed.Items = r.items

	rss, err := r.feed.ToRss()
	if err != nil {
		r.l.Error(err, "Failed to generate RSS")
		http.Error(w, "Failed to generate RSS", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	if _, err := w.Write([]byte(rss)); err != nil {
		r.l.Error(err, "Failed to write RSS response")
	}
}

// HandleIndex serves a simple index page.
func (r *RSS) HandleIndex(w http.ResponseWriter, req *http.Request) {
	html := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
	<title>%s</title>
</head>
<body>
	<h1>%s</h1>
	<p>%s</p>
	<p><a href="%s">RSS Feed</a></p>
</body>
</html>`, r.config.Title, r.config.Title, r.config.Description, r.config.Path)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := w.Write([]byte(html)); err != nil {
		r.l.Error(err, "Failed to write HTML response")
	}
}

// AddItem adds a new item to the RSS feed.
func (r *RSS) AddItem(title, description, link, author string) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	item := &feeds.Item{
		Title:       title,
		Link:        &feeds.Link{Href: link},
		Description: description,
		Author:      &feeds.Author{Name: author},
		Created:     time.Now(),
		Id:          fmt.Sprintf("%d", time.Now().UnixNano()),
	}

	// Add to beginning of slice
	r.items = append([]*feeds.Item{item}, r.items...)

	// Trim to max items
	if len(r.items) > r.config.MaxItems {
		r.items = r.items[:r.config.MaxItems]
	}

	r.l.Info("Added RSS item", "title", title, "author", author)
}

// Call is a generic method for extensions.
func (r *RSS) Call(name string, data any) (any, error) {
	switch name {
	case "addItem":
		itemData, ok := data.(map[string]string)
		if !ok {
			return nil, fmt.Errorf("invalid data format for addItem")
		}
		r.AddItem(
			itemData["title"],
			itemData["description"],
			itemData["link"],
			itemData["author"],
		)
		return nil, nil
	default:
		return nil, fmt.Errorf("method '%s' not implemented", name)
	}
}

// Name returns the name of the integration.
func (r *RSS) Name() string {
	return "rss"
}

// GetChannel returns the channel for internal triggers.
func (r *RSS) GetChannel() chan any {
	return r.channel
}

// RegisterTrigger registers a channel for a specific trigger.
func (r *RSS) RegisterTrigger(trigger string, data any, channel chan any) {
	_, ok := events[trigger]
	if !ok {
		r.l.Error(fmt.Errorf("trigger not found: %s", trigger), "Trigger not found")
		return
	}

	dataStr, ok := data.(string)
	if !ok && data != nil {
		r.l.Error(fmt.Errorf("trigger data is not a string: %v", data), "Data is not a string")
		return
	}

	triggerKey := trigger
	if dataStr != "" {
		triggerKey = trigger + dataStr
	}

	r.triggers[triggerKey] = channel
	r.l.Info("Registered trigger", "key", triggerKey)
}

// receiveTriggers listens on the internal channel for actions to perform.
func (r *RSS) receiveTriggers() {
	for trigger := range r.channel {
		triggerSettings, ok := trigger.(*types.TriggerSettings)
		if !ok {
			r.l.Error(fmt.Errorf("trigger is not a TriggerSettings"), "Trigger is not a TriggerSettings")
			continue
		}
		if triggerSettings.Integration != "rss" {
			r.l.Error(fmt.Errorf("trigger integration is not rss"), "Trigger integration is not rss")
			continue
		}
		switch triggerSettings.Event {
		case "addItem":
			itemData, ok := triggerSettings.Data.(map[string]string)
			if !ok {
				r.l.Error(fmt.Errorf("trigger data is not a map[string]string"), "Trigger data is not a map[string]string")
				continue
			}
			r.AddItem(
				itemData["title"],
				itemData["description"],
				itemData["link"],
				itemData["author"],
			)
		default:
			r.l.Error(fmt.Errorf("trigger event not supported: %s", triggerSettings.Event), "Unsupported trigger event")
		}
	}
}

// GetChatHistory returns empty string as RSS doesn't maintain chat history
func (r *RSS) GetChatHistory(channelID string, limit int) (string, error) {
	// RSS integration doesn't maintain chat history
	return "", nil
}

// ClearChatHistory does nothing as RSS doesn't maintain chat history
func (r *RSS) ClearChatHistory(channelID string) error {
	// RSS integration doesn't maintain chat history
	return nil
}
