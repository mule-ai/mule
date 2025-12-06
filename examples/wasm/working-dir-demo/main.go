package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

	// Get current working directory
	currentDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting current directory: %v\n", err)
		os.Exit(1)
	}

	// Create a new directory for demonstration
	newDir := filepath.Join(currentDir, "demo_subdir")
	if err := os.MkdirAll(newDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directory: %v\n", err)
		os.Exit(1)
	}

	// Change working directory by calling the host function
	// In a real implementation, this would be done through the host function
	// For now, we'll just demonstrate the concept
	
	// Create a file in the current directory
	testFile := filepath.Join(currentDir, "test_file.txt")
	if err := os.WriteFile(testFile, []byte("This file was created in the original working directory"), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating test file: %v\n", err)
		os.Exit(1)
	}

	// Create a file in the new directory
	subdirFile := filepath.Join(newDir, "subdir_file.txt")
	if err := os.WriteFile(subdirFile, []byte("This file was created in the subdirectory"), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating subdir file: %v\n", err)
		os.Exit(1)
	}

	// Create output showing what we did
	output := Output{
		Message: fmt.Sprintf("Created files in directories:\n- %s\n- %s", testFile, subdirFile),
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