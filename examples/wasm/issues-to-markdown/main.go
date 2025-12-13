package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// GitHubIssue represents a GitHub issue structure based on the provided example
type GitHubIssue struct {
	Body     string `json:"body"`
	DueDate  string `json:"due_date"`
	Filter   string `json:"filter"`
	State    string `json:"state"`
	Status   string `json:"status"`
	Title    string `json:"title"`
	URL      string `json:"url"`
}

// Input represents the expected input structure
type Input struct {
	Result []GitHubIssue `json:"result"`
}

// Output represents the output structure
type Output struct {
	Message string `json:"message,omitempty"`
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

func main() {
	// Read input from stdin
	var input Input
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&input); err != nil {
		outputError(fmt.Errorf("failed to decode input: %w", err))
		return
	}

	// Convert issues to markdown
	markdown := convertIssuesToMarkdown(input.Result)

	// Create output
	output := Output{
		Message: markdown,
		Success: true,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		outputError(fmt.Errorf("failed to encode output: %w", err))
		return
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

// convertIssuesToMarkdown converts a slice of GitHub issues to markdown format
func convertIssuesToMarkdown(issues []GitHubIssue) string {
	var builder strings.Builder

	for i, issue := range issues {
		// Add title as heading
		builder.WriteString(fmt.Sprintf("# %s\n\n", issue.Title))

		// Add metadata with transformed URL
		builder.WriteString(fmt.Sprintf("* Link: %s\n", transformURL(issue.URL)))
		builder.WriteString(fmt.Sprintf("* State: %s\n", formatState(issue.State, issue.Status)))
		builder.WriteString(fmt.Sprintf("* Due Date: %s\n", formatDueDate(issue.DueDate)))

		// Add description/body
		builder.WriteString("* Description: ")
		if issue.Body != "" {
			// Check if body contains newlines or list items
			if strings.Contains(issue.Body, "\n") || strings.Contains(issue.Body, "- ") {
				builder.WriteString("\n")
				// Indent each line of the body
				lines := strings.Split(issue.Body, "\n")
				for _, line := range lines {
					if line != "" {
						builder.WriteString(fmt.Sprintf("  %s\n", line))
					} else {
						builder.WriteString("\n")
					}
				}
			} else {
				builder.WriteString(fmt.Sprintf("%s\n", issue.Body))
			}
		} else {
			builder.WriteString("\n")
		}

		// Add separator except for the last issue
		if i < len(issues)-1 {
			builder.WriteString("\n-----\n\n")
		}
	}

	return builder.String()
}

// transformURL converts GitHub API URLs to regular GitHub URLs
func transformURL(apiURL string) string {
	// Replace "https://api.github.com/repos/" with "https://github.com/"
	return strings.Replace(apiURL, "https://api.github.com/repos/", "https://github.com/", 1)
}

// formatState formats the state based on both state and status fields
func formatState(state, status string) string {
	if status != "" {
		// Capitalize first letter of status
		if len(status) > 0 {
			return strings.ToUpper(status[:1]) + strings.ToLower(status[1:])
		}
		return status
	}
	
	// Default to state if status is empty
	return strings.ToUpper(state[:1]) + strings.ToLower(state[1:])
}

// formatDueDate formats the due date, handling "No Due Date" case
func formatDueDate(dueDate string) string {
	if dueDate == "No Due Date" {
		return dueDate
	}
	
	// Try to parse the date and reformat it as MM/DD/YY
	if parsedDate, err := time.Parse("2006-01-02", dueDate); err == nil {
		return parsedDate.Format("1/2/06")
	}
	
	// Return as is if parsing fails
	return dueDate
}
