package main

import (
	"encoding/json"
	"fmt"
	"os"
	"unsafe"
)

// InputData represents the input structure for the WASM module
type InputData struct {
	URL     string                 `json:"input"`               // URL to make HTTP request to
	Method  string                 `json:"method,omitempty"`  // HTTP method (default: GET)
	Headers map[string]string      `json:"headers,omitempty"` // HTTP headers
	Data    map[string]interface{} `json:"data,omitempty"`    // Request data
}

// OutputData represents the output structure from the WASM module
type OutputData struct {
	Result     string                 `json:"result"`              // Result message
	Data       map[string]interface{} `json:"data,omitempty"`      // Response data
	StatusCode int                    `json:"status_code,omitempty"` // HTTP status code
	Success    bool                   `json:"success"`             // Success flag
}

// http_request_with_headers is the enhanced host function for making HTTP requests with headers
// It's imported from the host environment
//
//go:wasmimport env http_request_with_headers
func http_request_with_headers(methodPtr, methodSize, urlPtr, urlSize, bodyPtr, bodySize, headersPtr, headersSize uintptr) uint32

// get_last_response_body gets the last response body
//go:wasmimport env get_last_response_body
func get_last_response_body(bufferPtr, bufferSize uintptr) uint32

// get_last_response_status gets the last response status code
//go:wasmimport env get_last_response_status
func get_last_response_status() uint32

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

	// Make HTTP request using the enhanced host function
	urlStr := input.URL
	method := input.Method

	// Convert strings to byte slices to get pointers and sizes
	urlBytes := []byte(urlStr)
	urlPtr := uintptr(unsafe.Pointer(&urlBytes[0]))
	urlSize := uintptr(len(urlBytes))

	methodBytes := []byte(method)
	methodPtr := uintptr(unsafe.Pointer(&methodBytes[0]))
	methodSize := uintptr(len(methodBytes))

	// Convert body to JSON if we have data
	var bodyPtr, bodySize uintptr
	if input.Data != nil {
		bodyBytes, err := json.Marshal(input.Data)
		if err != nil {
			return OutputData{
				Result:  fmt.Sprintf("Failed to marshal request data: %v", err),
				Success: false,
			}
		}
		bodyPtr = uintptr(unsafe.Pointer(&bodyBytes[0]))
		bodySize = uintptr(len(bodyBytes))
	}

	// Convert headers to JSON if we have headers
	var headersPtr, headersSize uintptr
	if input.Headers != nil {
		headersBytes, err := json.Marshal(input.Headers)
		if err != nil {
			return OutputData{
				Result:  fmt.Sprintf("Failed to marshal headers: %v", err),
				Success: false,
			}
		}
		headersPtr = uintptr(unsafe.Pointer(&headersBytes[0]))
		headersSize = uintptr(len(headersBytes))
	}

	// Call the enhanced host function to make HTTP request with headers
	resultCode := http_request_with_headers(methodPtr, methodSize, urlPtr, urlSize, bodyPtr, bodySize, headersPtr, headersSize)

	var result string
	switch resultCode {
	case 0:
		result = fmt.Sprintf("Successfully made HTTP request to: %s", urlStr)
	case 0xFFFFFFFF:
		result = "Error: Failed to read URL from memory"
	case 0xFFFFFFFE:
		result = "Error: URL not allowed"
	case 0xFFFFFFFD:
		result = "Error: Failed to create HTTP request"
	case 0xFFFFFFFC:
		result = "Error: Failed to make HTTP request"
	case 0xFFFFFFF0:
		result = "Error: Failed to read HTTP method from memory"
	case 0xFFFFFFF1:
		result = "Error: Failed to read HTTP body from memory"
	case 0xFFFFFFF2:
		result = "Error: Failed to read HTTP headers from memory"
	case 0xFFFFFFF3:
		result = "Error: Failed to parse HTTP headers JSON"
	default:
		result = fmt.Sprintf("Error: Unknown error code: 0x%08X", resultCode)
	}

	// If successful, get the response data
	statusCode := 0
	var responseData map[string]interface{}

	if resultCode == 0 {
		// Get status code
		statusCode = int(get_last_response_status())

		// Try to get response body
		// Allocate a buffer for the response body (10KB max)
		buffer := make([]byte, 10240)
		bufferPtr := uintptr(unsafe.Pointer(&buffer[0]))
		bufferSize := uintptr(len(buffer))

		bodySize := get_last_response_body(bufferPtr, bufferSize)
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
	}

	return OutputData{
		Result:     result,
		Data:       responseData,
		StatusCode: statusCode,
		Success:    resultCode == 0,
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
