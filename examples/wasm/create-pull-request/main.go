//go:build wasm || ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"unsafe"
)

// TODO: Consider parsing issue URL to extract owner/repo automatically.
// This would allow the github-comment wasm to pass an issue URL directly
// instead of requiring separate owner and repo fields.

// Input represents the input structure received from Mule runtime
type Input struct {
	Token  string           `json:"token"`          // GitHub token for authentication
	Owner  string           `json:"owner"`          // Repository owner
	Repo   string           `json:"repo"`           // Repository name
	Title  string           `json:"title"`          // Pull request title
	Head   string           `json:"head,omitempty"` // Head branch name (optional, will be detected if not provided)
	Base   string           `json:"base"`           // Base branch name
	Prompt PullRequestInput `json:"prompt"`
	Body   string           `json:"body,omitempty"`  // Pull request description (optional)
	Draft  bool             `json:"draft,omitempty"` // Whether to create as draft (optional)
}

type PullRequestInput struct {
	PRTitle string `json:"title"` // Pull Request Title
	PRBody  string `json:"body"`  // Pull Request Body
}

// PullRequestPayload represents the payload for creating a pull request
type PullRequestPayload struct {
	Title string `json:"title"`
	Head  string `json:"head"`
	Base  string `json:"base"`
	Body  string `json:"body,omitempty"`
	Draft bool   `json:"draft,omitempty"`
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
//
//go:wasmimport env get_last_response_body
func get_last_response_body(bufferPtr, bufferSize uintptr) uint32

// get_last_response_status gets the last response status code
//
//go:wasmimport env get_last_response_status
func get_last_response_status() uint32

// get_current_branch gets the current git branch name
//
//go:wasmimport env get_current_branch
func get_current_branch(basePathPtr, basePathSize, bufferPtr, bufferSize uint32) uint32

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

// getCurrentBranchName gets the current git branch name from the working directory
func getCurrentBranchName() (string, error) {
	// First, try to get the required buffer size
	requiredSize := get_current_branch(0, 0, 0, 0)
	if requiredSize == 0 {
		return "", fmt.Errorf("failed to get branch name size")
	}

	// Allocate buffer
	buffer := make([]byte, requiredSize)
	bufferPtr := uint32(uintptr(unsafe.Pointer(&buffer[0])))
	bufferSize := uint32(len(buffer))

	// Get the branch name
	errorCode := get_current_branch(0, 0, bufferPtr, bufferSize)
	if errorCode != 0 {
		return "", fmt.Errorf("failed to get current branch name: error code 0x%x", errorCode)
	}

	return string(buffer[:requiredSize]), nil
}

func main() {
	// Read input from stdin
	var input Input
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&input); err != nil {
		outputError(fmt.Errorf("failed to decode input: %w", err))
		return
	}

	// Validate required input
	if input.Token == "" {
		outputError(fmt.Errorf("GitHub token is required"))
		return
	}

	if input.Owner == "" {
		outputError(fmt.Errorf("repository owner is required"))
		return
	}

	if input.Repo == "" {
		outputError(fmt.Errorf("repository name is required"))
		return
	}

	if input.Title == "" && input.Prompt.PRTitle == "" {
		outputError(fmt.Errorf("pull request title is required"))
		return
	}

	if input.Base == "" {
		outputError(fmt.Errorf("base branch is required"))
		return
	}

	// If head branch is not provided, try to detect it automatically
	headBranch := input.Head
	if headBranch == "" {
		var err error
		headBranch, err = getCurrentBranchName()
		if err != nil {
			outputError(fmt.Errorf("failed to detect current branch name: %w", err))
			return
		}

		if headBranch == "" {
			outputError(fmt.Errorf("could not detect current branch name"))
			return
		}

		// Log that we're using the detected branch name
		fmt.Fprintf(os.Stderr, "Using detected branch name: %s\n", headBranch)
	}

	// set title and body
	var body, title string
	if input.Prompt.PRBody != "" {
		body = input.Prompt.PRBody
	} else {
		body = input.Body
	}
	if input.Prompt.PRTitle != "" {
		title = input.Prompt.PRTitle
	} else {
		title = input.Title
	}

	// Prepare the pull request payload
	payload := PullRequestPayload{
		Title: title,
		Head:  headBranch,
		Base:  input.Base,
		Body:  body,
		Draft: input.Draft,
	}

	// Convert payload to JSON
	payloadPtr, payloadSize, err := mapToJSONPtr(payload)
	if err != nil {
		outputError(fmt.Errorf("failed to marshal pull request payload: %w", err))
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
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls", input.Owner, input.Repo)
	urlPtr, urlSize := stringToPtr(url)

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
			Message: "Pull request created successfully",
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
	// Error codes for get_current_branch function
	case 0xFFFFFFE1:
		return "failed to read base path from memory (branch detection)"
	case 0xFFFFFFE2:
		return "failed to get current working directory (branch detection)"
	case 0xFFFFFFE3:
		return "base path is not a git repository (branch detection)"
	case 0xFFFFFFE4:
		return "failed to get current branch name"
	case 0xFFFFFFE5:
		return "buffer too small for branch name"
	case 0xFFFFFFE6:
		return "failed to write branch name to memory"
	case 0xFFFFFFE7:
		return "base path is not accessible (branch detection)"
	case 0xFFFFFFE8:
		return "empty branch name detected (branch detection)"
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
