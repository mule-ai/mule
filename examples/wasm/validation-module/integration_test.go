package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestValidationModuleWithSampleConfig(t *testing.T) {
	// Read the sample configuration file
	configFile, err := os.Open("sample-config.json")
	if err != nil {
		t.Fatalf("Failed to open sample config: %v", err)
	}
	defer func() {
		if err := configFile.Close(); err != nil {
			t.Errorf("Failed to close config file: %v", err)
		}
	}()

	var config map[string]interface{}
	decoder := json.NewDecoder(configFile)
	if err := decoder.Decode(&config); err != nil {
		t.Fatalf("Failed to decode sample config: %v", err)
	}

	// Verify the configuration was read correctly
	expectedCommand := "go test ./..."
	if config["validation_command"] != expectedCommand {
		t.Errorf("Expected validation command '%s', got '%s'", expectedCommand, config["validation_command"])
	}

	if config["max_attempts"] != 3.0 {
		t.Errorf("Expected max attempts 3, got %f", config["max_attempts"])
	}

	if config["working_directory"] != "." {
		t.Errorf("Expected working directory '.', got '%s'", config["working_directory"])
	}
}