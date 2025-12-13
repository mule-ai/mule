package tools

import (
	"context"
	"testing"
)

func TestBashTool(t *testing.T) {
	tool := NewBashTool()

	// Test basic command execution
	params := map[string]interface{}{
		"command": "echo 'hello world'",
	}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map[string]interface{}, got %T", result)
	}

	if resultMap["exitCode"] != 0 {
		t.Errorf("Expected exit code 0, got %v", resultMap["exitCode"])
	}

	stdout, ok := resultMap["stdout"].(string)
	if !ok {
		t.Fatalf("Expected stdout to be string, got %T", resultMap["stdout"])
	}

	if stdout != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", stdout)
	}
}

func TestBashToolError(t *testing.T) {
	tool := NewBashTool()

	// Test command that fails
	params := map[string]interface{}{
		"command": "exit 1",
	}

	result, err := tool.Execute(context.Background(), params)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map[string]interface{}, got %T", result)
	}

	if resultMap["exitCode"] == 0 {
		t.Errorf("Expected non-zero exit code, got %v", resultMap["exitCode"])
	}
}
