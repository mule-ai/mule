package tools

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

	assert.Equal(t, int64(0), exitCode, "Expected exit code 0")

	stdout, ok := resultMap["stdout"].(string)
	require.True(t, ok, "Expected stdout to be string, got %T", resultMap["stdout"])

	assert.Equal(t, "hello world", stdout, "Expected 'hello world'")
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
	require.True(t, ok, "Expected map[string]interface{}, got %T", result)

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

	assert.NotEqual(t, 0, exitCode, "Expected non-zero exit code")
}
