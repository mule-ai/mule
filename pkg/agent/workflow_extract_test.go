package agent

import (
	"testing"
)

func TestExtractReasoning(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "No think tags",
			input:    "VALID: The plan is comprehensive and well-structured.",
			expected: "VALID: The plan is comprehensive and well-structured.",
		},
		{
			name:     "Think tags with content after",
			input:    "<think>Internal reasoning here</think>VALID: The plan is comprehensive.",
			expected: "VALID: The plan is comprehensive.",
		},
		{
			name:     "Think tags with content before",
			input:    "Response: <think>Internal reasoning</think> The answer is 42.",
			expected: "Response:  The answer is 42.",
		},
		{
			name:     "Think tags only",
			input:    "<think>Only thinking content</think>",
			expected: "",
		},
		{
			name:     "Multiple lines with think tags",
			input:    "Before\n<think>Reasoning\nacross lines</think>\nAfter",
			expected: "Before\n\nAfter",
		},
		{
			name:     "Incomplete think tags - only opening",
			input:    "<think>No closing tag",
			expected: "<think>No closing tag",
		},
		{
			name:     "Missing opening tag - only closing",
			input:    "Some reasoning without opening tag</think>VALID: Response here",
			expected: "VALID: Response here",
		},
		{
			name:     "Only closing tag at end",
			input:    "Content here</think>",
			expected: "",
		},
		{
			name:     "Empty think tags",
			input:    "<think></think>Result",
			expected: "Result",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractReasoning(tt.input)
			if result != tt.expected {
				t.Errorf("extractReasoning(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
