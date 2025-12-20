//go:build wasm || ignore

package main

import (
	"encoding/json"
	"os"
	"unsafe"
)

// Input represents the input structure received from Mule runtime
type Input struct {
	Token      string    `json:"token"`      // Authentication token for git operations
	Repository string    `json:"repository"` // Base repository path (optional)
	UserName   string    `json:"user_name"`  // Git user name for commit (optional)
	UserEmail  string    `json:"user_email"` // Git user email for commit (optional)
	Prompt     PassInput `json:"prompt"`     // JSON string containing the actual input (issue and comment)
}

// CommentInput represents the actual input structure for posting a comment
type PassInput struct {
	PRTitle string `json:"title"` // Pull Request Title (optional)
	PRBody  string `json:"body"`  // Pull Request Body (optional)
}

// Output represents the output structure
type Output struct {
	PRTitle string `json:"title"` // Pull Request Title (optional)
	PRBody  string `json:"body"`  // Pull Request Body (optional)
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// push_current_branch is the host function for pushing the current branch
// It's imported from the host environment
//
//go:wasmimport env push_current_branch
func push_current_branch(tokenPtr, tokenSize, basePathPtr, basePathSize, userNamePtr, userNameSize, userEmailPtr, userEmailSize uintptr) uintptr

// stringToPtr converts a string to a pointer and size for WASM host functions
// Returns 0, 0 for empty strings to avoid panics
func stringToPtr(s string) (uintptr, uintptr) {
	if s == "" {
		return 0, 0
	}
	bytes := []byte(s)
	return uintptr(unsafe.Pointer(&bytes[0])), uintptr(len(bytes))
}

func main() {
	// Read input from stdin
	var input Input
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&input); err != nil {
		outputError(err.Error())
		return
	}

	// Call the host function to push the current branch
	tokenPtr, tokenSize := stringToPtr(input.Token)
	basePathPtr, basePathSize := stringToPtr(input.Repository)
	userNamePtr, userNameSize := stringToPtr(input.UserName)
	userEmailPtr, userEmailSize := stringToPtr(input.UserEmail)

	result := push_current_branch(tokenPtr, tokenSize, basePathPtr, basePathSize, userNamePtr, userNameSize, userEmailPtr, userEmailSize)

	// Check for errors
	if result != 0 {
		errorMsg := getErrorMessage(result)
		outputError(errorMsg)
		return
	}

	// Success
	outputResult := Output{
		Success: true,
		PRTitle: input.Prompt.PRTitle,
		PRBody:  input.Prompt.PRBody,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(outputResult)
}

// getErrorMessage returns a human-readable error message for the given error code
func getErrorMessage(errorCode uintptr) string {
	switch errorCode {
	case 0x00000000:
		return "success"
	case 0xFFFFFFF0:
		return "failed to read token from memory"
	case 0xFFFFFFF1:
		return "failed to read base path from memory"
	case 0xFFFFFFF2:
		return "failed to get current working directory"
	case 0xFFFFFFF3:
		return "base path is not a git repository"
	case 0xFFFFFFF4:
		return "invalid branch name derived from worktree"
	case 0xFFFFFFF5:
		return "failed to create git branch"
	case 0xFFFFFFF6:
		return "failed to push git branch"
	case 0xFFFFFFF7:
		return "failed to stage changes"
	case 0xFFFFFFF8:
		return "failed to commit changes"
	case 0xFFFFFFF9:
		return "failed to set git user config"
	default:
		return "unknown error"
	}
}

// outputError outputs an error message in the expected format
func outputError(errorMessage string) {
	output := Output{
		Success: false,
		Error:   errorMessage,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(output)

	os.Exit(1)
}
