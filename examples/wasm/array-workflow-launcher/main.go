//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"unsafe"
)

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

// Result represents the result of launching a workflow
type Result struct {
	Index   int         `json:"index"`
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// launchWorkflow launches a single workflow with the given parameters
func launchWorkflow(index int, title, body, workflowName, workingDir string, wg *sync.WaitGroup, results chan<- Result) {
	defer wg.Done()

	// Prepare parameters for the workflow
	// Pass the entire JSON string as the prompt
	params := map[string]interface{}{
		"prompt": body, // body now contains the entire JSON string
	}

	// If a working directory is specified, add it to the params
	if workingDir != "" {
		params["working_directory"] = workingDir
	}

	// Convert params to JSON
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		results <- Result{
			Index: index,
			Error: fmt.Sprintf("Error marshaling params: %v", err),
		}
		return
	}

	// Convert strings to pointers and sizes
	// Target type is "workflow"
	targetType := "workflow"
	targetTypePtr, targetTypeSize, targetTypeBytes := stringToPtr(targetType)

	// Target ID - use the specified workflow name
	targetID := workflowName
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
		results <- Result{
			Index: index,
			Error: fmt.Sprintf("Error executing workflow: %d", errorCode),
		}
		return
	}

	// Get the status
	status := get_last_operation_status()
	if status != 0 {
		results <- Result{
			Index: index,
			Error: fmt.Sprintf("Workflow execution failed with status: %d", status),
		}
		return
	}

	// Get the result
	result, err := getLastOperationResult()
	if err != nil {
		results <- Result{
			Index: index,
			Error: fmt.Sprintf("Error getting result: %v", err),
		}
		return
	}

	// Try to parse the result as JSON
	var resultData map[string]interface{}
	if err := json.Unmarshal(result, &resultData); err != nil {
		// If it's not JSON, treat it as a string
		resultData = map[string]interface{}{
			"result": string(result),
		}
	}

	results <- Result{
		Index:   index,
		Success: true,
		Data:    resultData,
	}
}

func main() {
	// Read input from stdin
	var inputData map[string]interface{}
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&inputData); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}

	// Extract the prompt array from input
	promptData, ok := inputData["prompt"]
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: 'prompt' field not found in input\n")
		os.Exit(1)
	}

	// Extract workflow name from config (defaults to "Default")
	workflowName := "Default"
	if workflow, ok := inputData["workflow"].(string); ok {
		workflowName = workflow
	}

	// Extract working directory from config (optional)
	workingDir := ""
	if wd, ok := inputData["working_directory"].(string); ok {
		workingDir = wd
	}

	// Convert to map and get the result array
	promptMap, ok := promptData.(map[string]interface{})
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: 'prompt' field is not an object\n")
		os.Exit(1)
	}

	resultArray, ok := promptMap["result"].([]interface{})
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: 'prompt.result' field is not an array\n")
		os.Exit(1)
	}

	// Process each item in the array
	var wg sync.WaitGroup
	results := make(chan Result, len(resultArray))

	// Launch workflows in parallel
	for i, item := range resultArray {
		// Convert the entire item to JSON string
		itemJSON, err := json.Marshal(item)
		if err != nil {
			results <- Result{
				Index: i,
				Error: fmt.Sprintf("Error marshaling item to JSON: %v", err),
			}
			continue
		}

		// Launch workflow in a goroutine, passing the entire JSON string and working directory
		wg.Add(1)
		go launchWorkflow(i, "", string(itemJSON), workflowName, workingDir, &wg, results)
	}

	// Close results channel when all goroutines are done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect all results
	allResults := make([]Result, len(resultArray))
	for result := range results {
		allResults[result.Index] = result
	}

	// Output the results as JSON
	output := map[string]interface{}{
		"results": allResults,
	}

	outputJSON, err := json.Marshal(output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(outputJSON))
}