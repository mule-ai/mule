package rss_workflow

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config == nil {
		t.Fatal("Expected config to be created")
	}

	if !config.Enabled {
		t.Error("Expected config to be enabled by default")
	}

	if config.CacheTTL != 6*time.Hour {
		t.Errorf("Expected cache TTL to be 6 hours, got %v", config.CacheTTL)
	}
}
