package state

import (
	"testing"

	"github.com/jbutlerdev/genai/tools"
)

func TestMemoryToolsAvailable(t *testing.T) {
	// Test that memory tools are available after initialization
	// Note: This test assumes the tools are registered globally by the genai library

	// Test memory_store tool
	_, err := tools.GetTool("memory_store")
	if err != nil {
		t.Errorf("Failed to get memory_store tool: %v", err)
	}

	// Test memory_retrieve tool
	_, err = tools.GetTool("memory_retrieve")
	if err != nil {
		t.Errorf("Failed to get memory_retrieve tool: %v", err)
	}

	// Test memory_update tool
	_, err = tools.GetTool("memory_update")
	if err != nil {
		t.Errorf("Failed to get memory_update tool: %v", err)
	}

	// Test memory_delete tool
	_, err = tools.GetTool("memory_delete")
	if err != nil {
		t.Errorf("Failed to get memory_delete tool: %v", err)
	}

	// Test memory_operation tool
	_, err = tools.GetTool("memory_operation")
	if err != nil {
		t.Errorf("Failed to get memory_operation tool: %v", err)
	}
}
