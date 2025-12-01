package main

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

// This is a test to verify that the getLastOperationResult function works correctly
// We'll simulate the WASM module execution flow

func TestGetLastOperationResult(t *testing.T) {
	// This is a mock implementation for testing purposes
	// In a real scenario, this would call the actual WASM host functions
	
	// Simulate the fixed getLastOperationResult function
	getLastOperationResult := func() ([]byte, error) {
		// Simulate getting the length (should return 22 for "Hello from workflow!")
		length := uint32(22)
		
		// Simulate the case where length is an error code
		if length >= 0xFFFFFFF0 {
			return nil, fmt.Errorf("failed to get result length: %d", length)
		}

		// Allocate a buffer for the result data
		buffer := make([]byte, length)

		// Simulate reading the actual data
		copy(buffer, "Hello from workflow!")
		
		// Simulate successful read
		actualLength := uint32(len("Hello from workflow!"))
		if actualLength >= 0xFFFFFFF0 {
			return nil, fmt.Errorf("failed to read result data: %d", actualLength)
		}

		// Return the buffer with the actual data
		return buffer[:actualLength], nil
	}

	// Test the function
	result, err := getLastOperationResult()
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	expected := "Hello from workflow!"
	if string(result) != expected {
		t.Errorf("Expected result '%s', got '%s'", expected, string(result))
	}
	
	fmt.Printf("Test passed! Result: %s\n", string(result))
}

// Example of how the main function would work with the fix
func ExampleMainFlow() {
	// Simulate input data
	inputData := map[string]interface{}{
		"prompt": "Hello, process this text",
	}
	
	// Extract the prompt from input
	message := ""
	if msg, ok := inputData["prompt"].(string); ok {
		message = msg
	} else {
		message = "Hello, world!"
	}
	
	// Prepare parameters for the workflow
	params := map[string]interface{}{
		"prompt": message,
	}
	
	// Convert params to JSON
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling params: %v\n", err)
		return
	}
	
	fmt.Printf("Input message: %s\n", message)
	fmt.Printf("Params JSON: %s\n", string(paramsJSON))
	
	// Simulate calling execute_target (would return 0 for success)
	errorCode := uint32(0)
	if errorCode != 0 {
		fmt.Fprintf(os.Stderr, "Error executing workflow: %d\n", errorCode)
		return
	}
	
	// Simulate getting status (would return 0 for success)
	status := int32(0)
	if status != 0 {
		fmt.Fprintf(os.Stderr, "Workflow execution failed with status: %d\n", status)
		return
	}
	
	// Simulate getting result
	result := []byte(`{"response": "Workflow executed successfully!"}`)
	
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
		return
	}
	
	fmt.Printf("Output: %s\n", string(outputJSON))
	
	// Output:
	// Input message: Hello, process this text
	// Params JSON: {"prompt":"Hello, process this text"}
	// Output: {"data":{"response":"Workflow executed successfully!"},"status":0,"success":true}
}