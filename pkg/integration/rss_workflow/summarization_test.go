package rss_workflow

import (
	"testing"
)

func TestSummarizationStepCreation(t *testing.T) {
	step := NewSummarizationStep(nil)

	if step == nil {
		t.Fatal("Expected summarization step to be created")
	}
}
