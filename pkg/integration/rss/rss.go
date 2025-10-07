package rss

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/feeds"
	"github.com/mule-ai/mule/pkg/agent"
	"github.com/mule-ai/mule/pkg/types"
)

// Config holds the configuration for the RSS integration.
type Config struct {
	Enabled                       bool   `json:"enabled,omitempty"`
	Title                         string `json:"title,omitempty"`                         // RSS feed title
	Description                   string `json:"description,omitempty"`                   // RSS feed description
	Link                          string `json:"link,omitempty"`                          // RSS feed link
	Author                        string `json:"author,omitempty"`                        // RSS feed author
	MaxItems                      int    `json:"maxItems,omitempty"`                      // Maximum number of items to keep in feed
	Path                          string `json:"path,omitempty"`                          // URL path for RSS feed (default: /rss)
	MirrorFrom                    string `json:"mirrorFrom,omitempty"`                    // External RSS feed URL to mirror
	EnhanceContent                bool   `json:"enhanceContent,omitempty"`                // Whether to enhance content with AI
	CacheDir                      string `json:"cacheDir,omitempty"`                      // Directory for caching enhanced content
	FetchInterval                 int    `json:"fetchInterval,omitempty"`                 // Interval in minutes to fetch external feed
	CacheTTL                      int    `json:"cacheTTL,omitempty"`                      // Cache TTL in minutes (default: 360)
	UseDeterministicPreprocessing bool   `json:"useDeterministicPreprocessing,omitempty"` // Whether to use deterministic preprocessing before LLM
	SearxngURL                    string `json:"searxngURL,omitempty"`                    // Searxng URL for search queries
}

// RSS represents the RSS integration.
type RSS struct {
	config        *Config
	l             logr.Logger
	feed          *feeds.Feed
	items         []*feeds.Item
	mirroredItems []*feeds.Item // Items from external RSS feed
	mutex         sync.RWMutex
	channel       chan any
	triggers      map[string]chan any
	cache         map[string]CachedContent // Cache for enhanced content
	cacheMutex    sync.RWMutex
	stopFetcher   chan bool            // Channel to stop the fetcher goroutine
	agents        map[int]*agent.Agent // Available agents for article summarization
	processing    map[string]bool      // Track items currently being processed to prevent duplicates
	processingMux sync.Mutex           // Mutex for processing map
}

// ArticleMeta represents metadata extracted from an article
type ArticleMeta struct {
	Title       string    `json:"title"`
	Author      string    `json:"author"`
	PublishDate time.Time `json:"publishDate"`
	WordCount   int       `json:"wordCount"`
	Keywords    []string  `json:"keywords"`
}

// SearchHit represents a search result from Searxng
type SearchHit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// EnhancedRSSItem represents an RSS item with enhanced content
type EnhancedRSSItem struct {
	OriginalItem    *feeds.Item `json:"originalItem"`
	ArticleContent  string      `json:"articleContent"`  // Cleaned article text
	Metadata        ArticleMeta `json:"metadata"`        // Extracted metadata
	RelatedHits     []SearchHit `json:"relatedHits"`     // Top search results
	FinalSummary    string      `json:"finalSummary"`    // LLM-generated summary
	ProcessingError string      `json:"processingError"` // Any error during processing
}

// CachedContent represents cached enhanced content for an RSS item
type CachedContent struct {
	ItemID          string          `json:"itemId"`
	EnhancedContent string          `json:"enhancedContent"`
	Comments        []Comment       `json:"comments,omitempty"`
	Summary         string          `json:"summary,omitempty"`
	EnhancedItem    EnhancedRSSItem `json:"enhancedItem,omitempty"` // New enhanced item structure
	CachedAt        time.Time       `json:"cachedAt"`
	TTL             int             `json:"ttl"` // TTL in minutes
}

// Comment represents a comment on an RSS item
type Comment struct {
	Author  string    `json:"author"`
	Content string    `json:"content"`
	Score   int       `json:"score"`
	Created time.Time `json:"created"`
}

// Reddit API structures for fetching comments
type RedditResponse struct {
	Data RedditData `json:"data"`
}

type RedditData struct {
	Children []RedditPost `json:"children"`
}

type RedditPost struct {
	Data RedditPostData `json:"data"`
}

type RedditPostData struct {
	Title       string  `json:"title"`
	Author      string  `json:"author"`
	Score       int     `json:"score"`
	NumComments int     `json:"num_comments"`
	Created     float64 `json:"created_utc"`
	SelfText    string  `json:"selftext"`
	URL         string  `json:"url"`
	Permalink   string  `json:"permalink"`
	Subreddit   string  `json:"subreddit"`
}

type RedditCommentResponse struct {
	Kind string                 `json:"kind"`
	Data RedditCommentContainer `json:"data"`
}

type RedditCommentContainer struct {
	Children []RedditCommentItem `json:"children"`
}

type RedditCommentItem struct {
	Kind string            `json:"kind"`
	Data RedditCommentData `json:"data"`
}

type RedditCommentData struct {
	Author  string      `json:"author"`
	Body    string      `json:"body"`
	Score   int         `json:"score"`
	Created float64     `json:"created_utc"`
	Replies interface{} `json:"replies,omitempty"` // Can be string or RedditCommentResponse
}

// Hacker News API structures
type HNItem struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	By          string `json:"by"`
	Time        int64  `json:"time"`
	Text        string `json:"text"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	Score       int    `json:"score"`
	Descendants int    `json:"descendants"`
	Kids        []int  `json:"kids"`
	Parent      int    `json:"parent"`
}

var (
	events = map[string]struct{}{
		"addItem": {},
	}
)

// New creates a new RSS integration instance.
func New(config *Config, l logr.Logger, agents map[int]*agent.Agent) *RSS {
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
		config:        config,
		l:             l,
		items:         make([]*feeds.Item, 0),
		mirroredItems: make([]*feeds.Item, 0),
		channel:       make(chan any),
		triggers:      make(map[string]chan any),
		cache:         make(map[string]CachedContent),
		stopFetcher:   make(chan bool),
		agents:        agents,
		processing:    make(map[string]bool),
	}

	l.Info("RSS integration created", "title", config.Title, "agents_count", len(agents))

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
		Enabled:                       true,
		Title:                         "Mule Discord RSS Feed",
		Description:                   "RSS feed of Discord messages processed by Mule",
		Link:                          "http://localhost:8083/rss",
		Author:                        "Mule AI",
		MaxItems:                      100,
		Path:                          "/rss",
		UseDeterministicPreprocessing: true, // Enable deterministic preprocessing by default
	}
}

// init initializes the RSS integration.
func (r *RSS) init() {
	if !r.config.Enabled {
		r.l.Info("RSS integration is disabled")
		return
	}

	r.l.Info("RSS integration initialized - handlers will be registered with main server", "path", r.config.Path, "url", r.config.Link)

	// Start RSS fetcher if mirroring is enabled
	if r.config.MirrorFrom != "" {
		r.l.Info("Starting RSS fetcher", "mirrorFrom", r.config.MirrorFrom, "interval", r.config.FetchInterval)
		go r.startFetcher()
	}

	// Initialize cache directory if content enhancement is enabled
	if r.config.EnhanceContent && r.config.CacheDir != "" {
		if err := os.MkdirAll(r.config.CacheDir, 0755); err != nil {
			r.l.Error(err, "Failed to create cache directory", "dir", r.config.CacheDir)
		} else {
			r.loadCache()
		}
	}
}

// HandleRSS serves the RSS feed.
func (r *RSS) HandleRSS(w http.ResponseWriter, req *http.Request) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()

	// Combine local and mirrored items
	allItems := make([]*feeds.Item, 0, len(r.items)+len(r.mirroredItems))
	allItems = append(allItems, r.items...)
	allItems = append(allItems, r.mirroredItems...)

	// Sort by created date (newest first)
	// Note: feeds.Feed will handle sorting internally

	// Update feed items
	r.feed.Items = allItems

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
		var itemData map[string]string

		// Try to handle different data formats
		switch v := data.(type) {
		case map[string]string:
			itemData = v
		case string:
			// Try to parse as JSON
			if err := json.Unmarshal([]byte(v), &itemData); err != nil {
				r.l.Error(err, "Failed to parse JSON data for addItem", "data", v)
				return nil, fmt.Errorf("invalid data format for addItem: %w", err)
			}
		default:
			return nil, fmt.Errorf("invalid data format for addItem: expected string or map[string]string, got %T", data)
		}

		r.AddItem(
			itemData["title"],
			itemData["description"],
			itemData["link"],
			itemData["author"],
		)
		return "", nil // Return empty string for workflow compatibility
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
			var itemData map[string]string

			// Try to handle different data formats
			switch v := triggerSettings.Data.(type) {
			case map[string]string:
				itemData = v
			case string:
				// Try to parse as JSON
				if err := json.Unmarshal([]byte(v), &itemData); err != nil {
					r.l.Error(err, "Failed to parse JSON data for addItem", "data", v)
					continue
				}
			default:
				r.l.Error(fmt.Errorf("trigger data is not a map[string]string or JSON string, got %T", triggerSettings.Data), "Invalid trigger data type")
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

// startFetcher starts a goroutine that periodically fetches external RSS feeds
func (r *RSS) startFetcher() {
	// Set default fetch interval if not specified
	interval := r.config.FetchInterval
	if interval <= 0 {
		interval = 5 // Default to 5 minutes
	}

	ticker := time.NewTicker(time.Duration(interval) * time.Minute)
	defer ticker.Stop()

	// Fetch immediately on startup
	r.fetchExternalRSS()

	for {
		select {
		case <-ticker.C:
			r.fetchExternalRSS()
		case <-r.stopFetcher:
			r.l.Info("Stopping RSS fetcher")
			return
		}
	}
}

// fetchExternalRSS fetches and parses an external RSS feed
func (r *RSS) fetchExternalRSS() {
	if r.config.MirrorFrom == "" {
		return
	}

	r.l.Info("Fetching external RSS feed", "url", r.config.MirrorFrom)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	req, err := http.NewRequest("GET", r.config.MirrorFrom, nil)
	if err != nil {
		r.l.Error(err, "Failed to create HTTP request", "url", r.config.MirrorFrom)
		return
	}

	// Set a proper User-Agent to avoid being blocked
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MuleAI-RSS/1.0; +http://localhost:8083)")
	req.Header.Set("Accept", "application/rss+xml, application/xml, text/xml, */*")

	resp, err := client.Do(req)
	if err != nil {
		r.l.Error(err, "Failed to fetch external RSS feed", "url", r.config.MirrorFrom)
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			r.l.Error(err, "Failed to close response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		r.l.Error(fmt.Errorf("HTTP %d", resp.StatusCode), "Failed to fetch external RSS feed", "url", r.config.MirrorFrom)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		r.l.Error(err, "Failed to read RSS response")
		return
	}

	// Try to parse as RSS 2.0 first
	type RSS struct {
		Channel struct {
			Title       string `xml:"title"`
			Link        string `xml:"link"`
			Description string `xml:"description"`
			Items       []struct {
				Title       string `xml:"title"`
				Link        string `xml:"link"`
				Description string `xml:"description"`
				PubDate     string `xml:"pubDate"`
				Author      string `xml:"author"`
				GUID        string `xml:"guid"`
			} `xml:"item"`
		} `xml:"channel"`
	}

	// Try to parse as Atom feed (used by Reddit)
	type AtomFeed struct {
		XMLName   xml.Name `xml:"feed"`
		Namespace string   `xml:"xmlns,attr"`
		Title     string   `xml:"title"`
		Link      struct {
			Href string `xml:"href,attr"`
		} `xml:"link"`
		Entries []struct {
			Title   string `xml:"title"`
			Content struct {
				Text string `xml:",chardata"`
				Type string `xml:"type,attr"`
			} `xml:"content"`
			Link struct {
				Href string `xml:"href,attr"`
			} `xml:"link"`
			Updated string `xml:"updated"`
			Author  struct {
				Name string `xml:"name"`
			} `xml:"author"`
			ID string `xml:"id"`
		} `xml:"entry"`
	}

	var mirroredItems []*feeds.Item

	// Try RSS 2.0 first
	var rss RSS
	if err := xml.Unmarshal(body, &rss); err == nil && len(rss.Channel.Items) > 0 {
		r.l.Info("Parsed RSS 2.0 feed", "channel_title", rss.Channel.Title, "item_count", len(rss.Channel.Items))

		mirroredItems = make([]*feeds.Item, 0, len(rss.Channel.Items))
		for _, item := range rss.Channel.Items {
			pubDate, _ := time.Parse(time.RFC1123Z, item.PubDate)
			if pubDate.IsZero() {
				pubDate, _ = time.Parse(time.RFC1123, item.PubDate)
			}

			feedItem := &feeds.Item{
				Title:       item.Title,
				Link:        &feeds.Link{Href: item.Link},
				Description: item.Description,
				Author:      &feeds.Author{Name: item.Author},
				Created:     pubDate,
				Id:          item.GUID,
			}

			// Generate ID from description for HN feeds (always override for consistency)
			if strings.Contains(item.Description, "news.ycombinator.com/item?id=") {
				// Extract HN item ID from the comments link in description
				re := regexp.MustCompile(`id=(\d+)`)
				matches := re.FindStringSubmatch(item.Description)
				if len(matches) > 1 {
					feedItem.Id = "hn-" + matches[1]
				}
			}

			// If still no ID, generate one from the link
			if feedItem.Id == "" && item.Link != "" {
				feedItem.Id = fmt.Sprintf("%d-%s", pubDate.Unix(), item.Link)
			}

			// Enhance content if enabled
			if r.config.EnhanceContent {
				r.enhanceItem(feedItem)
			}

			mirroredItems = append(mirroredItems, feedItem)
		}
	} else {
		// Try Atom format (used by Reddit)
		var atom AtomFeed
		if err := xml.Unmarshal(body, &atom); err == nil && len(atom.Entries) > 0 {
			r.l.Info("Parsed Atom feed", "feed_title", atom.Title, "entry_count", len(atom.Entries))

			mirroredItems = make([]*feeds.Item, 0, len(atom.Entries))
			for _, entry := range atom.Entries {
				updated, _ := time.Parse(time.RFC3339, entry.Updated)

				feedItem := &feeds.Item{
					Title:       entry.Title,
					Link:        &feeds.Link{Href: entry.Link.Href},
					Description: entry.Content.Text,
					Author:      &feeds.Author{Name: entry.Author.Name},
					Created:     updated,
					Id:          entry.ID,
				}

				// Generate ID if empty
				if feedItem.Id == "" && entry.Link.Href != "" {
					feedItem.Id = fmt.Sprintf("%d-%s", updated.Unix(), entry.Link.Href)
				}

				// Enhance content if enabled
				if r.config.EnhanceContent {
					r.enhanceItem(feedItem)
				}

				mirroredItems = append(mirroredItems, feedItem)
			}
		} else {
			preview := string(body)
			if len(preview) > 200 {
				preview = preview[:200]
			}
			r.l.Error(fmt.Errorf("failed to parse as RSS or Atom"), "Failed to parse feed", "response_length", len(body), "response_start", preview)
			return
		}
	}

	// Update mirrored items
	r.mutex.Lock()
	r.mirroredItems = mirroredItems
	r.mutex.Unlock()

	r.l.Info("Successfully fetched and parsed RSS feed", "items", len(mirroredItems))
}

// enhanceItem enhances an RSS item with additional content
func (r *RSS) enhanceItem(item *feeds.Item) {
	// Use URL as fallback ID if item.Id is empty
	itemID := item.Id
	if itemID == "" && item.Link != nil && item.Link.Href != "" {
		itemID = item.Link.Href
	}

	if itemID == "" {
		return
	}

	// Check cache first
	r.cacheMutex.RLock()
	cached, exists := r.cache[itemID]
	r.cacheMutex.RUnlock()

	if exists && !r.isCacheExpired(cached) {
		// Use cached content
		if cached.EnhancedContent != "" {
			item.Description = cached.EnhancedContent
		}
		return
	}

	// Check if this item is currently being processed to avoid duplicates
	if r.isProcessing(itemID) {
		r.l.Info("Item enhancement already in progress, skipping duplicate", "itemID", itemID)
		return
	}

	// Mark as processing
	r.setProcessing(itemID, true)
	defer r.setProcessing(itemID, false)

	// Check if this is a Hacker News post and fetch comments
	if isHN, hnItemID := r.isHackerNewsPost(item); isHN {
		comments := r.fetchHackerNewsComments(hnItemID)

		// Generate AI summary for HN posts with external links
		var summary string
		if item.Link != nil && item.Link.Href != "" && !strings.Contains(item.Link.Href, "news.ycombinator.com") {
			summary = r.generateArticleSummary(item, false)
		}

		// Build enhanced content
		enhancedContent := item.Description
		if enhancedContent != "" {
			enhancedContent += "\n\n"
		}

		// Add AI summary if available
		if summary != "" {
			enhancedContent += "\n\n<br/><br/>" + summary
		}

		// Add comments section
		if len(comments) > 0 {
			enhancedContent += r.formatHackerNewsComments(comments)
		} else {
			enhancedContent += "\n\n<br/><br/><strong>💬 Hacker News Comments:</strong><br/><br/><em>No comments available for this post.</em>"
		}

		// Cache the enhanced content
		cachedContent := CachedContent{
			ItemID:          itemID,
			EnhancedContent: enhancedContent,
			Comments:        comments,
			Summary:         summary,
			CachedAt:        time.Now(),
			TTL:             r.getCacheTTL(),
		}

		r.UpdateCachedContent(itemID, cachedContent)
		item.Description = enhancedContent

		r.l.Info("Enhanced HN item with AI summary and comments", "id", item.Id, "hnID", hnItemID, "comments", len(comments), "has_summary", summary != "")
		return
	}

	// Check if this is a Reddit post and fetch comments
	if r.isRedditPost(item) {
		comments := r.fetchRedditComments(item)
		// Always enhance with comments (even if empty) and generate AI summary for Reddit posts
		enhancedContent := r.enhanceWithComments(item, comments)

		// Also generate an AI summary for Reddit posts
		summary := r.generateArticleSummary(item, false)
		if summary != "" {
			// Add the AI summary before the comments
			enhancedContent = item.Description
			if enhancedContent != "" {
				enhancedContent += "\n\n"
			}
			enhancedContent += "\n\n<br/><br/>" + summary

			// Then add the comments section
			if len(comments) > 0 {
				enhancedContent += r.formatRedditComments(comments)
			} else {
				enhancedContent += "\n\n<br/><br/><strong>🗣️ Reddit Comments:</strong><br/><br/><em>No comments available for this post.</em>"
			}
		}

		// Cache the enhanced content
		cachedContent := CachedContent{
			ItemID:          itemID,
			EnhancedContent: enhancedContent,
			Comments:        comments,
			Summary:         summary,
			CachedAt:        time.Now(),
			TTL:             r.getCacheTTL(),
		}

		r.UpdateCachedContent(itemID, cachedContent)
		item.Description = enhancedContent

		r.l.Info("Enhanced Reddit item with AI summary and comments", "id", item.Id, "comments", len(comments), "has_summary", summary != "")
		return
	}

	// For non-Reddit content, use the deterministic enhancement approach if enabled
	if item.Link != nil && item.Link.Href != "" {
		if r.config.UseDeterministicPreprocessing {
			// Try deterministic enhancement first
			enhancedItem, err := r.enhanceItemDeterministic(item)
			if err != nil {
				r.l.Error(err, "Failed to enhance item deterministically, falling back to traditional method", "id", item.Id)
				// Fall back to traditional method
				summary := r.generateArticleSummary(item, false)
				if summary != "" {
					enhancedContent := item.Description
					if enhancedContent != "" {
						enhancedContent += "\n\n"
					}
					// Format the summary with proper HTML formatting
					formattedSummary := r.formatArticleSummary(summary)
					enhancedContent += "\n\n<br/><br/>" + formattedSummary

					// Cache the enhanced content
					cachedContent := CachedContent{
						ItemID:          itemID,
						EnhancedContent: enhancedContent,
						Summary:         summary,
						CachedAt:        time.Now(),
						TTL:             r.getCacheTTL(),
					}

					r.UpdateCachedContent(itemID, cachedContent)
					item.Description = enhancedContent

					r.l.Info("Enhanced item with AI summary (fallback method)", "id", item.Id, "summary_length", len(summary))
					return
				}
			} else {
				// Successfully enhanced with deterministic approach
				enhancedContent := item.Description
				if enhancedContent != "" {
					enhancedContent += "\n\n"
				}

				// Add the final summary from the enhanced item
				if enhancedItem.FinalSummary != "" {
					// Format the summary with proper HTML formatting
					formattedSummary := r.formatArticleSummary(enhancedItem.FinalSummary)
					enhancedContent += "\n\n<br/><br/>" + formattedSummary
				}

				// Cache the enhanced content
				cachedContent := CachedContent{
					ItemID:          itemID,
					EnhancedContent: enhancedContent,
					EnhancedItem:    *enhancedItem,
					CachedAt:        time.Now(),
					TTL:             r.getCacheTTL(),
				}

				r.UpdateCachedContent(itemID, cachedContent)
				item.Description = enhancedContent

				r.l.Info("Enhanced item with deterministic approach", "id", item.Id, "summary_length", len(enhancedItem.FinalSummary))
				return
			}
		} else {
			// Use traditional method if deterministic preprocessing is disabled
			summary := r.generateArticleSummary(item, false)
			if summary != "" {
				enhancedContent := item.Description
				if enhancedContent != "" {
					enhancedContent += "\n\n"
				}
				// Format the summary with proper HTML formatting
				formattedSummary := r.formatArticleSummary(summary)
				enhancedContent += "\n\n<br/><br/>" + formattedSummary

				// Cache the enhanced content
				cachedContent := CachedContent{
					ItemID:          itemID,
					EnhancedContent: enhancedContent,
					Summary:         summary,
					CachedAt:        time.Now(),
					TTL:             r.getCacheTTL(),
				}

				r.UpdateCachedContent(itemID, cachedContent)
				item.Description = enhancedContent

				r.l.Info("Enhanced item with AI summary (traditional method)", "id", item.Id, "summary_length", len(summary))
				return
			}
		}
	}

	r.l.Info("Item needs enhancement but no summarization available", "id", item.Id, "title", item.Title)
}

// getCacheTTL returns the configured cache TTL or default value (360 minutes = 6 hours)
func (r *RSS) getCacheTTL() int {
	if r.config.CacheTTL > 0 {
		return r.config.CacheTTL
	}
	return 360 // Default to 6 hours
}

// isCacheExpired checks if cached content has expired
func (r *RSS) isCacheExpired(cached CachedContent) bool {
	// Always use the current configured TTL to allow for TTL updates
	ttl := r.getCacheTTL()

	expiry := cached.CachedAt.Add(time.Duration(ttl) * time.Minute)
	return time.Now().After(expiry)
}

// isProcessing checks if an item is currently being processed
func (r *RSS) isProcessing(itemID string) bool {
	r.processingMux.Lock()
	defer r.processingMux.Unlock()
	return r.processing[itemID]
}

// setProcessing marks an item as being processed
func (r *RSS) setProcessing(itemID string, processing bool) {
	r.processingMux.Lock()
	defer r.processingMux.Unlock()
	if processing {
		r.processing[itemID] = true
	} else {
		delete(r.processing, itemID)
	}
}

// loadCache loads cached content from disk
func (r *RSS) loadCache() {
	cacheFile := filepath.Join(r.config.CacheDir, "rss_cache.json")

	data, err := os.ReadFile(cacheFile)
	if err != nil {
		if !os.IsNotExist(err) {
			r.l.Error(err, "Failed to read cache file", "file", cacheFile)
		}
		return
	}

	var cache map[string]CachedContent
	if err := json.Unmarshal(data, &cache); err != nil {
		r.l.Error(err, "Failed to parse cache file", "file", cacheFile)
		return
	}

	r.cacheMutex.Lock()
	r.cache = cache
	r.cacheMutex.Unlock()

	r.l.Info("Loaded cache", "items", len(cache))
}

// saveCache saves cached content to disk
func (r *RSS) saveCache() {
	if r.config.CacheDir == "" {
		return
	}

	cacheFile := filepath.Join(r.config.CacheDir, "rss_cache.json")

	r.cacheMutex.RLock()
	data, err := json.MarshalIndent(r.cache, "", "  ")
	r.cacheMutex.RUnlock()

	if err != nil {
		r.l.Error(err, "Failed to marshal cache")
		return
	}

	if err := os.WriteFile(cacheFile, data, 0644); err != nil {
		r.l.Error(err, "Failed to write cache file", "file", cacheFile)
		return
	}
}

// UpdateCachedContent updates the cache with enhanced content
func (r *RSS) UpdateCachedContent(itemID string, content CachedContent) {
	r.cacheMutex.Lock()
	r.cache[itemID] = content
	r.cacheMutex.Unlock()

	// Save cache to disk
	r.saveCache()
}

// isRedditPost checks if the RSS item is from Reddit
func (r *RSS) isRedditPost(item *feeds.Item) bool {
	if item.Link == nil || item.Link.Href == "" {
		return false
	}

	return strings.Contains(item.Link.Href, "reddit.com/r/")
}

// isHackerNewsPost checks if the RSS item is from Hacker News
func (r *RSS) isHackerNewsPost(item *feeds.Item) (bool, string) {
	if item.Link == nil {
		return false, ""
	}

	// Check if the description contains HN comments link
	if strings.Contains(item.Description, "news.ycombinator.com/item?id=") {
		// Extract the HN item ID from the description
		re := regexp.MustCompile(`https://news\.ycombinator\.com/item\?id=(\d+)`)
		matches := re.FindStringSubmatch(item.Description)
		if len(matches) > 1 {
			return true, matches[1]
		}
	}

	// Also check if the link itself is to HN (for Ask HN, Show HN posts)
	if strings.Contains(item.Link.Href, "news.ycombinator.com/item?id=") {
		re := regexp.MustCompile(`id=(\d+)`)
		matches := re.FindStringSubmatch(item.Link.Href)
		if len(matches) > 1 {
			return true, matches[1]
		}
	}

	return false, ""
}

// fetchHackerNewsComments fetches comments from Hacker News for a given post
func (r *RSS) fetchHackerNewsComments(itemID string) []Comment {
	if itemID == "" {
		return nil
	}

	apiURL := fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%s.json", itemID)
	r.l.Info("Fetching Hacker News item", "url", apiURL, "itemID", itemID)

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		r.l.Error(err, "Failed to create HN request", "itemID", itemID)
		return nil
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MuleAI-RSS/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		r.l.Error(err, "Failed to fetch HN item", "itemID", itemID)
		return nil
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			r.l.Error(err, "Failed to close response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		r.l.Error(fmt.Errorf("HTTP %d", resp.StatusCode), "Failed to fetch HN item", "itemID", itemID)
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		r.l.Error(err, "Failed to read HN response")
		return nil
	}

	var hnItem HNItem
	if err := json.Unmarshal(body, &hnItem); err != nil {
		r.l.Error(err, "Failed to parse HN item", "itemID", itemID)
		return nil
	}

	// Fetch top-level comments
	var comments []Comment
	if len(hnItem.Kids) == 0 {
		r.l.Info("No comments found for HN item", "itemID", itemID)
		return nil
	}

	// Fetch each comment (limit to first 30 to avoid too many API calls)
	maxComments := 30
	if len(hnItem.Kids) < maxComments {
		maxComments = len(hnItem.Kids)
	}

	for i := 0; i < maxComments; i++ {
		commentID := hnItem.Kids[i]
		if comment := r.fetchSingleHNComment(commentID); comment != nil {
			comments = append(comments, *comment)

			// Also fetch replies to this comment (one level deep)
			if hnComment := r.fetchHNItemDetails(commentID); hnComment != nil && len(hnComment.Kids) > 0 {
				// Limit replies to 5 per comment
				maxReplies := 5
				if len(hnComment.Kids) < maxReplies {
					maxReplies = len(hnComment.Kids)
				}

				for j := 0; j < maxReplies; j++ {
					replyID := hnComment.Kids[j]
					if reply := r.fetchSingleHNComment(replyID); reply != nil {
						comments = append(comments, *reply)
					}
				}
			}
		}
	}

	r.l.Info("Fetched HN comments", "itemID", itemID, "count", len(comments))
	return comments
}

// fetchSingleHNComment fetches a single comment from HN API
func (r *RSS) fetchSingleHNComment(commentID int) *Comment {
	apiURL := fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%d.json", commentID)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		r.l.Error(err, "Failed to create HN comment request", "commentID", commentID)
		return nil
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MuleAI-RSS/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		r.l.Error(err, "Failed to fetch HN comment", "commentID", commentID)
		return nil
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			r.l.Error(err, "Failed to close response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var hnComment HNItem
	if err := json.Unmarshal(body, &hnComment); err != nil {
		return nil
	}

	// Skip deleted or dead comments
	if hnComment.Text == "" || hnComment.By == "" {
		return nil
	}

	// Convert HTML entities in HN comments
	text := hnComment.Text
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&#x27;", "'")
	text = strings.ReplaceAll(text, "&#x2F;", "/")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	// Remove <p> tags but keep the content
	text = strings.ReplaceAll(text, "<p>", "\n\n")
	text = strings.TrimSpace(text)

	return &Comment{
		Author:  hnComment.By,
		Content: text,
		Score:   hnComment.Score,
		Created: time.Unix(hnComment.Time, 0),
	}
}

// fetchHNItemDetails fetches details of an HN item (for getting child comment IDs)
func (r *RSS) fetchHNItemDetails(itemID int) *HNItem {
	apiURL := fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%d.json", itemID)

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MuleAI-RSS/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			r.l.Error(err, "Failed to close response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var hnItem HNItem
	if err := json.Unmarshal(body, &hnItem); err != nil {
		return nil
	}

	return &hnItem
}

// fetchRedditComments fetches comments from Reddit for a given post
func (r *RSS) fetchRedditComments(item *feeds.Item) []Comment {
	if item.Link == nil || item.Link.Href == "" {
		return nil
	}

	// Extract Reddit post URL and convert to JSON API format
	postURL := item.Link.Href

	// Convert Reddit URL to JSON API URL
	// Example: https://www.reddit.com/r/LocalLLaMA/comments/123/title/ -> https://www.reddit.com/r/LocalLLaMA/comments/123/title/.json
	if !strings.HasSuffix(postURL, ".json") {
		if strings.HasSuffix(postURL, "/") {
			postURL = postURL + ".json"
		} else {
			postURL = postURL + "/.json"
		}
	}

	r.l.Info("Fetching Reddit comments", "url", postURL)

	client := &http.Client{
		Timeout: 15 * time.Second,
	}

	req, err := http.NewRequest("GET", postURL, nil)
	if err != nil {
		r.l.Error(err, "Failed to create Reddit request", "url", postURL)
		return nil
	}

	// Set User-Agent to avoid rate limiting
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MuleAI-RSS/1.0; +http://localhost:8083)")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		r.l.Error(err, "Failed to fetch Reddit comments", "url", postURL)
		return nil
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			r.l.Error(err, "Failed to close response body")
		}
	}()

	if resp.StatusCode != http.StatusOK {
		r.l.Error(fmt.Errorf("HTTP %d", resp.StatusCode), "Failed to fetch Reddit comments", "url", postURL)
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		r.l.Error(err, "Failed to read Reddit response")
		return nil
	}

	// Parse Reddit JSON response
	var responses []RedditCommentResponse
	if err := json.Unmarshal(body, &responses); err != nil {
		r.l.Error(err, "Failed to parse Reddit response")
		return nil
	}

	if len(responses) < 2 {
		r.l.Info("No comments found", "responses", len(responses))
		return nil
	}

	// Second response contains comments
	commentsResponse := responses[1]

	var comments []Comment

	// Extract comments from Reddit response
	for _, child := range commentsResponse.Data.Children {
		if child.Kind == "t1" && child.Data.Body != "[deleted]" && child.Data.Body != "[removed]" {
			comment := Comment{
				Author:  child.Data.Author,
				Content: child.Data.Body,
				Score:   child.Data.Score,
				Created: time.Unix(int64(child.Data.Created), 0),
			}
			comments = append(comments, comment)

			// Recursively get replies if they exist and are not empty string
			if child.Data.Replies != nil {
				if repliesData, ok := child.Data.Replies.(map[string]interface{}); ok {
					// Convert map to RedditCommentResponse
					repliesBytes, err := json.Marshal(repliesData)
					if err == nil {
						var repliesResponse RedditCommentResponse
						if err := json.Unmarshal(repliesBytes, &repliesResponse); err == nil {
							replies := r.extractReplies(&repliesResponse)
							comments = append(comments, replies...)
						}
					}
				}
			}
		}
	}

	r.l.Info("Fetched Reddit comments", "count", len(comments))
	return comments
}

// extractReplies recursively extracts replies from Reddit comment tree
func (r *RSS) extractReplies(replies *RedditCommentResponse) []Comment {
	var comments []Comment

	if replies == nil {
		return comments
	}

	for _, child := range replies.Data.Children {
		if child.Kind == "t1" && child.Data.Body != "[deleted]" && child.Data.Body != "[removed]" {
			comment := Comment{
				Author:  child.Data.Author,
				Content: child.Data.Body,
				Score:   child.Data.Score,
				Created: time.Unix(int64(child.Data.Created), 0),
			}
			comments = append(comments, comment)

			// Recursively get nested replies
			if child.Data.Replies != nil {
				if repliesData, ok := child.Data.Replies.(map[string]interface{}); ok {
					// Convert map to RedditCommentResponse
					repliesBytes, err := json.Marshal(repliesData)
					if err == nil {
						var repliesResponse RedditCommentResponse
						if err := json.Unmarshal(repliesBytes, &repliesResponse); err == nil {
							nestedReplies := r.extractReplies(&repliesResponse)
							comments = append(comments, nestedReplies...)
						}
					}
				}
			}
		}
	}

	return comments
}

// enhanceWithComments creates enhanced content by adding top comments to the original description
func (r *RSS) enhanceWithComments(item *feeds.Item, comments []Comment) string {
	if len(comments) == 0 {
		return item.Description
	}

	// Sort comments by score (highest first)
	sortedComments := make([]Comment, len(comments))
	copy(sortedComments, comments)

	// Simple bubble sort by score (descending)
	for i := 0; i < len(sortedComments)-1; i++ {
		for j := 0; j < len(sortedComments)-i-1; j++ {
			if sortedComments[j].Score < sortedComments[j+1].Score {
				sortedComments[j], sortedComments[j+1] = sortedComments[j+1], sortedComments[j]
			}
		}
	}

	// Build enhanced content with original description + top comments
	enhanced := item.Description

	if enhanced != "" {
		enhanced += "\n\n"
	}

	enhanced += "\n\n<br/><br/><strong>🗣️ Top Reddit Comments:</strong><br/><br/>"

	// Add top 10 comments (increased from 5)
	maxComments := 10
	if len(sortedComments) < maxComments {
		maxComments = len(sortedComments)
	}

	for i := 0; i < maxComments; i++ {
		comment := sortedComments[i]
		// Convert newlines to HTML breaks
		formattedContent := strings.ReplaceAll(comment.Content, "\n", "<br/>")

		enhanced += fmt.Sprintf("<strong>%s</strong> (Score: %d):<br/>%s<br/><br/>",
			comment.Author, comment.Score, formattedContent)
	}

	return enhanced
}

// formatRedditComments formats Reddit comments for display (helper function)
func (r *RSS) formatRedditComments(comments []Comment) string {
	if len(comments) == 0 {
		return "\n\n<br/><br/><strong>🗣️ Reddit Comments:</strong><br/><br/><em>No comments available for this post.</em>"
	}

	// Sort comments by score (highest first)
	sortedComments := make([]Comment, len(comments))
	copy(sortedComments, comments)

	// Simple bubble sort by score (descending)
	for i := 0; i < len(sortedComments)-1; i++ {
		for j := 0; j < len(sortedComments)-i-1; j++ {
			if sortedComments[j].Score < sortedComments[j+1].Score {
				sortedComments[j], sortedComments[j+1] = sortedComments[j+1], sortedComments[j]
			}
		}
	}

	formatted := "\n\n<br/><br/><strong>🗣️ Top Reddit Comments:</strong><br/><br/>"

	// Add top 10 comments
	maxComments := 10
	if len(sortedComments) < maxComments {
		maxComments = len(sortedComments)
	}

	for i := 0; i < maxComments; i++ {
		comment := sortedComments[i]
		// Convert newlines to HTML breaks
		formattedContent := strings.ReplaceAll(comment.Content, "\n", "<br/>")

		formatted += fmt.Sprintf("<strong>%s</strong> (Score: %d):<br/>%s<br/><br/>",
			comment.Author, comment.Score, formattedContent)
	}

	return formatted
}

// formatHackerNewsComments formats Hacker News comments for display
func (r *RSS) formatHackerNewsComments(comments []Comment) string {
	if len(comments) == 0 {
		return "\n\n<br/><br/><strong>💬 Hacker News Comments:</strong><br/><br/><em>No comments available for this post.</em>"
	}

	// Sort comments by score (highest first)
	sortedComments := make([]Comment, len(comments))
	copy(sortedComments, comments)

	// Simple bubble sort by score (descending)
	for i := 0; i < len(sortedComments)-1; i++ {
		for j := 0; j < len(sortedComments)-i-1; j++ {
			if sortedComments[j].Score < sortedComments[j+1].Score {
				sortedComments[j], sortedComments[j+1] = sortedComments[j+1], sortedComments[j]
			}
		}
	}

	formatted := "\n\n<br/><br/><strong>💬 Top Hacker News Comments:</strong><br/><br/>"

	// Add top 10 comments
	maxComments := 10
	if len(sortedComments) < maxComments {
		maxComments = len(sortedComments)
	}

	for i := 0; i < maxComments; i++ {
		comment := sortedComments[i]
		// Convert newlines to HTML breaks
		formattedContent := strings.ReplaceAll(comment.Content, "\n", "<br/>")
		// Also handle nested HTML from HN (like <i> tags)
		formattedContent = strings.ReplaceAll(formattedContent, "<i>", "<em>")
		formattedContent = strings.ReplaceAll(formattedContent, "</i>", "</em>")

		formatted += fmt.Sprintf("<strong>%s</strong> (Score: %d):<br/>%s<br/><br/>",
			comment.Author, comment.Score, formattedContent)
	}

	return formatted
}

// generateArticleSummary generates an AI summary of a web article using an agent with RetrievePage tool
// When checkProcessing is false, it skips processing state checks (used when called from within enhanceItem)
func (r *RSS) generateArticleSummary(item *feeds.Item, checkProcessing bool) string {
	if item.Link == nil || item.Link.Href == "" {
		return ""
	}

	// Use URL as fallback ID if item.Id is empty
	itemID := item.Id
	if itemID == "" {
		itemID = item.Link.Href
	}

	// Check if we already have a cached summary for this item
	r.cacheMutex.RLock()
	cached, exists := r.cache[itemID]
	r.cacheMutex.RUnlock()

	if exists && !r.isCacheExpired(cached) && cached.Summary != "" {
		r.l.Info("Using cached article summary", "url", item.Link.Href, "title", item.Title, "cached_at", cached.CachedAt)
		return cached.Summary
	}

	// Only check processing state if requested (to avoid issues when called from within enhanceItem)
	if checkProcessing {
		// Check if this item is currently being processed by another goroutine
		if r.isProcessing(itemID) {
			r.l.Info("Article is already being processed, skipping duplicate request", "url", item.Link.Href, "title", item.Title)
			return ""
		}

		// Mark as processing to prevent duplicate requests
		r.setProcessing(itemID, true)
		defer r.setProcessing(itemID, false)
	}

	r.l.Info("Article summarization requested", "url", item.Link.Href, "title", item.Title)

	// Try to find an agent with RetrievePage tool for article summarization

	// Find any agent that has RetrievePage tool (try common research agent IDs)
	var researchAgent *agent.Agent
	researchAgentIDs := []int{18, 17} // Try Research agent first, then Planning agent

	for _, agentID := range researchAgentIDs {
		if agent, exists := r.agents[agentID]; exists && agent != nil {
			// Check if agent has RetrievePage tool by looking at tools
			tools := agent.GetTools()
			for _, tool := range tools {
				if tool == "RetrievePage" {
					researchAgent = agent
					r.l.Info("Found research agent with RetrievePage tool", "agentID", agentID, "name", agent.Name)
					break
				}
			}
			if researchAgent != nil {
				break
			}
		}
	}

	if researchAgent == nil {
		r.l.Error(nil, "No agent with RetrievePage tool available for article summarization", "availableAgents", len(r.agents))
		return fmt.Sprintf("<strong>📰 Article Summary</strong><br/><br/>Article summarization service unavailable (no RetrievePage tool found).<br/><br/><a href='%s' target='_blank'>View Original Article</a>",
			item.Link.Href)
	}

	// Clone the agent to avoid shared state issues with parallel workflows
	researchAgent = researchAgent.Clone()

	// Resolve redirects to get the final URL
	r.l.Info("Starting redirect resolution", "original_url", item.Link.Href)
	finalURL := r.resolveRedirects(item.Link.Href)
	if finalURL != item.Link.Href {
		r.l.Info("Resolved redirect", "original", item.Link.Href, "final", finalURL)
	} else {
		r.l.Info("No redirect found", "url", item.Link.Href)
	}

	// Use the agent to fetch and summarize the article with both TL;DR and comprehensive summary
	prompt := fmt.Sprintf(`Please analyze this article and provide a two-part summary:

1. TL;DR (2-3 sentences): A brief overview of the most important points.
2. Comprehensive Summary: A more detailed summary covering the main arguments, key facts, supporting details, and any important context or implications.

Article URL: %s
Title: %s

IMPORTANT: When using the RetrievePage tool:
- If the retrieved content appears to be only JavaScript code, error messages (like 403, 404, etc.), or doesn't contain actual article text, try searching for alternative URLs or sources for this article.
- Look for patterns indicating the page requires JavaScript rendering or is behind a paywall/login wall.
- If you cannot retrieve the actual article content after trying alternative approaches, indicate this clearly in your response rather than attempting to summarize JavaScript or error content.

Format your response as:
**TL;DR:** [2-3 sentence summary]

**Detailed Summary:** [comprehensive summary]

Use the RetrievePage tool to fetch the article content. If the initial retrieval fails or returns non-article content, try alternative approaches to find the actual article.`,
		finalURL, item.Title)

	summary, err := researchAgent.GenerateWithTools("", agent.PromptInput{
		Message: prompt,
	})

	if err != nil {
		r.l.Error(err, "Failed to generate article summary", "url", finalURL)
		return fmt.Sprintf("<strong>📰 Article Summary</strong><br/><br/>Unable to summarize article from <em>%s</em>. Error: %s<br/><br/><a href='%s' target='_blank'>View Original Article</a>",
			item.Title, err.Error(), item.Link.Href)
	}

	if summary == "" {
		return fmt.Sprintf("<strong>📰 Article Summary</strong><br/><br/>No summary available for <em>%s</em>.<br/><br/><a href='%s' target='_blank'>View Original Article</a>",
			item.Title, item.Link.Href)
	}

	// Clean up the summary and format it
	cleanSummary := strings.TrimSpace(summary)

	// Cache the summary result for future use
	cachedContent := CachedContent{
		ItemID:   itemID,
		Summary:  cleanSummary,
		CachedAt: time.Now(),
		TTL:      r.getCacheTTL(),
	}
	r.UpdateCachedContent(itemID, cachedContent)

	// Format the summary with HTML
	formattedSummary := r.formatArticleSummary(cleanSummary)
	return fmt.Sprintf("%s<br/><br/><a href='%s' target='_blank'>View Original Article</a>",
		formattedSummary, item.Link.Href)
}

// formatArticleSummary formats an AI-generated article summary with HTML
func (r *RSS) formatArticleSummary(summary string) string {
	if summary == "" {
		return ""
	}

	// Check if the summary contains both TL;DR and Detailed sections
	if strings.Contains(summary, "TL;DR:") || strings.Contains(summary, "**TL;DR:**") {
		// Replace markdown bold with HTML strong tags
		formatted := strings.ReplaceAll(summary, "**TL;DR:**", "<strong>🤖 TL;DR:</strong>")
		formatted = strings.ReplaceAll(formatted, "TL;DR:", "<strong>🤖 TL;DR:</strong>")
		formatted = strings.ReplaceAll(formatted, "**Detailed Summary:**", "<br/><br/><strong>📰 Detailed Summary:</strong>")
		formatted = strings.ReplaceAll(formatted, "Detailed Summary:", "<br/><br/><strong>📰 Detailed Summary:</strong>")

		// Convert newlines to HTML breaks
		formatted = strings.ReplaceAll(formatted, "\n\n", "<br/><br/>")
		formatted = strings.ReplaceAll(formatted, "\n", "<br/>")

		return "<strong>📰 Article Summary</strong><br/><br/>" + formatted
	} else {
		// Fallback for simple summaries
		formatted := strings.ReplaceAll(summary, "\n\n", "<br/><br/>")
		formatted = strings.ReplaceAll(formatted, "\n", "<br/>")
		return "<strong>📰 Article Summary</strong><br/><br/>" + formatted
	}
}

// resolveRedirects follows HTTP redirects to get the final destination URL
func (r *RSS) resolveRedirects(url string) string {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Allow up to 10 redirects
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	// Check if this is a Google News URL and try to extract the real URL
	if strings.Contains(url, "news.google.com/rss/articles/") {
		if extractedURL := r.extractGoogleNewsURL(url); extractedURL != "" {
			r.l.Info("Extracted URL from Google News", "original", url, "extracted", extractedURL)
			return extractedURL
		}
	}

	// Fallback to HTTP redirect for non-Google News URLs or if decoding fails
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		r.l.Error(err, "Failed to create GET request for redirect resolution", "url", url)
		return url
	}

	// Set a proper User-Agent to avoid being blocked
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		r.l.Error(err, "Failed to resolve redirects", "url", url)
		return url
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			r.l.Error(err, "Failed to close response body")
		}
	}()

	finalURL := resp.Request.URL.String()
	r.l.Info("HTTP redirect resolution", "original", url, "final", finalURL, "status", resp.StatusCode)

	return finalURL
}

// extractGoogleNewsURL fetches a Google News URL and extracts the real article URL from redirects
func (r *RSS) extractGoogleNewsURL(googleURL string) string {
	var redirectURL string

	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Capture the redirect URL but don't follow it
			redirectURL = req.URL.String()
			r.l.Info("Found HTTP redirect from Google News", "original", googleURL, "redirect", redirectURL)
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequest("GET", googleURL, nil)
	if err != nil {
		r.l.Error(err, "Error creating request for Google News URL", "url", googleURL)
		return ""
	}

	// Set headers to look like a real browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("DNT", "1")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")

	resp, err := client.Do(req)
	if err != nil {
		r.l.Error(err, "Error fetching Google News URL", "url", googleURL)
		return ""
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			r.l.Error(err, "Failed to close response body")
		}
	}()

	// If we found an HTTP redirect, check if it's still a Google News URL or a real article URL
	if redirectURL != "" && redirectURL != googleURL {
		// If it's still a Google News URL, try one more level of redirect
		if strings.Contains(redirectURL, "news.google.com") {
			r.l.Info("First redirect still Google News, trying second level", "url", redirectURL)
			if finalURL := r.followSecondLevelRedirect(redirectURL); finalURL != "" {
				return finalURL
			}
		} else {
			return redirectURL
		}
	}

	// If no HTTP redirect, read the response body and look for JavaScript/meta redirects
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		r.l.Error(err, "Error reading Google News response body", "url", googleURL)
		return ""
	}

	bodyStr := string(bodyBytes)
	r.l.Info("Fetched Google News page", "url", googleURL, "status", resp.StatusCode, "content_length", len(bodyStr))

	// Look for JavaScript redirect patterns
	patterns := []string{
		`window\.location\.href\s*=\s*['"](https?://[^'"]+)['"]`,
		`window\.location\s*=\s*['"](https?://[^'"]+)['"]`,
		`location\.href\s*=\s*['"](https?://[^'"]+)['"]`,
		`document\.location\s*=\s*['"](https?://[^'"]+)['"]`,
		`url\s*:\s*['"](https?://[^'"]+)['"]`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(bodyStr)
		if len(matches) > 1 {
			extractedURL := matches[1]
			r.l.Info("Found JavaScript redirect URL", "pattern", pattern, "url", extractedURL)
			return extractedURL
		}
	}

	// Also look for meta refresh redirects
	metaRefreshPattern := `<meta[^>]*http-equiv\s*=\s*["']refresh["'][^>]*content\s*=\s*["'][^;]*;\s*url\s*=\s*([^"']+)["'][^>]*>`
	re := regexp.MustCompile(metaRefreshPattern)
	matches := re.FindStringSubmatch(bodyStr)
	if len(matches) > 1 {
		extractedURL := matches[1]
		r.l.Info("Found meta refresh redirect URL", "url", extractedURL)
		return extractedURL
	}

	r.l.Info("No redirect URL found in Google News page", "url", googleURL)
	return ""
}

// followSecondLevelRedirect follows a second level redirect from Google News to get the final article URL
func (r *RSS) followSecondLevelRedirect(googleURL string) string {
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Allow one redirect to the actual article
			if len(via) > 1 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	req, err := http.NewRequest("GET", googleURL, nil)
	if err != nil {
		r.l.Error(err, "Error creating request for second level redirect", "url", googleURL)
		return ""
	}

	// Set headers to look like a real browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		r.l.Error(err, "Error following second level redirect", "url", googleURL)
		return ""
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			r.l.Error(err, "Failed to close response body")
		}
	}()

	finalURL := resp.Request.URL.String()
	r.l.Info("Second level redirect result", "original", googleURL, "final", finalURL, "status", resp.StatusCode)

	// Make sure we got to a real article URL, not still Google News
	if !strings.Contains(finalURL, "news.google.com") && finalURL != googleURL {
		return finalURL
	}

	return ""
}

// enhanceItemDeterministic enhances an RSS item using deterministic processing before LLM summarization
func (r *RSS) enhanceItemDeterministic(item *feeds.Item) (*EnhancedRSSItem, error) {
	// Check if we already have cached enhanced content
	itemID := item.Id
	if itemID == "" && item.Link != nil && item.Link.Href != "" {
		itemID = item.Link.Href
	}

	if itemID == "" {
		return nil, fmt.Errorf("no item ID or link available")
	}

	// Check cache first
	r.cacheMutex.RLock()
	cached, exists := r.cache[itemID]
	r.cacheMutex.RUnlock()

	if exists && !r.isCacheExpired(cached) && cached.EnhancedItem.FinalSummary != "" {
		r.l.Info("Using cached deterministic enhanced content", "itemID", itemID)
		return &cached.EnhancedItem, nil
	}

	// Fetch article content using RetrievePage tool if available
	var articleContent string
	var err error

	// Try to find an agent with RetrievePage tool
	var researchAgent *agent.Agent
	researchAgentIDs := []int{18, 17} // Try Research agent first, then Planning agent

	for _, agentID := range researchAgentIDs {
		if agent, exists := r.agents[agentID]; exists && agent != nil {
			// Check if agent has RetrievePage tool
			tools := agent.GetTools()
			for _, tool := range tools {
				if tool == "RetrievePage" {
					researchAgent = agent
					r.l.Info("Found research agent with RetrievePage tool", "agentID", agentID, "name", agent.Name)
					break
				}
			}
			if researchAgent != nil {
				break
			}
		}
	}

	if researchAgent != nil && item.Link != nil && item.Link.Href != "" {
		// Clone the agent to avoid shared state issues
		researchAgent = researchAgent.Clone()

		// Use the agent to fetch the article content
		prompt := fmt.Sprintf("Retrieve the content of this article: %s", item.Link.Href)
		result, err := researchAgent.GenerateWithTools("", agent.PromptInput{
			Message: prompt,
		})

		if err != nil {
			r.l.Error(err, "Failed to retrieve article content", "url", item.Link.Href)
		} else {
			// Extract the article content from the result
			// This would depend on how the RetrievePage tool formats its output
			articleContent = result
		}
	}

	// Extract metadata using deterministic functions
	metadata, err := extractArticleMetadata(articleContent)
	if err != nil {
		r.l.Error(err, "Failed to extract article metadata", "url", item.Link.Href)
		// Continue with empty metadata rather than failing completely
		metadata = ArticleMeta{}
	}

	// Clean HTML content
	cleanedContent, err := cleanHTMLContent(articleContent)
	if err != nil {
		r.l.Error(err, "Failed to clean HTML content", "url", item.Link.Href)
		// Use original content if cleaning fails
		cleanedContent = articleContent
	}

	// Extract key sections
	sections, err := extractKeySections(cleanedContent)
	if err != nil {
		r.l.Error(err, "Failed to extract key sections", "url", item.Link.Href)
		sections = make(map[string][]string)
	}

	// Search for related content using Searxng if available
	var relatedHits []SearchHit
	if r.config.SearxngURL != "" && r.config.UseDeterministicPreprocessing {
		// Create search query using article metadata
		searchQuery := fmt.Sprintf("related to %s %s", metadata.Title, strings.Join(metadata.Keywords, " "))

		// Perform search using Searxng
		searchResults, err := r.searchWithSearxng(searchQuery)
		if err != nil {
			r.l.Error(err, "Failed to search for related content with Searxng", "query", searchQuery)
		} else {
			r.l.Info("Successfully searched for related content", "query", searchQuery, "results", len(searchResults))
			relatedHits = searchResults
		}
	} else if researchAgent != nil && r.config.UseDeterministicPreprocessing {
		// Fallback to agent-based search if Searxng is not configured
		// Use the agent to search for related content
		searchQuery := fmt.Sprintf("related to %s %s", metadata.Title, strings.Join(metadata.Keywords, " "))
		prompt := fmt.Sprintf(`Use the SearchWeb tool to find related articles and information about: %s

Please format your response as a JSON array of search results with the following structure:
[
  {
    "title": "Article Title",
    "url": "https://example.com/article",
    "snippet": "Brief description of the article"
  }
]`, searchQuery)

		result, err := researchAgent.GenerateWithTools("", agent.PromptInput{
			Message: prompt,
		})

		if err != nil {
			r.l.Error(err, "Failed to search for related content", "query", searchQuery)
		} else {
			// Try to parse the search results as JSON
			// This would depend on how the SearchWeb tool formats its output
			r.l.Info("Search results received", "result_length", len(result))

			// For now, we'll create a simple placeholder for related hits
			// In a real implementation, we would parse the actual search results
			if len(result) > 0 {
				relatedHits = append(relatedHits, SearchHit{
					Title:   "Related Article",
					URL:     "https://example.com",
					Snippet: "This is a related article found through search",
				})
			}
		}
	}

	// Create enhanced item
	enhancedItem := &EnhancedRSSItem{
		OriginalItem:   item,
		ArticleContent: cleanedContent,
		Metadata:       metadata,
		RelatedHits:    relatedHits,
	}

	// Generate final summary using LLM with pre-processed content
	if researchAgent != nil {
		// Create a prompt with all the pre-processed information
		summaryPrompt := fmt.Sprintf(`Please provide a concise summary of the following article:

Title: %s
Author: %s
Published: %s
Word Count: %d
Keywords: %s

Article Content:
%s

Key Sections Identified:
%v

Related Content:
%v

Please provide a summary that captures the essential points of this article.`,
			metadata.Title,
			metadata.Author,
			metadata.PublishDate.Format("2006-01-02"),
			metadata.WordCount,
			strings.Join(metadata.Keywords, ", "),
			cleanedContent,
			sections,
			relatedHits)

		summary, err := researchAgent.GenerateWithTools("", agent.PromptInput{
			Message: summaryPrompt,
		})

		if err != nil {
			r.l.Error(err, "Failed to generate final summary")
			enhancedItem.ProcessingError = fmt.Sprintf("Failed to generate summary: %v", err)
		} else {
			enhancedItem.FinalSummary = summary
		}
	} else {
		enhancedItem.ProcessingError = "No research agent available for final summarization"
	}

	// Cache the enhanced item
	cachedContent := CachedContent{
		ItemID:       itemID,
		EnhancedItem: *enhancedItem,
		CachedAt:     time.Now(),
		TTL:          r.getCacheTTL(),
	}
	r.UpdateCachedContent(itemID, cachedContent)

	return enhancedItem, nil
}

// extractArticleMetadata extracts metadata from article content
func extractArticleMetadata(content string) (ArticleMeta, error) {
	meta := ArticleMeta{}

	if content == "" {
		return meta, nil
	}

	// Count words
	words := strings.Fields(content)
	meta.WordCount = len(words)

	// Extract keywords (simple approach - could be enhanced with NLP)
	meta.Keywords = extractTopKeywords(content, 10)

	// Try to extract title from <title> tag or <h1> tag
	titleRegex := regexp.MustCompile(`(?i)<title[^>]*>([^<]+)`)
	titleMatches := titleRegex.FindStringSubmatch(content)
	if len(titleMatches) > 1 {
		// Clean up the title by removing extra whitespace and common suffixes
		title := strings.TrimSpace(titleMatches[1])
		// Remove common site name suffixes
		siteSuffixRegex := regexp.MustCompile(`(?i)\s*[|-]\s*[^|]+\s*$`)
		title = siteSuffixRegex.ReplaceAllString(title, "")
		meta.Title = strings.TrimSpace(title)
	} else {
		// Try h1 tag as fallback
		h1Regex := regexp.MustCompile(`(?i)<h1[^>]*>([^<]+)`)
		h1Matches := h1Regex.FindStringSubmatch(content)
		if len(h1Matches) > 1 {
			meta.Title = strings.TrimSpace(h1Matches[1])
		} else {
			// Try og:title meta tag
			ogTitleRegex := regexp.MustCompile(`(?i)<meta[^>]*property\s*=\s*["']og:title["'][^>]*content\s*=\s*["']([^"']+)["']`)
			ogTitleMatches := ogTitleRegex.FindStringSubmatch(content)
			if len(ogTitleMatches) > 1 {
				meta.Title = strings.TrimSpace(ogTitleMatches[1])
			} else {
				meta.Title = "Untitled Article"
			}
		}
	}

	// Try to extract author from common author meta tags or bylines
	authorRegex := regexp.MustCompile(`(?i)<meta[^>]*name\s*=\s*["']author["'][^>]*content\s*=\s*["']([^"']+)["']`)
	authorMatches := authorRegex.FindStringSubmatch(content)
	if len(authorMatches) > 1 {
		meta.Author = strings.TrimSpace(authorMatches[1])
	} else {
		// Try og:author meta tag
		ogAuthorRegex := regexp.MustCompile(`(?i)<meta[^>]*property\s*=\s*["']article:author["'][^>]*content\s*=\s*["']([^"']+)["']`)
		ogAuthorMatches := ogAuthorRegex.FindStringSubmatch(content)
		if len(ogAuthorMatches) > 1 {
			meta.Author = strings.TrimSpace(ogAuthorMatches[1])
		} else {
			// Try byline patterns
			bylineRegex := regexp.MustCompile(`(?i)(?:by|author)[:\s]+([^<>\n\r]{3,50})`)
			bylineMatches := bylineRegex.FindStringSubmatch(content)
			if len(bylineMatches) > 1 {
				meta.Author = strings.TrimSpace(bylineMatches[1])
			} else {
				// Try itemprop author
				itempropRegex := regexp.MustCompile(`(?i)<[^>]*itemprop\s*=\s*["']author["'][^>]*>([^<]+)`)
				itempropMatches := itempropRegex.FindStringSubmatch(content)
				if len(itempropMatches) > 1 {
					meta.Author = strings.TrimSpace(itempropMatches[1])
				} else {
					meta.Author = "Unknown Author"
				}
			}
		}
	}

	// Try to extract publish date from common date meta tags or patterns
	dateRegex := regexp.MustCompile(`(?i)<meta[^>]*name\s*=\s*["'](date|publishdate|publicationdate)["'][^>]*content\s*=\s*["']([^"']+)["']`)
	dateMatches := dateRegex.FindStringSubmatch(content)
	if len(dateMatches) > 2 {
		if parsedTime, err := time.Parse(time.RFC3339, dateMatches[2]); err == nil {
			meta.PublishDate = parsedTime
		} else if parsedTime, err := time.Parse("2006-01-02", dateMatches[2]); err == nil {
			meta.PublishDate = parsedTime
		} else {
			meta.PublishDate = time.Now()
		}
	} else {
		// Try og:article:published_time meta tag
		ogDateRegex := regexp.MustCompile(`(?i)<meta[^>]*property\s*=\s*["']article:published_time["'][^>]*content\s*=\s*["']([^"']+)["']`)
		ogDateMatches := ogDateRegex.FindStringSubmatch(content)
		if len(ogDateMatches) > 1 {
			if parsedTime, err := time.Parse(time.RFC3339, ogDateMatches[1]); err == nil {
				meta.PublishDate = parsedTime
			} else {
				meta.PublishDate = time.Now()
			}
		} else {
			// Try itemprop datePublished
			itempropDateRegex := regexp.MustCompile(`(?i)<[^>]*itemprop\s*=\s*["']datePublished["'][^>]*datetime\s*=\s*["']([^"']+)["']`)
			itempropDateMatches := itempropDateRegex.FindStringSubmatch(content)
			if len(itempropDateMatches) > 1 {
				if parsedTime, err := time.Parse(time.RFC3339, itempropDateMatches[1]); err == nil {
					meta.PublishDate = parsedTime
				} else if parsedTime, err := time.Parse("2006-01-02", itempropDateMatches[1]); err == nil {
					meta.PublishDate = parsedTime
				} else {
					meta.PublishDate = time.Now()
				}
			} else {
				// Try common date patterns in content
				datePatternRegex := regexp.MustCompile(`(?i)(\d{4}[-/]\d{1,2}[-/]\d{1,2}|\d{1,2}[-/]\d{1,2}[-/]\d{4})`)
				datePatternMatches := datePatternRegex.FindStringSubmatch(content)
				if len(datePatternMatches) > 1 {
					if parsedTime, err := time.Parse("2006-01-02", datePatternMatches[1]); err == nil {
						meta.PublishDate = parsedTime
					} else if parsedTime, err := time.Parse("01/02/2006", datePatternMatches[1]); err == nil {
						meta.PublishDate = parsedTime
					} else {
						meta.PublishDate = time.Now()
					}
				} else {
					meta.PublishDate = time.Now()
				}
			}
		}
	}

	return meta, nil
}

// cleanHTMLContent removes unwanted HTML elements from content
func cleanHTMLContent(content string) (string, error) {
	if content == "" {
		return "", nil
	}

	// Remove script and style tags
	scriptRegex := regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`)
	content = scriptRegex.ReplaceAllString(content, "")

	styleRegex := regexp.MustCompile(`(?i)<style[^>]*>.*?</style>`)
	content = styleRegex.ReplaceAllString(content, "")

	// Remove navigation and header elements
	navRegex := regexp.MustCompile(`(?i)<nav[^>]*>.*?</nav>`)
	content = navRegex.ReplaceAllString(content, "")

	headerRegex := regexp.MustCompile(`(?i)<header[^>]*>.*?</header>`)
	content = headerRegex.ReplaceAllString(content, "")

	footerRegex := regexp.MustCompile(`(?i)<footer[^>]*>.*?</footer>`)
	content = footerRegex.ReplaceAllString(content, "")

	// Remove advertisements and sidebars
	adRegex := regexp.MustCompile(`(?i)<aside[^>]*>.*?</aside>`)
	content = adRegex.ReplaceAllString(content, "")

	// Remove divs with common ad/sidebar classes
	adClassRegex := regexp.MustCompile(`(?i)<div[^>]*class\s*=\s*["'][^"']*(ad|advertisement|sidebar|menu|navigation)[^"']*["'][^>]*>.*?</div>`)
	content = adClassRegex.ReplaceAllString(content, "")

	// Remove common ad containers
	adIdRegex := regexp.MustCompile(`(?i)<div[^>]*id\s*=\s*["'][^"']*(ad|advertisement|sidebar)[^"']*["'][^>]*>.*?</div>`)
	content = adIdRegex.ReplaceAllString(content, "")

	// Remove tracking pixels and hidden elements
	trackingRegex := regexp.MustCompile(`(?i)<img[^>]*((width\s*=\s*["']1["'])|(height\s*=\s*["']1["'])|(display\s*:\s*none)).*?>`)
	content = trackingRegex.ReplaceAllString(content, "")

	hiddenRegex := regexp.MustCompile(`(?i)<[^>]*style\s*=\s*["'][^"']*display\s*:\s*none[^"']*["'][^>]*>.*?</[^>]+>`)
	content = hiddenRegex.ReplaceAllString(content, "")

	// Remove comments
	commentRegex := regexp.MustCompile(`(?s)<!--.*?-->`)
	content = commentRegex.ReplaceAllString(content, "")

	// Remove multiple whitespace and normalize
	whitespaceRegex := regexp.MustCompile(`\s+`)
	content = whitespaceRegex.ReplaceAllString(content, " ")

	// Trim leading/trailing whitespace
	content = strings.TrimSpace(content)

	return content, nil
}

// extractKeySections identifies key sections like headings, lists, and quotes
func extractKeySections(content string) (map[string][]string, error) {
	sections := make(map[string][]string)

	if content == "" {
		return sections, nil
	}

	// Extract headings (h1-h6) with hierarchy
	for i := 1; i <= 6; i++ {
		headingRegex := regexp.MustCompile(fmt.Sprintf(`(?i)<h%d[^>]*>(.*?)</h%d>`, i, i))
		headingMatches := headingRegex.FindAllStringSubmatch(content, -1)
		for _, match := range headingMatches {
			if len(match) > 1 {
				key := fmt.Sprintf("h%d", i)
				sections[key] = append(sections[key], match[1])
			}
		}
	}

	// Extract lists (ul, ol) with content
	listRegex := regexp.MustCompile(`(?i)<li[^>]*>(.*?)</li>`)
	listMatches := listRegex.FindAllStringSubmatch(content, -1)
	for _, match := range listMatches {
		if len(match) > 1 {
			sections["lists"] = append(sections["lists"], match[1])
		}
	}

	// Extract blockquotes
	quoteRegex := regexp.MustCompile(`(?i)<blockquote[^>]*>(.*?)</blockquote>`)
	quoteMatches := quoteRegex.FindAllStringSubmatch(content, -1)
	for _, match := range quoteMatches {
		if len(match) > 1 {
			sections["quotes"] = append(sections["quotes"], match[1])
		}
	}

	// Extract paragraphs (first few significant ones)
	paraRegex := regexp.MustCompile(`(?i)<p[^>]*>(.*?)</p>`)
	paraMatches := paraRegex.FindAllStringSubmatch(content, -1)
	paraCount := 0
	for _, match := range paraMatches {
		if len(match) > 1 && paraCount < 5 { // Only first 5 paragraphs
			// Clean paragraph content
			paraContent := match[1]
			// Remove extra whitespace
			paraContent = regexp.MustCompile(`\s+`).ReplaceAllString(paraContent, " ")
			paraContent = strings.TrimSpace(paraContent)

			// Only include paragraphs with substantial content
			if len(strings.Fields(paraContent)) > 5 {
				sections["paragraphs"] = append(sections["paragraphs"], paraContent)
				paraCount++
			}
		}
	}

	// Extract code blocks
	codeRegex := regexp.MustCompile(`(?i)<(pre|code)[^>]*>(.*?)</(?:pre|code)>`)
	codeMatches := codeRegex.FindAllStringSubmatch(content, -1)
	for _, match := range codeMatches {
		if len(match) > 2 {
			sections["code"] = append(sections["code"], match[2])
		}
	}

	// Extract images with alt text
	imgRegex := regexp.MustCompile(`(?i)<img[^>]*alt\s*=\s*["']([^"']*)["'][^>]*>`)
	imgMatches := imgRegex.FindAllStringSubmatch(content, -1)
	for _, match := range imgMatches {
		if len(match) > 1 && strings.TrimSpace(match[1]) != "" {
			sections["images"] = append(sections["images"], match[1])
		}
	}

	return sections, nil
}

// searchWithSearxng performs a search using Searxng API
func (r *RSS) searchWithSearxng(query string) ([]SearchHit, error) {
	if r.config.SearxngURL == "" {
		return nil, fmt.Errorf("SearxngURL not configured")
	}

	// Construct search URL
	searchURL := fmt.Sprintf("%s?q=%s&format=json&categories=general",
		strings.TrimRight(r.config.SearxngURL, "/"),
		url.QueryEscape(query))

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Create request
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set User-Agent to avoid blocking
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MuleAI-RSS/1.0; +http://localhost:8083)")

	// Perform request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform search: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			r.l.Error(err, "Failed to close response body")
		}
	}()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search returned status %d", resp.StatusCode)
	}

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse JSON response
	var searchResult struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}

	if err := json.Unmarshal(body, &searchResult); err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	// Convert to SearchHit format
	var hits []SearchHit
	for _, result := range searchResult.Results {
		// Only include results with substantial content
		if len(strings.Fields(result.Content)) > 5 {
			hits = append(hits, SearchHit{
				Title:   result.Title,
				URL:     result.URL,
				Snippet: result.Content,
			})
		}

		// Limit to top 10 results
		if len(hits) >= 10 {
			break
		}
	}

	return hits, nil
}

// extractTopKeywords extracts the most frequent words from content as keywords
func extractTopKeywords(content string, count int) []string {
	if content == "" || count <= 0 {
		return []string{}
	}

	// Convert to lowercase and remove HTML tags
	content = strings.ToLower(content)
	tagRegex := regexp.MustCompile(`<[^>]*>`)
	content = tagRegex.ReplaceAllString(content, " ")

	// Remove punctuation and split into words
	punctRegex := regexp.MustCompile(`[^\w\s]`)
	content = punctRegex.ReplaceAllString(content, " ")
	words := strings.Fields(content)

	// Filter out common stop words
	stopWords := map[string]bool{
		"the": true, "and": true, "or": true, "but": true, "in": true, "on": true, "at": true,
		"to": true, "for": true, "of": true, "with": true, "by": true, "a": true, "an": true,
		"is": true, "are": true, "was": true, "were": true, "be": true, "been": true, "have": true,
		"has": true, "had": true, "do": true, "does": true, "did": true, "will": true, "would": true,
		"could": true, "should": true, "may": true, "might": true, "must": true, "can": true,
		"this": true, "that": true, "these": true, "those": true, "i": true, "you": true, "he": true,
		"she": true, "it": true, "we": true, "they": true, "me": true, "him": true, "her": true,
		"us": true, "them": true, "my": true, "your": true, "his": true, "its": true, "our": true,
		"their": true, "myself": true, "yourself": true, "himself": true, "herself": true,
		"itself": true, "ourselves": true, "yourselves": true, "themselves": true,
	}

	// Count word frequencies
	wordFreq := make(map[string]int)
	for _, word := range words {
		if len(word) > 3 && !stopWords[word] { // Only consider words longer than 3 characters and not stop words
			wordFreq[word]++
		}
	}

	// Convert to slice for sorting
	type wordCount struct {
		word  string
		count int
	}

	wordCounts := make([]wordCount, 0, len(wordFreq))
	for word, count := range wordFreq {
		wordCounts = append(wordCounts, wordCount{word, count})
	}

	// Sort by frequency (descending) using more efficient algorithm
	sort.Slice(wordCounts, func(i, j int) bool {
		return wordCounts[i].count > wordCounts[j].count
	})

	// Take top N keywords
	var keywords []string
	for i := 0; i < len(wordCounts) && i < count; i++ {
		keywords = append(keywords, wordCounts[i].word)
	}

	return keywords
}
