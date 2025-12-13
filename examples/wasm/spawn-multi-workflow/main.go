//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"unsafe"
)

// Output represents the output structure
type Output struct {
	Message string           `json:"message,omitempty"`
	Results []WorkflowResult `json:"results,omitempty"`
	Success bool             `json:"success"`
	Error   string           `json:"error,omitempty"`
}

// execute_target is the host function for triggering workflows or calling agents
//
//go:wasmimport env execute_target
func execute_target(targetTypePtr, targetTypeSize, targetIDPtr, targetIDSize, paramsPtr, paramsSize uint32) uint32

// get_last_operation_result retrieves the result of the last operation
//
//go:wasmimport env get_last_operation_result
func get_last_operation_result(bufferPtr, bufferSize uint32) uint32

// get_last_operation_status retrieves the status of the last operation
//
//go:wasmimport env get_last_operation_status
func get_last_operation_status() int32

// Helper function to convert string to pointer and size
// Returns the pointer, size, and byte slice (to prevent garbage collection)
// Note: In a real WASM module, you would allocate in WASM linear memory
func stringToPtr(s string) (uint32, uint32, []byte) {
	if s == "" {
		return 0, 0, nil
	}
	// Create a byte slice and return it to prevent garbage collection
	bytes := []byte(s)
	ptr := uint32(uintptr(unsafe.Pointer(&bytes[0])))
	return ptr, uint32(len(bytes)), bytes
}

// Helper function to read the last operation result
// This reads the actual data from WASM memory
func getLastOperationResult() ([]byte, error) {
	// First get the length by calling with a zero buffer
	length := get_last_operation_result(0, 0)
	if length >= 0xFFFFFFF0 {
		return nil, fmt.Errorf("failed to get result length: %d", length)
	}

	// Allocate a buffer for the result data
	buffer := make([]byte, length)

	// If length is 0, return empty buffer
	if length == 0 {
		return buffer, nil
	}

	// Get pointer to buffer
	bufferPtr := uint32(uintptr(unsafe.Pointer(&buffer[0])))

	// Call the function again with the actual buffer to read the data
	actualLength := get_last_operation_result(bufferPtr, uint32(length))
	if actualLength >= 0xFFFFFFF0 {
		return nil, fmt.Errorf("failed to read result data: %d", actualLength)
	}

	// Return the buffer with the actual data
	return buffer, nil
}

// WorkflowResult represents the result of executing a workflow
type WorkflowResult struct {
	Name    string      `json:"name"`
	Success bool        `json:"success"`
	Error   string      `json:"error,omitempty"`
}

// executeWorkflow executes a single workflow with the given parameters
func executeWorkflow(name string, params map[string]interface{}, wg *sync.WaitGroup, results chan<- WorkflowResult) {
	defer wg.Done()

	// Convert params to JSON
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		results <- WorkflowResult{
			Name:  name,
			Error: fmt.Sprintf("Error marshaling params: %v", err),
		}
		return
	}

	// Convert strings to pointers and sizes
	// Target type is "workflow"
	targetType := "workflow"
	targetTypePtr, targetTypeSize, targetTypeBytes := stringToPtr(targetType)

	// Target ID - use the workflow name
	targetID := name
	targetIDPtr, targetIDSize, targetIDBytes := stringToPtr(targetID)

	// Params JSON
	paramsPtr, paramsSize, paramsBytes := stringToPtr(string(paramsJSON))

	// Keep references to prevent garbage collection
	_ = targetTypeBytes
	_ = targetIDBytes
	_ = paramsBytes

	// Call the execute_target host function to trigger the workflow
	errorCode := execute_target(targetTypePtr, targetTypeSize, targetIDPtr, targetIDSize, paramsPtr, paramsSize)
	if errorCode != 0 {
		results <- WorkflowResult{
			Name:  name,
			Error: fmt.Sprintf("Error executing workflow: %d", errorCode),
		}
		return
	}

	// Get the status
	status := get_last_operation_status()
	if status != 0 {
		results <- WorkflowResult{
			Name:  name,
			Error: fmt.Sprintf("Workflow execution failed with status: %d", status),
		}
		return
	}

	// Get the result (should contain job ID)
	result, err := getLastOperationResult()
	if err != nil {
		results <- WorkflowResult{
			Name:  name,
			Error: fmt.Sprintf("Error getting result: %v", err),
		}
		return
	}

	// Parse the result to get the job ID
	var jobResult map[string]interface{}
	if err := json.Unmarshal(result, &jobResult); err != nil {
		results <- WorkflowResult{
			Name:  name,
			Error: fmt.Sprintf("Error parsing job result: %v", err),
		}
		return
	}

	_, ok := jobResult["job_id"].(string)
	if !ok {
		results <- WorkflowResult{
			Name:  name,
			Error: fmt.Sprintf("Job ID not found in result: %s", string(result)),
		}
		return
	}
	results <- WorkflowResult{
		Name:    name,
		Success: true,
	}
}

func main() {
	// Read input from stdin
	var inputData map[string]interface{}
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&inputData); err != nil {
		outputError(fmt.Errorf("error reading input"))
		return
	}

	// Extract workflow names from config
	workflowNames, ok := inputData["workflow_names"]
	if !ok {
		outputError(fmt.Errorf("missing workflow_names"))
		return
	}

	// Convert to string slice
	namesSlice, ok := workflowNames.([]interface{})
	if !ok {
		outputError(fmt.Errorf("missing workflow_names"))
		return
	}

	// Convert interface{} slice to string slice
	var workflowNameStrings []string
	for _, name := range namesSlice {
		if nameStr, ok := name.(string); ok {
			workflowNameStrings = append(workflowNameStrings, nameStr)
		}
	}

	// Extract prompt from input (will be passed to all workflows)
	prompt := inputData["prompt"]

	// Extract working directory from config (optional)
	workingDir := ""
	if wd, ok := inputData["working_directory"].(string); ok {
		workingDir = wd
	}

	// Process each workflow
	var wg sync.WaitGroup
	results := make(chan WorkflowResult, len(workflowNameStrings))

	// Launch workflows in parallel
	for _, name := range workflowNameStrings {
		// Prepare parameters for the workflow
		params := map[string]interface{}{
			"prompt": prompt,
		}

		// If a working directory is specified, add it to the params
		if workingDir != "" {
			params["working_directory"] = workingDir
		}

		// Launch workflow in a goroutine
		wg.Add(1)
		go executeWorkflow(name, params, &wg, results)
	}

	// Close results channel when all goroutines are done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect all results
	var allResults []WorkflowResult
	for result := range results {
		allResults = append(allResults, result)
	}

	// Create output
	output := Output{
		Results: allResults,
		Success: true,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		outputError(fmt.Errorf("failed to encode output: %w", err))
		return
	}
}

// outputError outputs an error message in the expected format
func outputError(err error) {
	output := Output{
		Success: false,
		Error:   err.Error(),
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(output)

	os.Exit(1)
}
