//go:build ignore

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"unsafe"
)

// Input represents the expected input structure
type Input struct {
	Document string `json:"prompt"`
	Endpoint string `json:"endpoint"`
}

// Response represents the expected response structure from the mdserve API
type Response struct {
	APIURL   string `json:"api_url"`
	Filename string `json:"filename"`
	Message  string `json:"message"`
	URL      string `json:"url"`
}

// http_request_with_headers is the enhanced host function for making HTTP requests with headers
// It's imported from the host environment
//
//go:wasmimport env http_request_with_headers
func http_request_with_headers(methodPtr, methodSize, urlPtr, urlSize, bodyPtr, bodySize, headersPtr, headersSize uintptr) uintptr

// get_last_response_body gets the last response body
//
//go:wasmimport env get_last_response_body
func get_last_response_body(bufferPtr, bufferSize uintptr) uint32

// get_last_response_status gets the last response status code
//
//go:wasmimport env get_last_response_status
func get_last_response_status() uint32

// generateFilename creates a SHA256 hash of the content to use as a filename
func generateFilename(content string) string {
	hash := sha256.Sum256([]byte(content))
	// Use first 16 bytes of hash and encode as hex for a shorter filename
	return hex.EncodeToString(hash[:16])
}

func main() {
	// Read input from stdin
	var input Input
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&input); err != nil {
		outputError(fmt.Errorf("failed to decode input: %w", err))
		return
	}

	// Validate input
	if input.Document == "" {
		outputError(fmt.Errorf("document is required"))
		return
	}

	if input.Endpoint == "" {
		outputError(fmt.Errorf("endpoint is required"))
		return
	}

	// Generate a filename based on content hash
	filename := generateFilename(input.Document)

	// Create the JSON payload for mdserve API
	payload := map[string]string{
		"filename": filename,
		"content":  input.Document,
	}

	// Marshal the payload to JSON
	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		outputError(fmt.Errorf("failed to marshal payload: %w", err))
		return
	}

	// Prepare headers
	headers := map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
		"User-Agent":   "Mule-AI-WASM-Module",
	}

	// Convert method to bytes
	method := "POST"
	methodBytes := []byte(method)
	methodPtr := uintptr(unsafe.Pointer(&methodBytes[0]))
	methodSize := uintptr(len(methodBytes))

	// Convert URL to bytes
	urlBytes := []byte(input.Endpoint)
	urlPtr := uintptr(unsafe.Pointer(&urlBytes[0]))
	urlSize := uintptr(len(urlBytes))

	// Convert body to bytes (already done above)
	bodyPtr := uintptr(unsafe.Pointer(&bodyBytes[0]))
	bodySize := uintptr(len(bodyBytes))

	// Convert headers to JSON
	headersBytes, err := json.Marshal(headers)
	if err != nil {
		outputError(fmt.Errorf("failed to marshal headers: %w", err))
		return
	}
	headersPtr := uintptr(unsafe.Pointer(&headersBytes[0]))
	headersSize := uintptr(len(headersBytes))

	// Make HTTP request using the enhanced host function
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

	// Get response status
	statusCode := int(get_last_response_status())
	if statusCode < 200 || statusCode >= 300 {
		outputError(fmt.Errorf("API request failed with status: %d", statusCode))
		return
	}

	// Get response body
	// Allocate a buffer for the response body (500KB max)
	buffer := make([]byte, 512000)
	bufferPtr := uintptr(unsafe.Pointer(&buffer[0]))
	bufferSize := uintptr(len(buffer))

	bodySizeRet := get_last_response_body(bufferPtr, bufferSize)
	if bodySizeRet == 0 {
		outputError(fmt.Errorf("empty response from API"))
		return
	}

	if bodySizeRet > uint32(len(buffer)) {
		outputError(fmt.Errorf("response too large"))
		return
	}

	// Parse response body as JSON
	responseBody := string(buffer[:bodySizeRet])
	var response Response
	if err := json.Unmarshal([]byte(responseBody), &response); err != nil {
		// Try to parse as error response
		var errorResp map[string]interface{}
		if err2 := json.Unmarshal([]byte(responseBody), &errorResp); err2 == nil {
			if message, ok := errorResp["message"].(string); ok {
				outputError(fmt.Errorf("API error: %s", message))
				return
			}
		}
		outputError(fmt.Errorf("failed to parse API response: %w", err))
		return
	}

	// Output only the URL
	fmt.Print(response.URL)
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
	// Simple error output as JSON
	fmt.Printf("{\"error\": \"%s\"}\n", err.Error())
	os.Exit(1)
}
