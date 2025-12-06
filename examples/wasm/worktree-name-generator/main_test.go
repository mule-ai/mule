package main

import (
	"testing"
)

func TestGenerateWorktreeName(t *testing.T) {
	tests := []struct {
		title    string
		expected string
	}{
		{"Feature: Add MCP client support", "feature-add-mcp-client-support"},
		{"My test issue", "my-test-issue"},
		{"Issue with special chars !@#$%", "issue-with-special-chars"},
		{"Very long issue title that exceeds the character limit significantly and needs to be truncated to fit within the sixty four character limit", "very-long-issue-title-that-exceeds-the-character-limit-significa"},
		{"Multiple   spaces   in   title", "multiple-spaces-in-title"},
	}

	for _, test := range tests {
		result := generateWorktreeName(test.title)
		if result != test.expected {
			t.Errorf("generateWorktreeName(%q) = %q; expected %q", test.title, result, test.expected)
		}
	}

	// Test with a simple title
	title := "Test issue"
	result := generateWorktreeName(title)

	// Check that it follows the pattern
	if result != "test-issue" {
		t.Errorf("generateWorktreeName(%q) = %q; expected 'test-issue'", title, result)
	}

	// Check that it's not longer than 64 characters
	if len(result) > 64 {
		t.Errorf("generateWorktreeName(%q) = %q; length %d exceeds 64 characters", title, result, len(result))
	}
}