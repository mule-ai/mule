package rss_workflow

import (
	"testing"

	"github.com/go-logr/logr"
)

func TestContentExtractionStepCreation(t *testing.T) {
	logger := logr.Discard()
	step := NewContentExtractionStep(nil, logger)

	if step == nil {
		t.Fatal("Expected content extraction step to be created")
	}
}
