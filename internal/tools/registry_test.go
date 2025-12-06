package tools

import (
	"testing"
)

func TestBashToolRegistration(t *testing.T) {
	// Test that bash tool is included in built-in tools list
	builtInTools := BuiltInTools()
	found := false
	for _, toolName := range builtInTools {
		if toolName == "bash" {
			found = true
			break
		}
	}
	
	if !found {
		t.Errorf("bash tool not found in built-in tools list")
	}
	
	// Test that bash tool can be created and registered
	registry := NewRegistry()
	_, err := registry.Get("bash")
	if err != nil {
		t.Errorf("Failed to get bash tool from registry: %v", err)
	}
}