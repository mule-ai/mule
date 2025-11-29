//go:build ignore

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
func trigger_workflow_or_agent(operationTypePtr, operationTypeSize, idPtr, idSize, paramsPtr, paramsSize uint32) uint32

// get_last_operation_result gets the last operation result
//
//go:wasmimport env get_last_operation_result
func get_last_operation_result(bufferPtr, bufferSize uint32) uint32

// get_last_operation_status gets the last operation status
//
//go:wasmimport env get_last_operation_status
func get_last_operation_status() uint32

// get_last_response_body gets the last response body
//
//go:wasmimport env get_last_response_body
func get_last_response_body(bufferPtr, bufferSize uint32) uint32

// get_last_response_status gets the last response status
//
//go:wasmimport env get_last_response_status
func get_last_response_status() uint32

// InputData represents the input structure for the WASM module
type InputData struct {
	Action string                 `json:"action"` // "trigger_workflow", "call_agent", or "http_request"
	ID     string                 `json:"id"`     // workflow ID, agent ID, or URL
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
	case "http_request":
		result, err = makeHTTPRequest(input.ID, input.Params)
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
	status := trigger_workflow_or_agent(
		uint32(uintptr(unsafe.Pointer(&[]byte(operationType)[0]))), uint32(len(operationType)),
		uint32(uintptr(unsafe.Pointer(&[]byte(workflowID)[0]))), uint32(len(workflowID)),
		uint32(uintptr(unsafe.Pointer(¶msJSON[0]))), uint32(len(paramsJSON)))

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
	status := trigger_workflow_or_agent(
		uint32(uintptr(unsafe.Pointer(&[]byte(operationType)[0]))), uint32(len(operationType)),
		uint32(uintptr(unsafe.Pointer(&[]byte(agentID)[0]))), uint32(len(agentID)),
		uint32(uintptr(unsafe.Pointer(¶msJSON[0]))), uint32(len(paramsJSON)))

	if status != 0 {
		return "", fmt.Errorf("host function failed with status: %d", status)
	}

	// Get the result
	resultJSON := getLastOperationResult()

	return resultJSON, nil
}

// makeHTTPRequest makes an HTTP request using the host function
func makeHTTPRequest(url string, params map[string]interface{}) (string, error) {
	// Extract method, headers, and body from params
	method := "GET"
	if methodVal, ok := params["method"].(string); ok {
		method = methodVal
	}

	var headers map[string]string
	if headersVal, ok := params["headers"].(map[string]interface{}); ok {
		headers = make(map[string]string)
		for k, v := range headersVal {
			if strVal, ok := v.(string); ok {
				headers[k] = strVal
			}
		}
	}

	var body interface{}
	if bodyVal, ok := params["body"]; ok {
		body = bodyVal
	}

	// Convert body to JSON if we have data
	var bodyBytes []byte
	var err error
	if body != nil {
		bodyBytes, err = json.Marshal(body)
		if err != nil {
			return "", fmt.Errorf("failed to marshal request body: %w", err)
		}
	}

	// Convert headers to JSON if we have headers
	var headersBytes []byte
	if headers != nil {
		headersBytes, err = json.Marshal(headers)
		if err != nil {
			return "", fmt.Errorf("failed to marshal headers: %w", err)
		}
	}

	// Call the host function
	status := http_request_with_headers(
		uint32(uintptr(unsafe.Pointer(&[]byte(method)[0]))), uint32(len(method)),
		uint32(uintptr(unsafe.Pointer(&[]byte(url)[0]))), uint32(len(url)),
		uint32(uintptr(unsafe.Pointer(&bodyBytes[0]))), uint32(len(bodyBytes)),
		uint32(uintptr(unsafe.Pointer(&headersBytes[0]))), uint32(len(headersBytes)))

	if status != 0 {
		return "", fmt.Errorf("host function failed with status: %d", status)
	}

	// Get the response
	statusCode := get_last_response_status()
	
	// Try to get response body
	// Allocate a buffer for the response body (10KB max)
	buffer := make([]byte, 10240)
	bodySize := get_last_response_body(uint32(uintptr(unsafe.Pointer(&buffer[0]))), uint32(len(buffer)))
	
	var responseData interface{}
	if bodySize > 0 && bodySize <= uint32(len(buffer)) {
		// Parse response body as JSON
		responseBody := string(buffer[:bodySize])
		if err := json.Unmarshal([]byte(responseBody), &responseData); err != nil {
			// If not valid JSON, store as raw string
			responseData = map[string]interface{}{
				"raw_response": responseBody,
			}
		}
	}

	// Create output
	output := map[string]interface{}{
		"success":     true,
		"data":        responseData,
		"status_code": statusCode,
	}

	// Serialize output to JSON
	outputBytes, err := json.Marshal(output)
	if err != nil {
		return "", fmt.Errorf("failed to marshal output: %w", err)
	}

	return string(outputBytes), nil
}

// getLastOperationResult gets the last operation result from the host
func getLastOperationResult() string {
	// First, get the required buffer size
	bufferSize := get_last_operation_result(0, 0)
	
	// Allocate buffer
	buffer := make([]byte, bufferSize)
	
	// Get the actual result
	actualSize := get_last_operation_result(uint32(uintptr(unsafe.Pointer(&buffer[0]))), bufferSize)
	
	if actualSize <= bufferSize {
		return string(buffer[:actualSize])
	}
	
	return ""
}

// Wrapper functions for the host functions with proper parameter handling
func get_last_operation_result(bufferPtr, bufferSize uint32) uint32 {
	return get_last_operation_result(bufferPtr, bufferSize)
}

func get_last_response_status() uint32 {
	return get_last_response_status()
}

func get_last_response_body(bufferPtr, bufferSize uint32) uint32 {
	return get_last_response_body(bufferPtr, bufferSize)
}