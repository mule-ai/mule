package rss_host

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/feeds"
)

func TestRSSHost(t *testing.T) {
	// Create a new RSS host integration
	config := DefaultConfig()
	logger := logr.Discard()
	rssHost := New(config, logger)

	// Test that the integration was created correctly
	if rssHost == nil {
		t.Fatal("Failed to create RSS host integration")
	}

	if rssHost.Name() != "rss_host" {
		t.Errorf("Expected name 'rss_host', got '%s'", rssHost.Name())
	}

	// Test adding an item
	item := &feeds.Item{
		Title:       "Test Item",
		Link:        &feeds.Link{Href: "http://example.com"},
		Description: "This is a test item",
		Author:      &feeds.Author{Name: "Test Author"},
		Created:     time.Now(),
	}

	rssHost.AddItem(item)

	// Verify the item was added
	items := rssHost.GetItems()
	if len(items) != 1 {
		t.Errorf("Expected 1 item, got %d", len(items))
	}

	if items[0].Title != "Test Item" {
		t.Errorf("Expected title 'Test Item', got '%s'", items[0].Title)
	}
}

func TestRSSFeedGeneration(t *testing.T) {
	// Create a new RSS host integration
	config := DefaultConfig()
	logger := logr.Discard()
	rssHost := New(config, logger)

	// Add a test item
	item := &feeds.Item{
		Title:       "Test Item",
		Link:        &feeds.Link{Href: "http://example.com"},
		Description: "This is a test item",
		Author:      &feeds.Author{Name: "Test Author"},
		Created:     time.Now(),
	}

	rssHost.AddItem(item)

	// Test RSS feed generation
	req := httptest.NewRequest("GET", "/rss", nil)
	w := httptest.NewRecorder()

	rssHost.HandleRSS(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	if resp.Header.Get("Content-Type") != "application/rss+xml" {
		t.Errorf("Expected content type 'application/rss+xml', got '%s'", resp.Header.Get("Content-Type"))
	}

	// Parse the RSS feed to verify it's valid
	var rssFeed struct {
		XMLName xml.Name `xml:"rss"`
		Channel struct {
			Title       string `xml:"title"`
			Description string `xml:"description"`
			Link        string `xml:"link"`
			Items       []struct {
				Title       string `xml:"title"`
				Link        string `xml:"link"`
				Description string `xml:"description"`
			} `xml:"item"`
		} `xml:"channel"`
	}

	err := xml.Unmarshal(body, &rssFeed)
	if err != nil {
		t.Fatalf("Failed to parse RSS feed: %v", err)
	}

	if rssFeed.Channel.Title != config.Title {
		t.Errorf("Expected title '%s', got '%s'", config.Title, rssFeed.Channel.Title)
	}

	if len(rssFeed.Channel.Items) != 1 {
		t.Errorf("Expected 1 item, got %d", len(rssFeed.Channel.Items))
	}

	if rssFeed.Channel.Items[0].Title != "Test Item" {
		t.Errorf("Expected item title 'Test Item', got '%s'", rssFeed.Channel.Items[0].Title)
	}
}

func TestAtomFeedGeneration(t *testing.T) {
	// Create a new RSS host integration
	config := DefaultConfig()
	logger := logr.Discard()
	rssHost := New(config, logger)

	// Add a test item
	item := &feeds.Item{
		Title:       "Test Item",
		Link:        &feeds.Link{Href: "http://example.com"},
		Description: "This is a test item",
		Author:      &feeds.Author{Name: "Test Author"},
		Created:     time.Now(),
	}

	rssHost.AddItem(item)

	// Test Atom feed generation
	req := httptest.NewRequest("GET", "/rss-atom", nil)
	w := httptest.NewRecorder()

	rssHost.HandleAtom(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	if resp.Header.Get("Content-Type") != "application/atom+xml" {
		t.Errorf("Expected content type 'application/atom+xml', got '%s'", resp.Header.Get("Content-Type"))
	}

	// Parse the Atom feed to verify it's valid
	var atomFeed struct {
		XMLName xml.Name `xml:"feed"`
		Title   string   `xml:"title"`
		Entries []struct {
			Title   string `xml:"title"`
			Link    string `xml:"link"`
			Summary string `xml:"summary"`
		} `xml:"entry"`
	}

	err := xml.Unmarshal(body, &atomFeed)
	if err != nil {
		t.Fatalf("Failed to parse Atom feed: %v", err)
	}

	if atomFeed.Title != config.Title {
		t.Errorf("Expected title '%s', got '%s'", config.Title, atomFeed.Title)
	}

	if len(atomFeed.Entries) != 1 {
		t.Errorf("Expected 1 entry, got %d", len(atomFeed.Entries))
	}

	if atomFeed.Entries[0].Title != "Test Item" {
		t.Errorf("Expected entry title 'Test Item', got '%s'", atomFeed.Entries[0].Title)
	}
}

func TestWebInterface(t *testing.T) {
	// Create a new RSS host integration
	config := DefaultConfig()
	logger := logr.Discard()
	rssHost := New(config, logger)

	// Add a test item
	item := &feeds.Item{
		Title:       "Test Item",
		Link:        &feeds.Link{Href: "http://example.com"},
		Description: "This is a test item",
		Author:      &feeds.Author{Name: "Test Author"},
		Created:     time.Now(),
	}

	rssHost.AddItem(item)

	// Test web interface
	req := httptest.NewRequest("GET", "/rss-index", nil)
	w := httptest.NewRecorder()

	rssHost.HandleIndex(w, req)

	resp := w.Result()
	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	if resp.Header.Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("Expected content type 'text/html; charset=utf-8', got '%s'", resp.Header.Get("Content-Type"))
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, "<title>"+config.Title+"</title>") {
		t.Errorf("Expected title '%s' in HTML, but it was not found", config.Title)
	}

	if !strings.Contains(bodyStr, "Test Item") {
		t.Errorf("Expected 'Test Item' in HTML, but it was not found")
	}
}

func TestMaxItemsLimit(t *testing.T) {
	// Create a new RSS host integration with a small max items limit
	config := DefaultConfig()
	config.MaxItems = 2
	logger := logr.Discard()
	rssHost := New(config, logger)

	// Add more items than the limit
	for i := 0; i < 5; i++ {
		item := &feeds.Item{
			Title:       fmt.Sprintf("Test Item %d", i),
			Link:        &feeds.Link{Href: fmt.Sprintf("http://example.com/%d", i)},
			Description: fmt.Sprintf("This is test item %d", i),
			Author:      &feeds.Author{Name: "Test Author"},
			Created:     time.Now(),
		}
		rssHost.AddItem(item)
	}

	// Verify only the max items are kept
	items := rssHost.GetItems()
	if len(items) != 2 {
		t.Errorf("Expected 2 items due to MaxItems limit, got %d", len(items))
	}

	// Verify the most recent items are kept (items should be in reverse chronological order)
	if items[0].Title != "Test Item 4" {
		t.Errorf("Expected most recent item 'Test Item 4', got '%s'", items[0].Title)
	}

	if items[1].Title != "Test Item 3" {
		t.Errorf("Expected second most recent item 'Test Item 3', got '%s'", items[1].Title)
	}
}
