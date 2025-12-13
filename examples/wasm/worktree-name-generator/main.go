package main

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// Variable to allow mocking time.Now() in tests
var timeNow = time.Now

// Input represents the input structure received from Mule runtime
// The prompt field contains a JSON string with issue data
type Input struct {
	Prompt string `json:"prompt"` // JSON string containing issue data
}

// IssueData represents the structure of the GitHub issue data
type IssueData struct {
	Title string `json:"title"`
}

// PromptData represents the structure of the prompt data
type PromptData struct {
	Title string `json:"title"`
}

// Output represents the output structure with the generated worktree name
type Output struct {
	WorktreeName string `json:"worktree_name"`
}

func main() {
	// Read input from stdin
	var input Input
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&input); err != nil {
		outputError(fmt.Errorf("failed to decode input: %w", err))
		return
	}

	// Parse the prompt field to extract issue data
	var issueData IssueData
	if err := json.Unmarshal([]byte(input.Prompt), &issueData); err != nil {
		// Try alternative structure
		var promptData PromptData
		if err2 := json.Unmarshal([]byte(input.Prompt), &promptData); err2 != nil {
			outputError(fmt.Errorf("failed to decode prompt content: %w", err))
			return
		}
		issueData.Title = promptData.Title
	}

	// Validate input
	if issueData.Title == "" {
		outputError(fmt.Errorf("issue title is required"))
		return
	}

	// Generate worktree name based on current date and issue title
	worktreeName := generateWorktreeName(issueData.Title)

	// Create output
	output := Output{
		WorktreeName: worktreeName,
	}

	// Serialize output to JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		outputError(fmt.Errorf("failed to encode output: %w", err))
		return
	}
}

// generateWorktreeName creates a worktree name from the issue title
func generateWorktreeName(title string) string {
	// Process title:
	// 1. Convert to lowercase
	// 2. Replace spaces with dashes
	// 3. Remove special characters
	// 4. Limit to 64 characters
	worktreeName := strings.ToLower(title)
	worktreeName = regexp.MustCompile(`\s+`).ReplaceAllString(worktreeName, "-")
	worktreeName = regexp.MustCompile(`[^a-z0-9\-]`).ReplaceAllString(worktreeName, "")

	// Limit to 64 characters
	if len(worktreeName) > 64 {
		worktreeName = worktreeName[:64]
	}

	// Remove trailing dashes
	worktreeName = strings.TrimRight(worktreeName, "-")

	// Remove any double dashes
	worktreeName = regexp.MustCompile(`\-+`).ReplaceAllString(worktreeName, "-")

	// Remove leading/trailing dashes
	worktreeName = strings.Trim(worktreeName, "-")

	return worktreeName
}

// outputError outputs an error message in the expected format
func outputError(err error) {
	// Simple error output as JSON
	fmt.Printf("{\"error\": \"%s\"}\n", err.Error())
	os.Exit(1)
}