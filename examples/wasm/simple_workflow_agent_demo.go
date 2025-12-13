//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
)

// InputData represents the input structure for the WASM module
type InputData struct {
	Action string                 `json:"action"` // "trigger_workflow" or "call_agent"
	ID     string                 `json:"id"`     // workflow ID or agent ID
	Params map[string]interface{} `json:"params"` // parameters for the operation
}

func main() {
	// Read input from stdin
	decoder := json.NewDecoder(os.Stdin)
	var input InputData

	if err := decoder.Decode(&input); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to decode input: %v\n", err)
		os.Exit(1)
	}

	// Process the input based on action
	var result string
	var err error

	switch input.Action {
	case "trigger_workflow":
		result, err = triggerWorkflow(input.ID, input.Params)
	case "call_agent":
		result, err = callAgent(input.ID, input.Params)
	default:
		fmt.Fprintf(os.Stderr, "Error: unknown action: %s\n", input.Action)
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to %s: %v\n", input.Action, err)
		os.Exit(1)
	}

	// Output result as JSON
	fmt.Print(result)
}

// triggerWorkflow triggers a workflow using the host function
func triggerWorkflow(workflowID string, params map[string]interface{}) (string, error) {
	// Convert parameters to JSON
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("failed to marshal parameters: %w", err)
	}

	// In a real implementation, we would call the host function here
	// For this demo, we'll just return a mock response
	result := fmt.Sprintf(`{"success": true, "workflow_id": "%s", "params": %s}`, workflowID, string(paramsJSON))
	return result, nil
}

// callAgent calls an agent using the host function
func callAgent(agentID string, params map[string]interface{}) (string, error) {
	// Convert parameters to JSON
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("failed to marshal parameters: %w", err)
	}

	// In a real implementation, we would call the host function here
	// For this demo, we'll just return a mock response
	result := fmt.Sprintf(`{"success": true, "agent_id": "%s", "params": %s, "response": "Mock agent response"}`, agentID, string(paramsJSON))
	return result, nil
}
