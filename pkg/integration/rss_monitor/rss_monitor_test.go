package rss_monitor

import (
	"testing"

	"github.com/go-logr/logr"
)

func TestConfigDefaults(t *testing.T) {
	config := DefaultConfig()

	if !config.Enabled {
		t.Error("Expected Enabled to be true by default")
	}

	if config.PollInterval != 5 {
		t.Errorf("Expected PollInterval to be 5, got %d", config.PollInterval)
	}

	if config.MaxItems != 10 {
		t.Errorf("Expected MaxItems to be 10, got %d", config.MaxItems)
	}

	if config.UserAgent == "" {
		t.Error("Expected UserAgent to be set by default")
	}

	if config.Timeout != 30 {
		t.Errorf("Expected Timeout to be 30, got %d", config.Timeout)
	}
}

func TestNewRSSMonitor(t *testing.T) {
	config := &Config{
		FeedURL:      "http://example.com/rss",
		PollInterval: 1,
	}

	monitor := New(config, logr.Discard())

	if monitor == nil {
		t.Fatal("Expected New to return a non-nil RSSMonitor")
	}

	if monitor.Name() != "rss_monitor" {
		t.Errorf("Expected Name to return 'rss_monitor', got '%s'", monitor.Name())
	}

	if monitor.GetChannel() == nil {
		t.Error("Expected GetChannel to return a non-nil channel")
	}
}

func TestDetectNewItems(t *testing.T) {
	monitor := &RSSMonitor{
		config:    DefaultConfig(),
		lastItems: make(map[string]Item),
	}

	// Test with empty lastItems (first poll)
	currentItems := []Item{
		{ID: "1", Title: "Item 1"},
		{ID: "2", Title: "Item 2"},
		{ID: "3", Title: "Item 3"},
	}

	newItems := monitor.detectNewItems(currentItems)

	// Should return up to MaxItems items
	if len(newItems) > monitor.config.MaxItems {
		t.Errorf("Expected at most %d items, got %d", monitor.config.MaxItems, len(newItems))
	}

	// Update lastItems
	monitor.updateLastItems(currentItems)

	// Test with existing lastItems
	newCurrentItems := []Item{
		{ID: "1", Title: "Item 1"}, // Existing
		{ID: "2", Title: "Item 2"}, // Existing
		{ID: "4", Title: "Item 4"}, // New
	}

	newItems = monitor.detectNewItems(newCurrentItems)

	if len(newItems) != 1 {
		t.Errorf("Expected 1 new item, got %d", len(newItems))
	}

	if len(newItems) > 0 && newItems[0].ID != "4" {
		t.Errorf("Expected new item with ID '4', got '%s'", newItems[0].ID)
	}
}

func TestUpdateLastItems(t *testing.T) {
	monitor := &RSSMonitor{
		config:    DefaultConfig(),
		lastItems: make(map[string]Item),
	}

	items := []Item{
		{ID: "1", Title: "Item 1"},
		{ID: "2", Title: "Item 2"},
	}

	monitor.updateLastItems(items)

	if len(monitor.lastItems) != 2 {
		t.Errorf("Expected 2 items in lastItems, got %d", len(monitor.lastItems))
	}

	if _, exists := monitor.lastItems["1"]; !exists {
		t.Error("Expected item with ID '1' to exist in lastItems")
	}

	if _, exists := monitor.lastItems["2"]; !exists {
		t.Error("Expected item with ID '2' to exist in lastItems")
	}
}
