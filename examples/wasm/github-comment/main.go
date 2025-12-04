//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"unsafe"
)

// Input represents the input structure received from Mule runtime
type Input struct {
	Prompt string `json:"prompt"` // JSON string containing the actual input (issue and comment)
	Token  string `json:"token"`  // GitHub token for authentication
}

// CommentInput represents the actual input structure for posting a comment
type CommentInput struct {
	Issue   string `json:"issue"`   // GitHub issue API URL
	Comment string `json:"comment"` // Comment content to post
}

// CommentPayload represents the payload for creating a comment
type CommentPayload struct {
	Body string `json:"body"` // Comment body
}

// Output represents the output structure
type Output struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	URL     string `json:"url,omitempty"`
	Error   string `json:"error,omitempty"`
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

// stringToPtr converts a string to a pointer and size for WASM host functions
func stringToPtr(s string) (uintptr, uintptr) {
	bytes := []byte(s)
	return uintptr(unsafe.Pointer(&bytes[0])), uintptr(len(bytes))
}

// mapToJSONPtr converts a map to JSON and returns pointer and size for WASM host functions
func mapToJSONPtr(m interface{}) (uintptr, uintptr, error) {
	bytes, err := json.Marshal(m)
	if err != nil {
		return 0, 0, err
	}
	return uintptr(unsafe.Pointer(&bytes[0])), uintptr(len(bytes)), nil
}

// isValidGitHubAPIURL validates that the URL follows GitHub API format
func isValidGitHubAPIURL(url string) bool {
	// Check if it starts with the GitHub API base URL
	const githubAPIBase = "https://api.github.com/repos/"
	if !strings.HasPrefix(url, githubAPIBase) {
		return false
	}

	// Check if it has the expected path structure
	// Expected: https://api.github.com/repos/{owner}/{repo}/issues/{number}
	path := url[len(githubAPIBase):]
	parts := strings.Split(path, "/")

	// Should have at least owner/repo/issues/number (4 parts)
	if len(parts) < 4 {
		return false
	}

	// Check if the third-to-last part is "issues"
	if parts[len(parts)-2] != "issues" {
		return false
	}

	// Check if the last part (issue number) is numeric
	issueNumber := parts[len(parts)-1]
	if _, err := strconv.Atoi(issueNumber); err != nil {
		return false
	}

	return true
}

func main() {
	// Read input from stdin
	var input Input
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&input); err != nil {
		outputError(fmt.Errorf("failed to decode input: %w", err))
		return
	}

	// Parse the actual input from the prompt field
	var commentInput CommentInput
	if err := json.Unmarshal([]byte(input.Prompt), &commentInput); err != nil {
		outputError(fmt.Errorf("failed to decode prompt content: %w", err))
		return
	}

	// Validate input
	if commentInput.Issue == "" {
		outputError(fmt.Errorf("issue URL is required"))
		return
	}

	// Basic validation of GitHub API URL format
	if !isValidGitHubAPIURL(commentInput.Issue) {
		outputError(fmt.Errorf("invalid GitHub API URL format. Expected format: https://api.github.com/repos/{owner}/{repo}/issues/{number}"))
		return
	}

	// Special case: if comment is empty string, exit successfully without posting
	if commentInput.Comment == "" {
		output := Output{
			Success: true,
			Message: "Empty comment - no action taken",
		}

		encoder := json.NewEncoder(os.Stdout)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(output); err != nil {
			outputError(fmt.Errorf("failed to encode output: %w", err))
			return
		}
		return
	}

	// Validate token
	if input.Token == "" {
		outputError(fmt.Errorf("GitHub token is required"))
		return
	}

	// Prepare the comment payload
	payload := CommentPayload{
		Body: commentInput.Comment,
	}

	// Convert payload to JSON
	payloadPtr, payloadSize, err := mapToJSONPtr(payload)
	if err != nil {
		outputError(fmt.Errorf("failed to marshal comment payload: %w", err))
		return
	}

	// Prepare headers
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", input.Token),
		"Accept":        "application/vnd.github.v3+json",
		"Content-Type":  "application/json",
		"User-Agent":    "Mule-AI-WASM-Module",
	}

	// Convert headers to JSON
	headersPtr, headersSize, err := mapToJSONPtr(headers)
	if err != nil {
		outputError(fmt.Errorf("failed to marshal headers: %w", err))
		return
	}

	// Prepare HTTP request parameters
	method := "POST"
	methodPtr, methodSize := stringToPtr(method)
	urlPtr, urlSize := stringToPtr(commentInput.Issue + "/comments")

	// Make HTTP request using the enhanced host function
	errorCode := http_request_with_headers(
		methodPtr, methodSize,
		urlPtr, urlSize,
		payloadPtr, payloadSize,
		headersPtr, headersSize)

	// Check for errors
	if errorCode != 0 {
		errorMsg := getErrorMessage(errorCode)
		outputError(fmt.Errorf("HTTP request failed: %s", errorMsg))
		return
	}

	// Get response status
	statusCode := int(get_last_response_status())

	// Get response body
	// Allocate a buffer for the response body (500KB max)
	const maxBufferSize = 512000
	buffer := make([]byte, maxBufferSize)
	bufferPtr := uintptr(unsafe.Pointer(&buffer[0]))
	bufferSize := uintptr(len(buffer))

	bodySizeRet := get_last_response_body(bufferPtr, bufferSize)

	var responseMessage string
	var responseURL string

	if bodySizeRet > 0 && bodySizeRet <= uint32(len(buffer)) {
		// Parse response body as JSON to extract URL
		responseBody := string(buffer[:bodySizeRet])

		// Try to parse as JSON to extract URL
		var responseMap map[string]interface{}
		if err := json.Unmarshal([]byte(responseBody), &responseMap); err == nil {
			if url, ok := responseMap["html_url"].(string); ok {
				responseURL = url
			}
			if message, ok := responseMap["message"].(string); ok {
				responseMessage = message
			}
		}
	}

	// Check if request was successful
	if statusCode >= 200 && statusCode < 300 {
		// Success
		output := Output{
			Success: true,
			Message: "Comment posted successfully",
			URL:     responseURL,
		}

		encoder := json.NewEncoder(os.Stdout)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(output); err != nil {
			outputError(fmt.Errorf("failed to encode output: %w", err))
			return
		}
	} else {
		// Error
		var errorMessage string
		if responseMessage != "" {
			errorMessage = fmt.Sprintf("GitHub API error: %s (status: %d)", responseMessage, statusCode)
		} else {
			errorMessage = fmt.Sprintf("GitHub API request failed with status: %d", statusCode)
		}

		// Try to parse detailed error response
		if bodySizeRet > 0 && bodySizeRet <= uint32(len(buffer)) {
			responseBody := string(buffer[:bodySizeRet])
			var errorDetails map[string]interface{}
			if err := json.Unmarshal([]byte(responseBody), &errorDetails); err == nil {
				if details, ok := errorDetails["errors"]; ok {
					if detailsBytes, err := json.Marshal(details); err == nil {
						errorMessage += fmt.Sprintf(", details: %s", string(detailsBytes))
					}
				}
			}
		}

		outputError(fmt.Errorf(errorMessage))
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

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(output)

	os.Exit(1)
}