package rss_workflow

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/gorilla/feeds"
	"github.com/mule-ai/mule/pkg/agent"
	"github.com/mule-ai/mule/pkg/integration/rss_monitor"
	"github.com/mule-ai/mule/pkg/types"
)

// RSSItem represents an RSS/Atom feed item
type RSSItem struct {
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Link        string    `json:"link"`
	Author      string    `json:"author"`
	PublishDate time.Time `json:"publishDate"`
	ID          string    `json:"id"`
}

// ArticleMeta represents metadata extracted from an article
type ArticleMeta struct {
	Title       string    `json:"title"`
	Author      string    `json:"author"`
	PublishDate time.Time `json:"publishDate"`
	WordCount   int       `json:"wordCount"`
	Keywords    []string  `json:"keywords"`
}

// SearchHit represents a search result from Searxng or similar services
type SearchHit struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// EnhancedRSSItem represents an RSS item with enhanced content
type EnhancedRSSItem struct {
	OriginalItem    *RSSItem      `json:"originalItem"`
	ArticleContent  string        `json:"articleContent"`  // Cleaned article text
	Metadata        ArticleMeta   `json:"metadata"`        // Extracted metadata
	RelatedHits     []SearchHit   `json:"relatedHits"`     // Top search results
	FinalSummary    string        `json:"finalSummary"`    // LLM-generated summary
	ProcessingError string        `json:"processingError"` // Any error during processing
	CachedAt        time.Time     `json:"cachedAt"`        // When this item was cached
	TTL             time.Duration `json:"ttl"`             // How long this item should be cached
}

// CachedContent represents cached enhanced content for an RSS item
type CachedContent struct {
	ItemID          string          `json:"itemId"`
	EnhancedContent string          `json:"enhancedContent"`
	EnhancedItem    EnhancedRSSItem `json:"enhancedItem,omitempty"`
	CachedAt        time.Time       `json:"cachedAt"`
	TTL             time.Duration   `json:"ttl"` // TTL duration
}

// RSSWorkflow represents the RSS enhancement workflow integration
type RSSWorkflow struct {
	logger     logr.Logger
	channel    chan any
	triggers   map[string]chan any
	cache      map[string]CachedContent // Cache for enhanced content
	cacheMutex sync.RWMutex             // Mutex for thread-safe cache access
	agents     map[int]*agent.Agent     // Available agents for enhancement
	config     *Config                  // Configuration for the workflow
}

// New creates a new RSS workflow integration instance
func New(config *Config, logger logr.Logger, agents map[int]*agent.Agent) *RSSWorkflow {
	if config == nil {
		config = DefaultConfig()
	}

	r := &RSSWorkflow{
		logger:   logger,
		channel:  make(chan any, 100), // Buffered channel to prevent blocking
		triggers: make(map[string]chan any),
		cache:    make(map[string]CachedContent),
		agents:   agents,
		config:   config,
	}

	logger.Info("RSS workflow integration created", "agentID", config.AgentID)
	go r.receiveTriggers()
	return r
}

// Name returns the name of the integration
func (r *RSSWorkflow) Name() string {
	return "rss_workflow"
}

// GetChannel returns the channel for internal triggers
func (r *RSSWorkflow) GetChannel() chan any {
	return r.channel
}

// RegisterTrigger registers a channel for a specific trigger
func (r *RSSWorkflow) RegisterTrigger(trigger string, data any, channel chan any) {
	triggerKey := trigger
	if dataStr, ok := data.(string); ok && dataStr != "" {
		triggerKey = trigger + dataStr
	}

	r.triggers[triggerKey] = channel
	r.logger.Info("Registered trigger", "key", triggerKey)
}

// createFeedItem creates a feeds.Item from an EnhancedRSSItem
func (r *RSSWorkflow) createFeedItem(enhancedItem *EnhancedRSSItem) *feeds.Item {
	// Create a more informative description that includes both the summary and related content
	description := enhancedItem.FinalSummary

	// Add related content if available
	if len(enhancedItem.RelatedHits) > 0 {
		description += "\n\nRelated Articles:\n"
		for i, hit := range enhancedItem.RelatedHits {
			if i >= 5 { // Limit to top 5 related articles
				break
			}
			description += fmt.Sprintf("%d. %s (%s)\n", i+1, hit.Title, hit.URL)
		}
	}

	// Add processing error info if there was one
	if enhancedItem.ProcessingError != "" {
		description += fmt.Sprintf("\n\nNote: Processing issue - %s", enhancedItem.ProcessingError)
	}

	return &feeds.Item{
		Title:       fmt.Sprintf("[Enhanced] %s", enhancedItem.OriginalItem.Title),
		Link:        &feeds.Link{Href: enhancedItem.OriginalItem.Link},
		Description: description,
		Author:      &feeds.Author{Name: enhancedItem.OriginalItem.Author},
		Created:     enhancedItem.OriginalItem.PublishDate,
		Id:          enhancedItem.OriginalItem.ID,
	}
}

// AddItemToHost adds an enhanced item to an RSS host
func (r *RSSWorkflow) AddItemToHost(enhancedItem *EnhancedRSSItem, hostChannel chan any) {
	// Create a feeds.Item for the RSS host
	feedItem := r.createFeedItem(enhancedItem)

	// Create a trigger settings for the RSS host
	triggerSettings := &types.TriggerSettings{
		Integration: "rss_host",
		Event:       "addItem",
		Data:        feedItem,
	}

	// Send to the RSS host
	select {
	case hostChannel <- triggerSettings:
		r.logger.Info("Sent enhanced item to RSS host")
	default:
		r.logger.Error(fmt.Errorf("channel is blocking"), "Failed to send enhanced item to RSS host")
	}
}

// Call is a generic method for extensions
func (r *RSSWorkflow) Call(name string, data any) (any, error) {
	switch name {
	case "extractContent":
		return r.extractContent(data)
	case "extractMetadata":
		return r.extractMetadata(data)
	case "cleanContent":
		return r.cleanContent(data)
	case "searchRelated":
		return r.searchRelated(data)
	case "summarize":
		return r.summarize(data)
	case "enhanceItem":
		return r.enhanceItem(data)
	case "convertToFeedItem":
		// Convert EnhancedRSSItem to feeds.Item
		enhancedItem, ok := data.(*EnhancedRSSItem)
		if !ok {
			return nil, fmt.Errorf("data must be an EnhancedRSSItem")
		}
		feedItem := r.createFeedItem(enhancedItem)
		return feedItem, nil
	default:
		return nil, fmt.Errorf("method '%s' not implemented", name)
	}
}

// GetChatHistory returns empty string as RSS workflow doesn't maintain chat history
func (r *RSSWorkflow) GetChatHistory(channelID string, limit int) (string, error) {
	return "", nil
}

// ClearChatHistory does nothing as RSS workflow doesn't maintain chat history
func (r *RSSWorkflow) ClearChatHistory(channelID string) error {
	return nil
}

// receiveTriggers listens on the internal channel for actions to perform
func (r *RSSWorkflow) receiveTriggers() {
	for trigger := range r.channel {
		r.logger.Info("Received trigger in RSS workflow", "triggerType", fmt.Sprintf("%T", trigger))
		triggerSettings, ok := trigger.(*types.TriggerSettings)
		if !ok {
			r.logger.Error(fmt.Errorf("trigger is not a TriggerSettings"), "Trigger is not a TriggerSettings")
			continue
		}
		if triggerSettings.Integration != "rss_workflow" {
			r.logger.Error(fmt.Errorf("trigger integration is not rss_workflow"), "Trigger integration is not rss_workflow")
			continue
		}

		// Handle triggers
		switch triggerSettings.Event {
		case "newItem":
			// Handle new RSS item from monitor
			r.logger.Info("Processing new RSS item", "data_type", fmt.Sprintf("%T", triggerSettings.Data))

			// Convert rss_monitor.Item to rss_workflow.RSSItem
			monitorItem, ok := triggerSettings.Data.(rss_monitor.Item)
			if !ok {
				// Try pointer version
				if itemPtr, ok := triggerSettings.Data.(*rss_monitor.Item); ok {
					monitorItem = *itemPtr
				} else {
					r.logger.Error(fmt.Errorf("data is not an rss_monitor.Item"), "Failed to extract RSS item from trigger")
					continue
				}
			}

			// Convert to RSSItem format
			workflowItem := &RSSItem{
				Title:       monitorItem.Title,
				Description: monitorItem.Description,
				Link:        monitorItem.Link,
				Author:      monitorItem.Author,
				PublishDate: monitorItem.PublishDate,
				ID:          monitorItem.ID,
			}

			// Enhance the item
			result, err := r.enhanceItem(workflowItem)
			if err != nil {
				r.logger.Error(err, "Failed to enhance RSS item")
				continue
			}

			// Get the enhanced item
			enhancedItem, ok := result.(*EnhancedRSSItem)
			if !ok {
				r.logger.Error(fmt.Errorf("result is not an EnhancedRSSItem"), "Failed to get enhanced item")
				continue
			}

			// Convert enhanced item to feeds.Item and send to output channels
			feedItem := r.createFeedItem(enhancedItem)

			triggerSettings := &types.TriggerSettings{
				Integration: "rss_host",
				Event:       "addItem",
				Data:        feedItem,
			}

			// Send to output channels if any
			for _, channel := range r.triggers {
				select {
				case channel <- triggerSettings:
					r.logger.Info("Sent enhanced item to trigger channel")
				default:
					r.logger.Error(fmt.Errorf("channel is blocking"), "Failed to send enhanced item to trigger channel")
				}
			}
		default:
			r.logger.Error(fmt.Errorf("trigger event not supported: %s", triggerSettings.Event), "Unsupported trigger event")
		}
	}
}

// extractContent extracts article content from a URL using RetrievePage tool
func (r *RSSWorkflow) extractContent(data any) (any, error) {
	// Convert data to string (URL)
	_, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("data must be a string URL")
	}

	// Create a temporary agent for content extraction
	// In practice, this would be injected or configured
	return nil, fmt.Errorf("content extraction requires an agent with RetrievePage tool")
}

// extractMetadata extracts metadata from article content
func (r *RSSWorkflow) extractMetadata(data any) (any, error) {
	// Convert data to string (content)
	content, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("data must be a string content")
	}

	step := NewMetadataExtractionStep()
	meta, err := step.ExtractMetadata(content)
	if err != nil {
		return nil, fmt.Errorf("failed to extract metadata: %w", err)
	}

	return meta, nil
}

// cleanContent removes unwanted HTML elements from content
func (r *RSSWorkflow) cleanContent(data any) (any, error) {
	// Convert data to string (content)
	content, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("data must be a string content")
	}

	step := NewContentCleaningStep()
	cleanedContent, err := step.CleanContent(content)
	if err != nil {
		return nil, fmt.Errorf("failed to clean content: %w", err)
	}

	return cleanedContent, nil
}

// searchRelated searches for related content using search tools
func (r *RSSWorkflow) searchRelated(data any) (any, error) {
	// Convert data to string (search query)
	query, ok := data.(string)
	if !ok {
		return nil, fmt.Errorf("data must be a string query")
	}

	// Create a temporary step for related search
	// In practice, this would use a configured agent or Searxng URL
	step := NewRelatedSearchStep(nil, "")
	results, err := step.SearchRelated(query)
	if err != nil {
		return nil, fmt.Errorf("failed to search related content: %w", err)
	}

	return results, nil
}

// summarize generates an AI summary of content
func (r *RSSWorkflow) summarize(data any) (any, error) {
	// Create a temporary step for summarization
	// In practice, this would use a configured agent
	step := NewSummarizationStep(nil)

	// Convert data to EnhancedRSSItem
	item, ok := data.(*EnhancedRSSItem)
	if !ok {
		return nil, fmt.Errorf("data must be an EnhancedRSSItem")
	}

	summary, err := step.Summarize(item)
	if err != nil {
		return nil, fmt.Errorf("failed to generate summary: %w", err)
	}

	return summary, nil
}

// enhanceItem enhances an RSS item with all steps
func (r *RSSWorkflow) enhanceItem(data any) (any, error) {
	// Convert data to RSSItem
	originalItem, ok := data.(*RSSItem)
	if !ok {
		return nil, fmt.Errorf("data must be an RSSItem")
	}

	// Check cache first
	cached, exists := r.getCachedContent(originalItem.ID)
	if exists {
		r.logger.Info("Using cached enhanced content", "itemID", originalItem.ID)
		return &cached.EnhancedItem, nil
	}

	// Create enhanced item
	enhancedItem := &EnhancedRSSItem{
		OriginalItem: originalItem,
		CachedAt:     time.Now(),
		TTL:          r.config.CacheTTL,
	}

	// Get the agent for enhancement
	enhancementAgent, exists := r.agents[r.config.AgentID]
	if !exists {
		enhancedItem.ProcessingError = fmt.Sprintf("Agent %d not found for enhancement", r.config.AgentID)
		r.logger.Error(fmt.Errorf("agent not found"), "Agent not found for enhancement", "agentID", r.config.AgentID)
	} else {
		// Apply all enhancement steps
		if err := r.applyEnhancementSteps(enhancedItem, enhancementAgent); err != nil {
			enhancedItem.ProcessingError = fmt.Sprintf("Enhancement failed: %v", err)
			r.logger.Error(err, "Failed to apply enhancement steps", "itemID", originalItem.ID)
		}
	}

	// Provide fallback summary if needed
	if enhancedItem.FinalSummary == "" {
		// Try to use the original description
		if originalItem.Description != "" {
			enhancedItem.FinalSummary = originalItem.Description
		} else {
			// Last resort fallback
			enhancedItem.FinalSummary = fmt.Sprintf("No summary available for \"%s\"", originalItem.Title)
		}
	}

	// Cache the result
	cachedContent := CachedContent{
		ItemID:       originalItem.ID,
		EnhancedItem: *enhancedItem,
		CachedAt:     time.Now(),
		TTL:          r.config.CacheTTL,
	}
	r.setCachedContent(originalItem.ID, cachedContent)

	return enhancedItem, nil
}

// applyEnhancementSteps applies all enhancement steps to an item
func (r *RSSWorkflow) applyEnhancementSteps(enhancedItem *EnhancedRSSItem, agent *agent.Agent) error {
	// Content extraction
	if enhancedItem.OriginalItem.Link != "" {
		contentExtractionStep := NewContentExtractionStep(agent, r.logger)
		if err := contentExtractionStep.Process(enhancedItem); err != nil {
			r.logger.Error(err, "Content extraction failed", "itemID", enhancedItem.OriginalItem.ID, "link", enhancedItem.OriginalItem.Link)
			// Continue processing even if content extraction fails - we'll use fallbacks
		}
	}

	// Metadata extraction
	if enhancedItem.ArticleContent != "" {
		metadataExtractionStep := NewMetadataExtractionStep()
		if err := metadataExtractionStep.Process(enhancedItem); err != nil {
			r.logger.Error(err, "Metadata extraction failed", "itemID", enhancedItem.OriginalItem.ID)
			// Continue processing even if metadata extraction fails
		}
	}

	// Content cleaning
	if enhancedItem.ArticleContent != "" {
		contentCleaningStep := NewContentCleaningStep()
		if err := contentCleaningStep.Process(enhancedItem); err != nil {
			r.logger.Error(err, "Content cleaning failed", "itemID", enhancedItem.OriginalItem.ID)
			// Continue processing even if content cleaning fails
		}
	}

	// Related content search
	if r.config.SearxngURL != "" && enhancedItem.Metadata.Title != "" {
		relatedSearchStep := NewRelatedSearchStep(agent, r.config.SearxngURL)
		if err := relatedSearchStep.Process(enhancedItem); err != nil {
			r.logger.Error(err, "Related content search failed", "itemID", enhancedItem.OriginalItem.ID)
			// Continue processing even if related search fails
		}
	}

	// Summarization
	summarizationStep := NewSummarizationStep(agent)
	if err := summarizationStep.Process(enhancedItem); err != nil {
		r.logger.Error(err, "Summarization failed", "itemID", enhancedItem.OriginalItem.ID)
		// If summarization fails, we'll use a fallback in the enhanceItem function
	}

	return nil
}

// getCachedContent retrieves cached content for an item ID
func (r *RSSWorkflow) getCachedContent(itemID string) (*CachedContent, bool) {
	r.cacheMutex.RLock()
	defer r.cacheMutex.RUnlock()

	cached, exists := r.cache[itemID]
	if !exists {
		return nil, false
	}

	// Check if cache has expired
	if time.Now().After(cached.CachedAt.Add(cached.TTL)) {
		// Note: We're not deleting the expired entry here to avoid requiring a write lock
		// The deletion will happen in setCachedContent when a new entry is added
		return nil, false
	}

	return &cached, true
}

// setCachedContent stores enhanced content in cache
func (r *RSSWorkflow) setCachedContent(itemID string, content CachedContent) {
	r.cacheMutex.Lock()
	defer r.cacheMutex.Unlock()

	// Check if there's an existing entry that has expired
	if existing, exists := r.cache[itemID]; exists {
		if time.Now().After(existing.CachedAt.Add(existing.TTL)) {
			// Remove expired entry
			delete(r.cache, itemID)
		}
	}

	r.cache[itemID] = content
}
