package main

import (
	"encoding/json"
	"fmt"
	"os"
	"syscall/js"
	"unsafe"
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

	// Call the host function
	operationType := "workflow"
	status := triggerWorkflowOrAgent(operationType, workflowID, string(paramsJSON))
	
	if status != 0 {
		return "", fmt.Errorf("host function failed with status: %d", status)
	}

	// Get the result
	resultJSON := getLastOperationResult()
	
	return resultJSON, nil
}

// callAgent calls an agent using the host function
func callAgent(agentID string, params map[string]interface{}) (string, error) {
	// Convert parameters to JSON
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return "", fmt.Errorf("failed to marshal parameters: %w", err)
	}

	// Call the host function
	operationType := "agent"
	status := triggerWorkflowOrAgent(operationType, agentID, string(paramsJSON))
	
	if status != 0 {
		return "", fmt.Errorf("host function failed with status: %d", status)
	}

	// Get the result
	resultJSON := getLastOperationResult()
	
	return resultJSON, nil
}

// Host function declarations
// These would be provided by the runtime in a real implementation

//go:wasmimport env trigger_workflow_or_agent
func triggerWorkflowOrAgent(operationTypePtr, operationTypeSize, idPtr, idSize, paramsPtr, paramsSize uint32) uint32

//go:wasmimport env get_last_operation_result
func getLastOperationResult(bufferPtr, bufferSize uint32) uint32

//go:wasmimport env get_last_operation_status
func getLastOperationStatus() uint32