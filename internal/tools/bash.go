package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"google.golang.org/adk/tool"
	"google.golang.org/genai"
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

// ToTool converts this to an ADK tool
func (b *BashTool) ToTool() tool.Tool {
	return &bashToolAdapter{tool: b}
}

// bashToolAdapter adapts BashTool to the ADK tool interface
type bashToolAdapter struct {
	tool *BashTool
}

func (a *bashToolAdapter) Name() string {
	return a.tool.Name()
}

func (a *bashToolAdapter) Description() string {
	return a.tool.Description()
}

func (a *bashToolAdapter) IsLongRunning() bool {
	return a.tool.IsLongRunning()
}

func (a *bashToolAdapter) GetTool() interface{} {
	return a.tool
}

// Declaration returns the function declaration for this tool
func (a *bashToolAdapter) Declaration() *genai.FunctionDeclaration {
	schema := a.tool.GetSchema()
	paramsJSON, _ := json.Marshal(schema)

	return &genai.FunctionDeclaration{
		Name:                 a.tool.Name(),
		Description:          a.tool.Description(),
		ParametersJsonSchema: string(paramsJSON),
	}
}

// Run executes the tool with the provided context and arguments
func (a *bashToolAdapter) Run(ctx tool.Context, args any) (map[string]any, error) {
	// Convert args to map[string]interface{}
	argsMap, ok := args.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected map[string]any, got %T", args)
	}

	result, err := a.tool.Execute(context.Background(), argsMap)
	if err != nil {
		return nil, err
	}

	// Convert result to map[string]any
	resultMap, ok := result.(map[string]any)
	if !ok {
		return map[string]any{"result": result}, nil
	}

	return resultMap, nil
}
