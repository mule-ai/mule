//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"unsafe"
)

// Input represents the expected input structure
type Input struct {
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Method  string            `json:"method,omitempty"`
	Data    interface{}       `json:"data,omitempty"`
}

// Output represents the output structure
type Output struct {
	Success    bool        `json:"success"`
	Data       interface{} `json:"data,omitempty"`
	StatusCode int         `json:"status_code,omitempty"`
	Error      string      `json:"error,omitempty"`
}

// http_request_with_headers is the enhanced host function for making HTTP requests with headers
// It's imported from the host environment
//
//go:wasmimport env http_request_with_headers
func http_request_with_headers(methodPtr, methodSize, urlPtr, urlSize, bodyPtr, bodySize, headersPtr, headersSize uintptr) uintptr

// get_last_response_body gets the last response body
//go:wasmimport env get_last_response_body
func get_last_response_body(bufferPtr, bufferSize uintptr) uint32

// get_last_response_status gets the last response status code
//go:wasmimport env get_last_response_status
func get_last_response_status() uint32

// getStringFromMemory reads a string from memory at the given pointer and length
func getStringFromMemory(ptr uintptr, length uintptr) string {
	// Convert uintptr to byte slice
	byteSlice := (*[1 << 30]byte)(unsafe.Pointer(ptr))[:length:length]
	return string(byteSlice)
}

// main is the entry point for the WASM module
func main() {
	// Read input from stdin
	var input Input
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&input); err != nil {
		outputError(fmt.Errorf("failed to decode input: %w", err))
		return
	}

	// Validate input
	if input.URL == "" {
		outputError(fmt.Errorf("URL is required"))
		return
	}

	// Set default method
	if input.Method == "" {
		input.Method = "GET"
	}

	// Make HTTP request using the enhanced host function
	method := input.Method
	urlStr := input.URL

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
			outputError(fmt.Errorf("failed to marshal request data: %w", err))
			return
		}
		bodyPtr = uintptr(unsafe.Pointer(&bodyBytes[0]))
		bodySize = uintptr(len(bodyBytes))
	}

	// Convert headers to JSON if we have headers
	var headersPtr, headersSize uintptr
	if input.Headers != nil {
		headersBytes, err := json.Marshal(input.Headers)
		if err != nil {
			outputError(fmt.Errorf("failed to marshal headers: %w", err))
			return
		}
		headersPtr = uintptr(unsafe.Pointer(&headersBytes[0]))
		headersSize = uintptr(len(headersBytes))
	}

	// Call the enhanced host function to make HTTP request with headers
	errorCode := http_request_with_headers(
		methodPtr, methodSize,
		urlPtr, urlSize,
		bodyPtr, bodySize,
		headersPtr, headersSize)

	// Check for errors
	if errorCode != 0 {
		errorMsg := getErrorMessage(errorCode)
		outputError(fmt.Errorf("HTTP request failed: %s", errorMsg))
		return
	}

	// Get response data
	statusCode := int(get_last_response_status())

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

	// Create output
	output := Output{
		Success:    true,
		Data:       responseData,
		StatusCode: statusCode,
	}

	// Serialize output to JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		outputError(fmt.Errorf("failed to encode output: %w", err))
		return
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

// outputError outputs an error message in the expected format
func outputError(err error) {
	output := Output{
		Success: false,
		Error:   err.Error(),
	}

	encoder := json.NewEncoder(os.Stderr)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(output)

	os.Exit(1)
}
