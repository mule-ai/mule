// Package main demonstrates how to make HTTP requests (GET and POST) from a WASM module in Mule
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"unsafe"
)

// InputData represents the input structure for the WASM module
type InputData struct {
	URL    string                 `json:"url"`              // URL to make HTTP request to
	Method string                 `json:"method,omitempty"` // HTTP method (GET, POST, PUT, etc.)
	Body   map[string]interface{} `json:"body,omitempty"`   // Request body for POST/PUT requests
}

// OutputData represents the output structure from the WASM module
type OutputData struct {
	Result  string                 `json:"result"`         // Result message
	Data    map[string]interface{} `json:"data,omitempty"` // Response data
	Success bool                   `json:"success"`        // Success flag
}

// http_request is the host function for making HTTP requests with any method
// It's imported from the host environment
//
//go:wasmimport env http_request
func http_request(methodPtr, methodSize, urlPtr, urlSize, bodyPtr, bodySize uintptr) uint32

func main() {
	// Read input from stdin
	decoder := json.NewDecoder(os.Stdin)
	var input InputData

	if err := decoder.Decode(&input); err != nil {
		outputError(fmt.Errorf("failed to decode input: %w", err))
		return
	}

	// Process the input
	result := processInput(input)

	// Output result as JSON
	outputResult(result)
}

func processInput(input InputData) OutputData {
	// Set default method
	if input.Method == "" {
		input.Method = "GET"
	}

	// Check if we have a URL to call
	if input.URL == "" {
		return OutputData{
			Result:  "No URL provided for HTTP request",
			Success: false,
		}
	}

	// Make HTTP request using the generic host function
	method := input.Method
	urlStr := input.URL

	// Prepare parameters for the host function
	methodBytes := []byte(method)
	methodPtr := uintptr(unsafe.Pointer(&methodBytes[0]))
	methodSize := uintptr(len(methodBytes))

	urlBytes := []byte(urlStr)
	urlPtr := uintptr(unsafe.Pointer(&urlBytes[0]))
	urlSize := uintptr(len(urlBytes))

	// Convert body to JSON string if provided
	var bodyPtr, bodySize uintptr
	if input.Body != nil {
		bodyBytes, err := json.Marshal(input.Body)
		if err != nil {
			return OutputData{
				Result:  fmt.Sprintf("Failed to marshal request body: %v", err),
				Success: false,
			}
		}
		bodyPtr = uintptr(unsafe.Pointer(&bodyBytes[0]))
		bodySize = uintptr(len(bodyBytes))
	}

	resultCode := http_request(methodPtr, methodSize, urlPtr, urlSize, bodyPtr, bodySize)

	var result string
	switch resultCode {
	case 0:
		result = fmt.Sprintf("Successfully made HTTP %s request to: %s", method, urlStr)
	case 0xFFFFFFF0:
		result = "Error: Failed to read HTTP method from memory"
	case 0xFFFFFFF1:
		result = "Error: Failed to read HTTP body from memory"
	case 0xFFFFFFFF:
		result = "Error: Failed to read URL from memory"
	case 0xFFFFFFFE:
		result = "Error: URL not allowed"
	case 0xFFFFFFFD:
		result = "Error: Failed to create HTTP request"
	case 0xFFFFFFFC:
		result = "Error: Failed to make HTTP request"
	default:
		result = fmt.Sprintf("Error: Unknown error code: 0x%08X", resultCode)
	}

	return OutputData{
		Result:  result,
		Success: resultCode == 0,
	}
}

func outputResult(result OutputData) {
	encoder := json.NewEncoder(os.Stdout)
	// Important: Disable HTML escaping to preserve special characters
	encoder.SetEscapeHTML(false)

	if err := encoder.Encode(result); err != nil {
		outputError(fmt.Errorf("failed to encode output: %w", err))
	}
}

func outputError(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}