package main

import (
	"encoding/json"
	"fmt"
	"os"
	"unsafe"
)

// http_request_with_headers is the enhanced host function for making HTTP requests with headers
//
//go:wasmimport env http_request_with_headers
func http_request_with_headers(methodPtr, methodSize, urlPtr, urlSize, bodyPtr, bodySize, headersPtr, headersSize uintptr) uint32

// trigger_workflow_or_agent is the unified host function for triggering workflows or calling agents
//
//go:wasmimport env trigger_workflow_or_agent
func trigger_workflow_or_agent(operationTypePtr, operationTypeSize, idPtr, idSize, paramsPtr, paramsSize uintptr) uint32

// get_last_operation_result retrieves the result of the last operation
//
//go:wasmimport env get_last_operation_result
func get_last_operation_result(bufferPtr, bufferSize uintptr) uint32

// get_last_operation_status gets the status of the last operation
//
//go:wasmimport env get_last_operation_status
func get_last_operation_status() uint32

func main() {
	// Read input data from stdin
	inputData := make([]byte, 1024)
	n, err := os.Stdin.Read(inputData)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}
	
	// Parse input data as JSON
	var input map[string]interface{}
	if err := json.Unmarshal(inputData[:n], &input); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing input JSON: %v\n", err)
		os.Exit(1)
	}
	
	// Get operation type from input
	operationType, ok := input["operation_type"].(string)
	if !ok {
		fmt.Fprintf(os.Stderr, "Missing or invalid operation_type in input\n")
		os.Exit(1)
	}
	
	// Get ID from input
	id, ok := input["id"].(string)
	if !ok {
		fmt.Fprintf(os.Stderr, "Missing or invalid id in input\n")
		os.Exit(1)
	}
	
	// Get parameters from input
	params, ok := input["params"].(map[string]interface{})
	if !ok {
		params = make(map[string]interface{})
	}
	
	// Convert parameters to JSON
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling parameters: %v\n", err)
		os.Exit(1)
	}
	
	// Call the unified function
	resultCode := trigger_workflow_or_agent(
		uintptr(unsafe.Pointer(&[]byte(operationType)[0])), uintptr(len(operationType)),
		uintptr(unsafe.Pointer(&[]byte(id)[0])), uintptr(len(id)),
		uintptr(unsafe.Pointer(&paramsJSON[0])), uintptr(len(paramsJSON)),
	)
	
	// Check result
	if resultCode != 0 {
		fmt.Fprintf(os.Stderr, "Error calling trigger_workflow_or_agent: 0x%08X\n", resultCode)
		os.Exit(1)
	}
	
	// Get operation status
	status := get_last_operation_status()
	
	// Allocate buffer for result
	buffer := make([]byte, 4096)
	
	// Get operation result
	resultSize := get_last_operation_result(
		uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)),
	)
	
	// Check if we got a valid result
	if resultSize > 0 && resultSize <= uint32(len(buffer)) {
		// Parse the result as JSON
		var result map[string]interface{}
		if err := json.Unmarshal(buffer[:resultSize], &result); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing result JSON: %v\n", err)
			os.Exit(1)
		}
		
		// Add our metadata
		result["operation_type"] = operationType
		result["id"] = id
		result["status_code"] = status
		
		// Output the result as JSON
		output, err := json.Marshal(result)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error marshaling output: %v\n", err)
			os.Exit(1)
		}
		
		fmt.Printf("%s\n", output)
	} else {
		// Simple output for error cases
		output := map[string]interface{}{
			"operation_type": operationType,
			"id":            id,
			"status_code":   status,
			"result_size":   resultSize,
			"message":       "Operation completed",
		}
		
		jsonOutput, _ := json.Marshal(output)
		fmt.Printf("%s\n", jsonOutput)
	}
}