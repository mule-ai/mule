//go:build !lint

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"unsafe"
)

// InputData represents the input structure for WASM modules
// Configuration values (like URL) are automatically merged with input data
type InputData struct {
	Prompt string                 `json:"prompt"`           // Main input from previous workflow step or user input
	Data   map[string]interface{} `json:"data,omitempty"`    // Additional data
	URL    string                 `json:"url,omitempty"`     // Service URL (from configuration or input)
}

// OutputData represents the output structure from the WASM module
type OutputData struct {
	Result     string                 `json:"result"`              // Result message
	Data       map[string]interface{} `json:"data,omitempty"`       // Response data
	StatusCode int                    `json:"status_code,omitempty"` // HTTP status code
	Success    bool                   `json:"success"`             // Success flag
}

// http_request_with_headers is the enhanced host function for making HTTP requests with headers
// It's imported from the host environment
//
//go:wasmimport env http_request_with_headers
//nolint:unused
func http_request_with_headers(methodPtr, methodSize, urlPtr, urlSize, bodyPtr, bodySize, headersPtr, headersSize uintptr) uint32

// get_last_response_body gets the last response body
//go:wasmimport env get_last_response_body
//nolint:unused
func get_last_response_body(bufferPtr, bufferSize uintptr) uint32

// get_last_response_status gets the last response status code
//go:wasmimport env get_last_response_status
//nolint:unused
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
	// Get URL from configuration/input
	urlStr := input.URL
	if urlStr == "" {
		return OutputData{
			Result:  "No URL provided for HTTP request",
			Success: false,
		}
	}

	// Prepare the message payload in the required format
	messagePayload := map[string]interface{}{
		"message": input.Prompt,
	}

	// Merge any additional data from input.Data into messagePayload
	if input.Data != nil {
		for key, value := range input.Data {
			messagePayload[key] = value
		}
	}

	// Convert payload to JSON
	bodyBytes, err := json.Marshal(messagePayload)
	if err != nil {
		return OutputData{
			Result:  fmt.Sprintf("Failed to marshal message payload: %v", err),
			Success: false,
		}
	}

	// Set method to POST
	method := "POST"

	// Prepare headers (set Content-Type to application/json)
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	// Convert strings to byte slices to get pointers and sizes
	urlBytes := []byte(urlStr)
	urlPtr := uintptr(unsafe.Pointer(&urlBytes[0]))
	urlSize := uintptr(len(urlBytes))

	methodBytes := []byte(method)
	methodPtr := uintptr(unsafe.Pointer(&methodBytes[0]))
	methodSize := uintptr(len(methodBytes))

	bodyPtr := uintptr(unsafe.Pointer(&bodyBytes[0]))
	bodySize := uintptr(len(bodyBytes))

	headersBytes, err := json.Marshal(headers)
	if err != nil {
		return OutputData{
			Result:  fmt.Sprintf("Failed to marshal headers: %v", err),
			Success: false,
		}
	}
	headersPtr := uintptr(unsafe.Pointer(&headersBytes[0]))
	headersSize := uintptr(len(headersBytes))

	// Call the enhanced host function to make HTTP request with headers
	resultCode := http_request_with_headers(
		methodPtr, methodSize,
		urlPtr, urlSize,
		bodyPtr, bodySize,
		headersPtr, headersSize)

	// Get response status code
	statusCode := int(get_last_response_status())

	// Handle result
	if resultCode != 0 {
		errorMsg := getErrorMessage(uintptr(resultCode))
		return OutputData{
			Result:     fmt.Sprintf("HTTP request failed: %s", errorMsg),
			StatusCode: statusCode,
			Success:    false,
		}
	}

	// Try to get response body
	// Allocate a buffer for the response body (100KB max)
	buffer := make([]byte, 102400)
	bufferPtr := uintptr(unsafe.Pointer(&buffer[0]))
	bufferSize := uintptr(len(buffer))

	bodySizeRet := get_last_response_body(bufferPtr, bufferSize)
	var responseData interface{}

	if bodySizeRet > 0 && bodySizeRet <= uint32(len(buffer)) {
		// Parse response body as JSON
		responseBody := string(buffer[:bodySizeRet])
		if err := json.Unmarshal([]byte(responseBody), &responseData); err != nil {
			// If not valid JSON, store as raw string
			responseData = map[string]interface{}{
				"raw_response": responseBody,
			}
		}
	}

	return OutputData{
		Result:     fmt.Sprintf("Successfully sent prompt to %s", urlStr),
		Data:       map[string]interface{}{"response": responseData},
		StatusCode: statusCode,
		Success:    true,
	}
}

// getErrorMessage returns a human-readable error message for the given error code
func getErrorMessage(errorCode uintptr) string {
	switch errorCode {
	case 0x00000000:
		return "success"
	case 0xFFFFFFFF:
		return "failed to read URL from memory"
	case 0xFFFFFFFE:
		return "URL not allowed"
	case 0xFFFFFFFD:
		return "failed to create HTTP request"
	case 0xFFFFFFFC:
		return "failed to make HTTP request"
	case 0xFFFFFFFB:
		return "failed to read response body"
	case 0xFFFFFFF0:
		return "failed to read HTTP method from memory"
	case 0xFFFFFFF1:
		return "failed to read HTTP body from memory"
	case 0xFFFFFFF2:
		return "failed to read HTTP headers from memory"
	case 0xFFFFFFF3:
		return "failed to parse HTTP headers JSON"
	case 0xFFFFFFF4:
		return "no response available"
	case 0xFFFFFFF5:
		return "buffer too small for response data"
	case 0xFFFFFFF6:
		return "failed to write response data to memory"
	case 0xFFFFFFF7:
		return "failed to read header name from memory"
	default:
		return fmt.Sprintf("unknown error (code: 0x%x)", errorCode)
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