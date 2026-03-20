package tools

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// BashTool provides bash command execution capabilities for agents
type BashTool struct {
	name       string
	desc       string
	timeout    time.Duration
	workingDir string
}

// NewBashTool creates a new bash tool
func NewBashTool() *BashTool {
	return &BashTool{
		name:    "bash",
		desc:    "Execute bash commands in a secure shell environment",
		timeout: 30 * time.Second,
	}
}

// SetWorkingDirectory sets the working directory for this tool instance
func (b *BashTool) SetWorkingDirectory(workingDir string) {
	b.workingDir = workingDir
}

// Name returns the tool name
func (b *BashTool) Name() string {
	return b.name
}

// Description returns the tool description
func (b *BashTool) Description() string {
	return b.desc
}

// IsLongRunning indicates if this is a long-running operation
func (b *BashTool) IsLongRunning() bool {
	return false
}

// Execute executes the bash tool with the given parameters
func (b *BashTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	command, ok := params["command"].(string)
	if !ok {
		return nil, fmt.Errorf("command parameter is required")
	}

	// Create a context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, b.timeout)
	defer cancel()

	// Create the command
	cmd := exec.CommandContext(timeoutCtx, "bash", "-c", command)

	// Set working directory if specified
	if b.workingDir != "" {
		cmd.Dir = b.workingDir
	}

	// Execute the command
	output, err := cmd.CombinedOutput()

	// Check if the context was cancelled (timeout)
	if timeoutCtx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("command timed out after %v", b.timeout)
	}

	// Return the result
	result := map[string]interface{}{
		"command":  command,
		"exitCode": 0,
		"stdout":   "",
		"stderr":   "",
	}

	if err != nil {
		// Check if it's an exit error
		if exitErr, ok := err.(*exec.ExitError); ok {
			result["exitCode"] = exitErr.ExitCode()
		} else {
			result["exitCode"] = -1
		}
		result["error"] = err.Error()
	}

	// Split output into stdout and stderr
	// Since we used CombinedOutput, we need to handle this differently
	// For now, we'll put everything in stdout and leave stderr empty
	// A more sophisticated implementation might separate them
	outputStr := string(output)
	result["stdout"] = strings.TrimSpace(outputStr)

	return result, nil
}

// GetSchema returns the JSON schema for this tool
func (b *BashTool) GetSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The bash command to execute",
			},
		},
		"required": []string{"command"},
	}
}
