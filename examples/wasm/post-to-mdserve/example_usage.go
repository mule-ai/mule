package main

import (
	"encoding/json"
	"fmt"
	"log"
)

// Example of how to use the post-to-mdserve WASM module
func main() {
	// Example input for testing
	input := map[string]interface{}{
		"prompt":   "# Test Document\n\nThis is a test markdown document.",
		"endpoint": "https://md.butler.ooo/api/document",
	}
	
	inputJSON, err := json.MarshalIndent(input, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("Example input for the post-to-mdserve WASM module:\n%s\n", inputJSON)
	
	// Example configuration
	config := map[string]interface{}{
		"endpoint": "https://md.butler.ooo/api/document",
	}
	
	configJSON, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("\nExample configuration for the module:\n%s\n", configJSON)
	
	// Expected output format
	fmt.Printf("\nExpected output format (just the URL):\n%s\n", "https://md.butler.ooo/test")
}