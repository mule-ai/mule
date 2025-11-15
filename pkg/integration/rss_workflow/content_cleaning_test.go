package rss_workflow

import (
	"strings"
	"testing"
)

func TestContentCleaningStepCreation(t *testing.T) {
	step := NewContentCleaningStep()

	if step == nil {
		t.Fatal("Expected content cleaning step to be created")
	}
}

func TestCleanContent(t *testing.T) {
	step := NewContentCleaningStep()

	// Test content with script tags
	content := "<html><head><script>alert('test');</script></head><body><p>This is test content</p></body></html>"
	cleaned, err := step.CleanContent(content)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if strings.Contains(cleaned, "<script>") {
		t.Error("Expected script tags to be removed")
	}

	if !strings.Contains(cleaned, "This is test content") {
		t.Error("Expected content to be preserved")
	}
}

func TestCleanContentEmpty(t *testing.T) {
	step := NewContentCleaningStep()

	cleaned, err := step.CleanContent("")

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if cleaned != "" {
		t.Errorf("Expected empty string, got '%s'", cleaned)
	}
}
