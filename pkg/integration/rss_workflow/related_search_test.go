package rss_workflow

import (
	"testing"
)

func TestRelatedSearchStepCreation(t *testing.T) {
	step := NewRelatedSearchStep(nil, "")

	if step == nil {
		t.Fatal("Expected related search step to be created")
	}
}
