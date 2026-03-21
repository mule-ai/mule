package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
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

	assert.True(t, found, "bash tool not found in built-in tools list")

	// Test that bash tool can be created and registered
	registry := NewRegistry()
	_, err := registry.Get("bash")
	assert.NoError(t, err, "Failed to get bash tool from registry")
}
