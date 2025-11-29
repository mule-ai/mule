package main

import (
	"encoding/json"
	"fmt"
	"os"
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

func main() {
	// Read input from stdin
	var inputData map[string]interface{}
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&inputData); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}

	// Extract the prompt/message from input
	message := ""
	if msg, ok := inputData["prompt"].(string); ok {
		message = msg
	} else if msg, ok := inputData["message"].(string); ok {
		message = msg
	} else {
		message = "Hello, world!"
	}

	// Prepare parameters for the workflow
	// We'll pass the input message as the prompt for the workflow
	params := map[string]interface{}{
		"prompt": message,
	}

	// Convert params to JSON
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling params: %v\n", err)
		os.Exit(1)
	}

	// Convert strings to pointers and sizes
	// Target type is "workflow"
	targetType := "workflow"
	targetTypePtr, targetTypeSize, targetTypeBytes := stringToPtr(targetType)

	// Target ID - we'll use "Default" for the default workflow
	targetID := "Default"
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
		fmt.Fprintf(os.Stderr, "Error executing workflow: %d\n", errorCode)
		os.Exit(1)
	}

	// Get the status
	status := get_last_operation_status()
	if status != 0 {
		fmt.Fprintf(os.Stderr, "Workflow execution failed with status: %d\n", status)
		os.Exit(1)
	}

	// Get the result
	result, err := getLastOperationResult()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting result: %v\n", err)
		os.Exit(1)
	}

	// Try to parse the result as JSON
	var resultData map[string]interface{}
	if err := json.Unmarshal(result, &resultData); err != nil {
		// If it's not JSON, treat it as a string
		resultData = map[string]interface{}{
			"result": string(result),
		}
	}

	// Output the result as JSON
	output := map[string]interface{}{
		"success": true,
		"data":    resultData,
		"status":  status,
	}

	outputJSON, err := json.Marshal(output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(outputJSON))
}