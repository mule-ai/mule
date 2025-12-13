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

type Input struct {
	Prompt string   `json:"prompt"`
	Token  string   `json:"token"`
	States []string `json:"states"`
}

// StateInput represents the actual input structure for updating issue state
type StateInput struct {
	Issue   string `json:"issue"`   // GitHub issue API URL
	Label   string `json:"label"`   // The new label to apply
	Comment string `json:"comment"` // Comment to add (optional)
}

// Label represents a GitHub label structure
type Label struct {
	Name string `json:"name"`
}

// IssueLabelsPayload represents the payload for updating issue labels
type IssueLabelsPayload struct {
	Labels []string `json:"labels"`
}

// Output represents the output structure
type Output struct {
	Success bool   `json:"success"`
	Issue   string `json:"issue"`
	Label   string `json:"label"`
	Comment string `json:"comment"`
	Error   string `json:"error,omitempty"`
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

// containsString checks if a string is in a slice
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
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
	var stateInput StateInput
	if err := json.Unmarshal([]byte(input.Prompt), &stateInput); err != nil {
		outputError(fmt.Errorf("failed to decode prompt content: %w", err))
		return
	}

	// Validate input
	if stateInput.Issue == "" {
		outputError(fmt.Errorf("issue URL is required"))
		return
	}

	// Validate input
	if stateInput.Issue == "" {
		outputError(fmt.Errorf("issue URL is required"))
		return
	}

	// Basic validation of GitHub API URL format
	if !isValidGitHubAPIURL(stateInput.Issue) {
		outputError(fmt.Errorf("invalid GitHub API URL format. Expected format: https://api.github.com/repos/{owner}/{repo}/issues/{number}"))
		return
	}

	if stateInput.Label == "" {
		outputError(fmt.Errorf("label is required"))
		return
	}

	// Validate that the label is in the allowed states if states are configured
	if len(input.States) > 0 {
		if !containsString(input.States, stateInput.Label) {
			outputError(fmt.Errorf("label '%s' is not in the allowed states: %v", stateInput.Label, input.States))
			return
		}
	}

	// Validate token
	if input.Token == "" {
		outputError(fmt.Errorf("GitHub token is required"))
		return
	}

	// First, get the current labels on the issue to determine what needs to be removed
	currentLabels, err := getCurrentIssueLabels(stateInput.Issue, input.Token)
	if err != nil {
		outputError(fmt.Errorf("failed to get current issue labels: %w", err))
		return
	}

	// Determine which labels to remove (all state labels except the new one)
	labelsToRemove := []string{}
	for _, currentLabel := range currentLabels {
		// If this is a state label (in our config) and it's not the new label, remove it
		if containsString(input.States, currentLabel.Name) && currentLabel.Name != stateInput.Label {
			labelsToRemove = append(labelsToRemove, currentLabel.Name)
		}
	}

	// Log what we're doing for debugging
	fmt.Fprintf(os.Stderr, "DEBUG: Current labels: %v\n", func() []string {
		names := make([]string, len(currentLabels))
		for i, label := range currentLabels {
			names[i] = label.Name
		}
		return names
	}())
	fmt.Fprintf(os.Stderr, "DEBUG: Config states: %v\n", input.States)
	fmt.Fprintf(os.Stderr, "DEBUG: Labels to remove: %v\n", labelsToRemove)
	fmt.Fprintf(os.Stderr, "DEBUG: New label: %s\n", stateInput.Label)

	// Remove old state labels
	for _, labelToRemove := range labelsToRemove {
		if err := removeLabelFromIssue(stateInput.Issue, labelToRemove, input.Token); err != nil {
			// Log the error but continue - don't fail the whole operation
			fmt.Fprintf(os.Stderr, "Warning: failed to remove label '%s': %v\n", labelToRemove, err)
		} else {
			fmt.Fprintf(os.Stderr, "DEBUG: Successfully removed label '%s'\n", labelToRemove)
		}
	}

	// Add the new label
	if err := addLabelToIssue(stateInput.Issue, stateInput.Label, input.Token); err != nil {
		outputError(fmt.Errorf("failed to add label '%s': %w", stateInput.Label, err))
		return
	} else {
		fmt.Fprintf(os.Stderr, "DEBUG: Successfully added label '%s'\n", stateInput.Label)
	}

	// Success - return the input unwrapped from the prompt field
	output := Output{
		Success: true,
		Issue:   stateInput.Issue,
		Label:   stateInput.Label,
		Comment: stateInput.Comment,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		outputError(fmt.Errorf("failed to encode output: %w", err))
		return
	}
}

// getCurrentIssueLabels gets the current labels on an issue
func getCurrentIssueLabels(issueURL, token string) ([]Label, error) {
	// Prepare headers
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token),
		"Accept":        "application/vnd.github.v3+json",
		"User-Agent":    "Mule-AI-WASM-Module",
	}

	// Convert headers to JSON
	headersPtr, headersSize, err := mapToJSONPtr(headers)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal headers: %w", err)
	}

	// Prepare HTTP request parameters
	method := "GET"
	methodPtr, methodSize := stringToPtr(method)
	urlPtr, urlSize := stringToPtr(issueURL)

	// No body for GET request
	var bodyPtr, bodySize uintptr = 0, 0

	// Make HTTP request using the enhanced host function
	errorCode := http_request_with_headers(
		methodPtr, methodSize,
		urlPtr, urlSize,
		bodyPtr, bodySize,
		headersPtr, headersSize)

	// Check for errors
	if errorCode != 0 {
		errorMsg := getErrorMessage(errorCode)
		return nil, fmt.Errorf("HTTP request failed: %s", errorMsg)
	}

	// Get response status
	statusCode := int(get_last_response_status())
	if statusCode == 403 {
		return nil, fmt.Errorf("GitHub API request failed with status: %d (Forbidden) - check that the token has 'issues:read' permissions", statusCode)
	} else if statusCode == 404 {
		return nil, fmt.Errorf("GitHub API request failed with status: %d (Not Found) - check that the issue exists and the token has access to this repository", statusCode)
	} else if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("GitHub API request failed with status: %d", statusCode)
	}

	// Get response body
	// Allocate a buffer for the response body (500KB max)
	const maxBufferSize = 512000
	buffer := make([]byte, maxBufferSize)
	bufferPtr := uintptr(unsafe.Pointer(&buffer[0]))
	bufferSize := uintptr(len(buffer))

	bodySizeRet := get_last_response_body(bufferPtr, bufferSize)
	if bodySizeRet == 0 {
		return nil, fmt.Errorf("empty response from GitHub API")
	}

	if bodySizeRet > uint32(len(buffer)) {
		return nil, fmt.Errorf("response too large")
	}

	// Parse response body as JSON
	responseBody := string(buffer[:bodySizeRet])

	// Log the response for debugging
	fmt.Fprintf(os.Stderr, "DEBUG: GitHub API response body: %s\n", responseBody)

	// Parse the issue to get labels
	var issue struct {
		Labels []Label `json:"labels"`
	}
	if err := json.Unmarshal([]byte(responseBody), &issue); err != nil {
		// Try to parse as error response
		var errorResp map[string]interface{}
		if err2 := json.Unmarshal([]byte(responseBody), &errorResp); err2 == nil {
			if message, ok := errorResp["message"].(string); ok {
				return nil, fmt.Errorf("GitHub API error: %s", message)
			}
		}
		return nil, fmt.Errorf("failed to parse GitHub API response: %w", err)
	}

	// Log the parsed labels for debugging
	fmt.Fprintf(os.Stderr, "DEBUG: Parsed labels: %v\n", func() []string {
		names := make([]string, len(issue.Labels))
		for i, label := range issue.Labels {
			names[i] = label.Name
		}
		return names
	}())

	return issue.Labels, nil
}

// removeLabelFromIssue removes a label from an issue
func removeLabelFromIssue(issueURL, labelName, token string) error {
	// Prepare headers
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token),
		"Accept":        "application/vnd.github.v3+json",
		"User-Agent":    "Mule-AI-WASM-Module",
	}

	// Convert headers to JSON
	headersPtr, headersSize, err := mapToJSONPtr(headers)
	if err != nil {
		return fmt.Errorf("failed to marshal headers: %w", err)
	}

	// Prepare HTTP request parameters
	method := "DELETE"
	methodPtr, methodSize := stringToPtr(method)
	// URL format: {issueURL}/labels/{label}
	encodedLabel := strings.ReplaceAll(labelName, " ", "%20")
	urlPtr, urlSize := stringToPtr(fmt.Sprintf("%s/labels/%s", issueURL, encodedLabel))

	// No body for DELETE request
	var bodyPtr, bodySize uintptr = 0, 0

	// Make HTTP request using the enhanced host function
	errorCode := http_request_with_headers(
		methodPtr, methodSize,
		urlPtr, urlSize,
		bodyPtr, bodySize,
		headersPtr, headersSize)

	// Check for errors
	if errorCode != 0 {
		errorMsg := getErrorMessage(errorCode)
		return fmt.Errorf("HTTP request failed: %s", errorMsg)
	}

	// Get response status
	statusCode := int(get_last_response_status())
	// 200 OK or 404 Not Found (if label wasn't there anyway) are both acceptable
	if statusCode != 200 && statusCode != 404 {
		if statusCode == 403 {
			return fmt.Errorf("GitHub API request failed with status: %d (Forbidden) - check that the token has 'issues:write' and 'labels:write' permissions", statusCode)
		} else if statusCode == 404 {
			return fmt.Errorf("GitHub API request failed with status: %d (Not Found) - check that the issue exists and the token has access to this repository", statusCode)
		} else {
			return fmt.Errorf("GitHub API request failed with status: %d", statusCode)
		}
	}

	return nil
}

// addLabelToIssue adds a label to an issue
func addLabelToIssue(issueURL, labelName, token string) error {
	// Prepare the labels payload
	payload := IssueLabelsPayload{
		Labels: []string{labelName},
	}

	// Convert payload to JSON
	payloadPtr, payloadSize, err := mapToJSONPtr(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal labels payload: %w", err)
	}

	// Prepare headers
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token),
		"Accept":        "application/vnd.github.v3+json",
		"Content-Type":  "application/json",
		"User-Agent":    "Mule-AI-WASM-Module",
	}

	// Convert headers to JSON
	headersPtr, headersSize, err := mapToJSONPtr(headers)
	if err != nil {
		return fmt.Errorf("failed to marshal headers: %w", err)
	}

	// Prepare HTTP request parameters
	method := "POST"
	methodPtr, methodSize := stringToPtr(method)
	urlPtr, urlSize := stringToPtr(fmt.Sprintf("%s/labels", issueURL))

	// Make HTTP request using the enhanced host function
	errorCode := http_request_with_headers(
		methodPtr, methodSize,
		urlPtr, urlSize,
		payloadPtr, payloadSize,
		headersPtr, headersSize)

	// Check for errors
	if errorCode != 0 {
		errorMsg := getErrorMessage(errorCode)
		return fmt.Errorf("HTTP request failed: %s", errorMsg)
	}

	// Get response status
	statusCode := int(get_last_response_status())
	if statusCode == 403 {
		return fmt.Errorf("GitHub API request failed with status: %d (Forbidden) - check that the token has 'issues:write' and 'labels:write' permissions", statusCode)
	} else if statusCode == 404 {
		return fmt.Errorf("GitHub API request failed with status: %d (Not Found) - check that the issue exists and the token has access to this repository", statusCode)
	} else if statusCode < 200 || statusCode >= 300 {
		return fmt.Errorf("GitHub API request failed with status: %d", statusCode)
	}

	return nil
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
