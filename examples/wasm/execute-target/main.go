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

// Helper function to allocate memory and copy string to it
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
func getLastOperationResult() ([]byte, error) {
	// Allocate a buffer for the result
	// First get the length by calling with a zero buffer
	length := get_last_operation_result(0, 0)
	if length >= 0xFFFFFFF0 {
		return nil, fmt.Errorf("failed to get result length: %d", length)
	}

	// Allocate a buffer of the required size
	// Note: In a real WASM module, you would allocate memory in the WASM linear memory
	// For this example, we're just returning a byte slice of the length
	return make([]byte, length), nil
}

func main() {
	// Read input from stdin
	var inputData map[string]interface{}
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&inputData); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}

	// Extract parameters
	targetType, _ := inputData["target_type"].(string)
	targetID, _ := inputData["target_id"].(string)
	params, _ := inputData["params"].(map[string]interface{})

	// Convert params to JSON
	paramsJSON := "{}"
	if params != nil {
		if paramsBytes, err := json.Marshal(params); err == nil {
			paramsJSON = string(paramsBytes)
		}
	}

	// Convert strings to pointers and sizes
	targetTypePtr, targetTypeSize, targetTypeBytes := stringToPtr(targetType)
	targetIDPtr, targetIDSize, targetIDBytes := stringToPtr(targetID)
	paramsPtr, paramsSize, paramsBytes := stringToPtr(paramsJSON)

	// Keep references to prevent garbage collection
	_ = targetTypeBytes
	_ = targetIDBytes
	_ = paramsBytes

	// Call the execute_target host function
	errorCode := execute_target(targetTypePtr, targetTypeSize, targetIDPtr, targetIDSize, paramsPtr, paramsSize)
	if errorCode != 0 {
		fmt.Fprintf(os.Stderr, "Error executing target: %d\n", errorCode)
		os.Exit(1)
	}

	// Get the status
	status := get_last_operation_status()
	if status != 0 {
		fmt.Fprintf(os.Stderr, "Operation failed with status: %d\n", status)
		os.Exit(1)
	}

	// Get the result
	result, err := getLastOperationResult()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting result: %v\n", err)
		os.Exit(1)
	}

	// Output result as JSON
	output := map[string]interface{}{
		"success": true,
		"result":  string(result),
		"status":  status,
	}

	outputJSON, err := json.Marshal(output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(string(outputJSON))
}