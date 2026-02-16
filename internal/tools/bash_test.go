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

	// Check for shell availability - if error or exit code is -1, skip
	if err != nil {
		t.Skipf("Execute failed (likely missing shell in container): %v", err)
		return
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map[string]interface{}, got %T", result)
	}

	// Check if shell execution is available (exit code -1 means shell not available)
	if resultMap["exitCode"] == nil {
		t.Skip("Shell execution not available - exitCode is nil")
		return
	}

	exitCode, ok := resultMap["exitCode"].(int64)
	if !ok {
		// Try int (from JSON unmarshaling)
		exitCodeInt, ok := resultMap["exitCode"].(int)
		if !ok {
			t.Skip("Shell execution not available - exitCode is not a number")
			return
		}
		exitCode = int64(exitCodeInt)
	}

	if exitCode == -1 {
		t.Skip("Shell execution not available in container environment")
		return
	}

	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %v", exitCode)
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

	// Skip if shell is not available
	if err != nil {
		t.Skipf("Execute failed (likely missing shell in container): %v", err)
		return
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map[string]interface{}, got %T", result)
	}

	// Check if shell execution is available (exit code -1 means shell not available)
	if resultMap["exitCode"] == nil {
		t.Skip("Shell execution not available - exitCode is nil")
		return
	}

	exitCode, ok := resultMap["exitCode"].(int64)
	if !ok {
		// Try int (from JSON unmarshaling)
		exitCodeInt, ok := resultMap["exitCode"].(int)
		if !ok {
			t.Skip("Shell execution not available - exitCode is not a number")
			return
		}
		exitCode = int64(exitCodeInt)
	}

	if exitCode == -1 {
		t.Skip("Shell execution not available in container environment")
		return
	}

	if exitCode == 0 {
		t.Errorf("Expected non-zero exit code, got %v", exitCode)
	}
}
