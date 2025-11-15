package rss_workflow

import (
	"testing"
)

func TestMetadataExtractionStepCreation(t *testing.T) {
	step := NewMetadataExtractionStep()

	if step == nil {
		t.Fatal("Expected metadata extraction step to be created")
	}
}

func TestExtractTopKeywords(t *testing.T) {
	step := NewMetadataExtractionStep()

	content := "This is a test article about artificial intelligence and machine learning technologies"
	keywords := step.extractTopKeywords(content, 5)

	if len(keywords) == 0 {
		t.Error("Expected to extract keywords")
	}

	// Check that common stop words are filtered out
	for _, keyword := range keywords {
		if keyword == "this" || keyword == "is" || keyword == "a" {
			t.Errorf("Expected stop words to be filtered out, but found '%s'", keyword)
		}
	}
}
