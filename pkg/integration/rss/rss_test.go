package rss

import (
	"testing"
)

func TestNormalizeWhitespace(t *testing.T) {
	// Test cases
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			"Multiple spaces",
			"Hello    world",
			"Hello world",
		},
		{
			"Multiple tabs",
			"Hello\t\tworld",
			"Hello world",
		},
		{
			"Mixed horizontal whitespace",
			"Hello \t  world",
			"Hello world",
		},
		{
			"Double newlines preserved",
			"Paragraph 1.\n\nParagraph 2.",
			"Paragraph 1.\n\nParagraph 2.",
		},
		{
			"Excessive newlines reduced",
			"Paragraph 1.\n\n\n\nParagraph 2.",
			"Paragraph 1.\n\nParagraph 2.",
		},
		{
			"Single newlines converted to spaces",
			"Line 1\nLine 2\nLine 3",
			"Line 1 Line 2 Line 3",
		},
		{
			"Multiple paragraph breaks preserved",
			"Line 1\n\nLine 2\n\nLine 3",
			"Line 1\n\nLine 2\n\nLine 3",
		},
		{
			"Leading and trailing newlines trimmed",
			"\n\n\nHello world\n\n\n",
			"Hello world",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := normalizeWhitespace(tc.input)
			if result != tc.expected {
				t.Errorf("normalizeWhitespace(%q) = %q; expected %q", tc.input, result, tc.expected)
			}
		})
	}
}
