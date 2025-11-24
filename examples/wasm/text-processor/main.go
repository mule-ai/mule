package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Input represents the expected input structure
type Input struct {
	Prompt string `json:"prompt"`
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

	// Parse input JSON (if any)
	var input Input
	if len(inputData) > 0 {
		if err := json.Unmarshal(inputData, &input); err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing input JSON: %v\n", err)
			fmt.Fprintf(os.Stderr, "Input data: %s\n", string(inputData))
			os.Exit(1)
		}
	}

	// Process the input (example: convert to uppercase)
	processedText := strings.ToUpper(input.Prompt)
	if processedText == "" {
		processedText = "NO INPUT PROVIDED"
	}

	// Create output
	output := Output{
		Message: processedText,
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
