package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Input represents the flexible input structure
type Input struct {
	Prompt   string                 `json:"prompt"`
	Options  map[string]interface{} `json:"options,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// Output represents the output structure with additional metadata
type Output struct {
	Message     string                 `json:"message"`
	ProcessedAt string                 `json:"processed_at"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	Options     map[string]interface{} `json:"options,omitempty"`
}

func main() {
	// Read input from stdin
	inputData, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
		os.Exit(1)
	}

	// Parse input JSON
	var input Input
	if len(inputData) > 0 {
		if err := json.Unmarshal(inputData, &input); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing input JSON: %v\n", err)
			fmt.Fprintf(os.Stderr, "Input data: %s\n", string(inputData))
			os.Exit(1)
		}
	}

	// Process based on options
	operation := "uppercase"
	if input.Options != nil {
		if op, ok := input.Options["operation"].(string); ok {
			operation = op
		}
	}

	var processedText string
	switch operation {
	case "uppercase":
		processedText = strings.ToUpper(input.Prompt)
	case "lowercase":
		processedText = strings.ToLower(input.Prompt)
	case "reverse":
		runes := []rune(input.Prompt)
		for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
			runes[i], runes[j] = runes[j], runes[i]
		}
		processedText = string(runes)
	case "count":
		processedText = fmt.Sprintf("Character count: %d", len(input.Prompt))
	default:
		processedText = fmt.Sprintf("Unknown operation: %s", operation)
	}

	if input.Prompt == "" {
		processedText = "NO INPUT PROVIDED"
	}

	// Create output with metadata
	output := Output{
		Message:  processedText,
		Metadata: input.Metadata,
		Options:  input.Options,
	}

	// Serialize output to JSON
	outputData, err := json.Marshal(output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error serializing output: %v\n", err)
		os.Exit(1)
	}

	// Write output to stdout
	fmt.Print(string(outputData))
}
