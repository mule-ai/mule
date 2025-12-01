//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"unsafe"
)

// Input represents the expected input structure for GitHub issues
type Input struct {
	RepoURL string `json:"repo_url"`
	Token   string `json:"token"`
}

// GitHubIssue represents a GitHub issue structure
type GitHubIssue struct {
	ID     int    `json:"id"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	URL    string `json:"url"`
	Body   string `json:"body"`
}

// Output represents the output structure
type Output struct {
	Issues []GitHubIssue `json:"issues"`
	Count  int           `json:"count"`
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

func main() {
	// Read input from stdin
	var input Input
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&input); err != nil {
		outputError(fmt.Errorf("failed to decode input: %w", err))
		return
	}

	// Validate input
	if input.RepoURL == "" {
		outputError(fmt.Errorf("repo_url is required"))
		return
	}

	if input.Token == "" {
		outputError(fmt.Errorf("token is required"))
		return
	}

	// Extract owner and repo from URL
	owner, repo, err := parseGitHubURL(input.RepoURL)
	if err != nil {
		outputError(fmt.Errorf("invalid repo_url: %w", err))
		return
	}

	// Construct GitHub API URL
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", owner, repo)

	// Prepare headers
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", input.Token),
		"Accept":        "application/vnd.github.v3+json",
		"User-Agent":    "Mule-AI-WASM-Module",
	}

	// Convert method to bytes
	method := "GET"
	methodBytes := []byte(method)
	methodPtr := uintptr(unsafe.Pointer(&methodBytes[0]))
	methodSize := uintptr(len(methodBytes))

	// Convert URL to bytes
	urlBytes := []byte(apiURL)
	urlPtr := uintptr(unsafe.Pointer(&urlBytes[0]))
	urlSize := uintptr(len(urlBytes))

	// No body for GET request
	var bodyPtr, bodySize uintptr = 0, 0

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
		outputError(fmt.Errorf("GitHub API request failed with status: %d", statusCode))
		return
	}

	// Get response body
	// Allocate a buffer for the response body (500KB max)
	buffer := make([]byte, 512000)
	bufferPtr := uintptr(unsafe.Pointer(&buffer[0]))
	bufferSize := uintptr(len(buffer))

	bodySizeRet := get_last_response_body(bufferPtr, bufferSize)
	if bodySizeRet == 0 {
		outputError(fmt.Errorf("empty response from GitHub API"))
		return
	}

	if bodySizeRet > uint32(len(buffer)) {
		outputError(fmt.Errorf("response too large"))
		return
	}

	// Parse response body as JSON
	responseBody := string(buffer[:bodySizeRet])
	var issues []GitHubIssue
	if err := json.Unmarshal([]byte(responseBody), &issues); err != nil {
		// Try to parse as error response
		var errorResp map[string]interface{}
		if err2 := json.Unmarshal([]byte(responseBody), &errorResp); err2 == nil {
			if message, ok := errorResp["message"].(string); ok {
				outputError(fmt.Errorf("GitHub API error: %s", message))
				return
			}
		}
		outputError(fmt.Errorf("failed to parse GitHub API response: %w", err))
		return
	}

	// Create output
	output := Output{
		Issues: issues,
		Count:  len(issues),
	}

	// Serialize output to JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		outputError(fmt.Errorf("failed to encode output: %w", err))
		return
	}
}

// parseGitHubURL extracts owner and repo from a GitHub URL
func parseGitHubURL(url string) (owner, repo string, err error) {
	// Handle different GitHub URL formats
	// https://github.com/owner/repo
	// https://github.com/owner/repo/
	
	// Remove trailing slash if present
	if len(url) > 0 && url[len(url)-1] == '/' {
		url = url[:len(url)-1]
	}
	
	// Check if it's a GitHub URL
	const prefix = "https://github.com/"
	if !startsWith(url, prefix) {
		return "", "", fmt.Errorf("not a valid GitHub URL")
	}
	
	// Extract owner/repo part
	path := url[len(prefix):]
	
	// Split by slash
	parts := split(path, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid GitHub URL format")
	}
	
	return parts[0], parts[1], nil
}

// startsWith checks if a string starts with a prefix
func startsWith(s, prefix string) bool {
	if len(prefix) > len(s) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if s[i] != prefix[i] {
			return false
		}
	}
	return true
}

// split splits a string by a delimiter (simple implementation since we can't use strings.Split)
func split(s, delim string) []string {
	if delim == "" {
		result := make([]string, len(s))
		for i, c := range []byte(s) {
			result[i] = string(c)
		}
		return result
	}
	
	var result []string
	start := 0
	
	for i := 0; i <= len(s)-len(delim); i++ {
		match := true
		for j := 0; j < len(delim); j++ {
			if s[i+j] != delim[j] {
				match = false
				break
			}
		}
		
		if match {
			result = append(result, s[start:i])
			start = i + len(delim)
			i = start - 1 // Adjust for loop increment
		}
	}
	
	// Add the remaining part
	result = append(result, s[start:])
	return result
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