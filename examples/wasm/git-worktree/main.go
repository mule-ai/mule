//go:build wasm || ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unsafe"
)

// Input represents the input structure received from Mule runtime
type Input struct {
	Prompt     interface{} `json:"prompt"`     // Can be string or object containing worktree_name
	Repository string      `json:"repository"` // Base repository path (optional)
}

// WorktreeInput represents the actual input structure for creating a worktree
type WorktreeInput struct {
	WorktreeName string `json:"worktree_name"` // Name of the worktree to create
}

// Output represents the output structure
type Output struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
	Error   string `json:"error,omitempty"`
}

// create_git_worktree is the host function for creating a git worktree
// It's imported from the host environment
//
//go:wasmimport env create_git_worktree
func create_git_worktree(namePtr, nameSize, basePathPtr, basePathSize uintptr) uintptr

// stringToPtr converts a string to a pointer and size for WASM host functions
func stringToPtr(s string) (uintptr, uintptr) {
	bytes := []byte(s)
	return uintptr(unsafe.Pointer(&bytes[0])), uintptr(len(bytes))
}

// isValidWorktreeName validates that the worktree name is safe to use
func isValidWorktreeName(name string) bool {
	// Check for empty name
	if name == "" {
		return false
	}

	// Check for invalid characters
	invalidChars := []string{"/", "\\", "..", "~"}
	for _, char := range invalidChars {
		if strings.Contains(name, char) {
			return false
		}
	}

	// Check for reserved names
	reservedNames := []string{".", "..", "HEAD", "ORIG_HEAD", "FETCH_HEAD", "MERGE_HEAD", "CHERRY_PICK_HEAD"}
	for _, reserved := range reservedNames {
		if name == reserved {
			return false
		}
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

	// Parse the worktree input - handle both string and object cases
	var worktreeInput WorktreeInput

	// Check if prompt is a string or an object
	switch v := input.Prompt.(type) {
	case string:
		// Prompt is a JSON string, unmarshal it
		if err := json.Unmarshal([]byte(v), &worktreeInput); err != nil {
			outputError(fmt.Errorf("failed to decode prompt content: %w", err))
			return
		}
	case map[string]interface{}:
		// Prompt is already an object, extract worktree_name
		if name, ok := v["worktree_name"].(string); ok {
			worktreeInput.WorktreeName = name
		} else {
			outputError(fmt.Errorf("worktree_name not found in prompt object"))
			return
		}
	default:
		outputError(fmt.Errorf("unexpected prompt type: %T", input.Prompt))
		return
	}

	// Validate input
	if worktreeInput.WorktreeName == "" {
		outputError(fmt.Errorf("worktree name is required"))
		return
	}

	// Validate worktree name
	if !isValidWorktreeName(worktreeInput.WorktreeName) {
		outputError(fmt.Errorf("invalid worktree name: %s", worktreeInput.WorktreeName))
		return
	}

	// Create the git worktree using the host function
	namePtr, nameSize := stringToPtr(worktreeInput.WorktreeName)
	var basePathPtr, basePathSize uintptr
	if input.Repository != "" {
		basePathPtr, basePathSize = stringToPtr(input.Repository)
	}

	errorCode := create_git_worktree(namePtr, nameSize, basePathPtr, basePathSize)

	// Check for errors
	if errorCode != 0 {
		errorMsg := getErrorMessage(errorCode)
		outputError(fmt.Errorf("failed to create git worktree: %s", errorMsg))
		return
	}

	// Success - the host function has already set the working directory
	outputResult := Output{
		Success: true,
		Message: fmt.Sprintf("Git worktree '%s' created successfully", worktreeInput.WorktreeName),
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(outputResult); err != nil {
		outputError(fmt.Errorf("failed to encode output: %w", err))
		return
	}
}

// getErrorMessage returns a human-readable error message for the given error code
func getErrorMessage(errorCode uintptr) string {
	switch errorCode {
	case 0x00000000:
		return "success"
	case 0xFFFFFFF0:
		return "failed to read worktree name from memory"
	case 0xFFFFFFF1:
		return "failed to read base path from memory"
	case 0xFFFFFFF2:
		return "failed to get current working directory"
	case 0xFFFFFFF3:
		return "base path is not a git repository"
	case 0xFFFFFFF4:
		return "failed to create git worktree"
	case 0xFFFFFFF5:
		return "buffer too small for result"
	case 0xFFFFFFF6:
		return "failed to write result to memory"
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