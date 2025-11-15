package rss_workflow

import (
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/mule-ai/mule/pkg/agent"
)

func TestRSSWorkflowCreation(t *testing.T) {
	logger := logr.Discard()
	config := DefaultConfig()
	agents := make(map[int]*agent.Agent)
	workflow := New(config, logger, agents)

	if workflow == nil {
		t.Fatal("Expected workflow to be created")
	}

	if workflow.Name() != "rss_workflow" {
		t.Errorf("Expected name to be 'rss_workflow', got '%s'", workflow.Name())
	}
}

func TestRSSItemStruct(t *testing.T) {
	item := &RSSItem{
		Title:       "Test Title",
		Description: "Test Description",
		Link:        "https://example.com",
		Author:      "Test Author",
		PublishDate: time.Now(),
		ID:          "test-id",
	}

	if item.Title != "Test Title" {
		t.Errorf("Expected title to be 'Test Title', got '%s'", item.Title)
	}
}

func TestEnhancedRSSItemStruct(t *testing.T) {
	originalItem := &RSSItem{
		Title:       "Test Title",
		Description: "Test Description",
		Link:        "https://example.com",
		Author:      "Test Author",
		PublishDate: time.Now(),
		ID:          "test-id",
	}

	enhancedItem := &EnhancedRSSItem{
		OriginalItem:    originalItem,
		ArticleContent:  "Test content",
		Metadata:        ArticleMeta{},
		RelatedHits:     []SearchHit{},
		FinalSummary:    "Test summary",
		ProcessingError: "",
		CachedAt:        time.Now(),
		TTL:             6 * time.Hour,
	}

	if enhancedItem.OriginalItem.Title != "Test Title" {
		t.Errorf("Expected original item title to be 'Test Title', got '%s'", enhancedItem.OriginalItem.Title)
	}

	if enhancedItem.FinalSummary != "Test summary" {
		t.Errorf("Expected final summary to be 'Test summary', got '%s'", enhancedItem.FinalSummary)
	}
}

func TestCachedContentStruct(t *testing.T) {
	originalItem := &RSSItem{
		Title:       "Test Title",
		Description: "Test Description",
		Link:        "https://example.com",
		Author:      "Test Author",
		PublishDate: time.Now(),
		ID:          "test-id",
	}

	enhancedItem := &EnhancedRSSItem{
		OriginalItem:    originalItem,
		ArticleContent:  "Test content",
		Metadata:        ArticleMeta{},
		RelatedHits:     []SearchHit{},
		FinalSummary:    "Test summary",
		ProcessingError: "",
		CachedAt:        time.Now(),
		TTL:             6 * time.Hour,
	}

	cachedContent := &CachedContent{
		ItemID:          "test-id",
		EnhancedContent: "Test enhanced content",
		EnhancedItem:    *enhancedItem,
		CachedAt:        time.Now(),
		TTL:             6 * time.Hour,
	}

	if cachedContent.ItemID != "test-id" {
		t.Errorf("Expected item ID to be 'test-id', got '%s'", cachedContent.ItemID)
	}

	if cachedContent.EnhancedContent != "Test enhanced content" {
		t.Errorf("Expected enhanced content to be 'Test enhanced content', got '%s'", cachedContent.EnhancedContent)
	}
}
