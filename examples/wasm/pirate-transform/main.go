package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Input represents the expected input structure
type Input struct {
	Prompt string `json:"prompt"`
	Input  string `json:"input"`  // Fallback for direct API calls
}

// Output represents the output structure
type Output struct {
	Message string `json:"message"`
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

	// Pass through the input and append pirate instruction
	// Check prompt first, then input field, then default message
	originalText := input.Prompt
	if originalText == "" {
		originalText = input.Input
	}
	if originalText == "" {
		originalText = "Arrr, I need something to say!"
	}

	// Create output with original text + pirate instruction
	outputText := originalText + "\nSay this in pirate speak"

	// Create output
	output := Output{
		Message: outputText,
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
