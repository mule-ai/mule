package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mule-ai/mule/internal/state"
	"github.com/mule-ai/mule/pkg/log"
)

func TestLoadConfig(t *testing.T) {
	configPath := filepath.Join("testdata", "config.json")
	l := log.NewStdoutLogger()
	// Clean up config file after test
	defer os.RemoveAll("testdata")

	// Test when config file doesn't exist
	appState, err := LoadConfig(configPath, l)
	if err != nil {
		t.Fatalf("Error loading config: %v", err)
	}

	if appState == nil {
		t.Error("AppState should not be nil")
	}

	// Test SaveConfig
	err = SaveConfig(configPath)
	if err != nil {
		t.Fatalf("Error saving config: %v", err)
	}

	// Test when config file exists
	appState, err = LoadConfig(configPath, l)
	if err != nil {
		t.Fatalf("Error loading config: %v", err)
	}

	if appState == nil {
		t.Error("AppState should not be nil")
	}
}

func TestSaveConfig(t *testing.T) {
	configPath := filepath.Join("testdata", "config.json")

	// Clean up config file after test
	defer os.RemoveAll("testdata")

	// Create a dummy state
	state.State = &state.AppState{}

	// Save the config
	err := SaveConfig(configPath)
	if err != nil {
		t.Fatalf("Error saving config: %v", err)
	}

	// Check if the file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Errorf("Config file was not created")
	}
}
