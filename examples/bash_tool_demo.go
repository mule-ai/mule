package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/mule-ai/mule/internal/tools"
)

func main() {
	// Create a new bash tool
	bashTool := tools.NewBashTool()

	// Example 1: Simple command execution
	fmt.Println("=== Example 1: Simple command execution ===")
	params := map[string]interface{}{
		"command": "echo 'Hello from bash tool!'",
	}

	result, err := bashTool.Execute(context.Background(), params)
	if err != nil {
		log.Fatalf("Error executing command: %v", err)
	}

	// Pretty print the result
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(resultJSON))

	// Example 2: Command that lists files
	fmt.Println("\n=== Example 2: List files ===")
	params = map[string]interface{}{
		"command": "ls -la",
	}

	result, err = bashTool.Execute(context.Background(), params)
	if err != nil {
		log.Fatalf("Error executing command: %v", err)
	}

	// Pretty print the result
	resultJSON, _ = json.MarshalIndent(result, "", "  ")
	fmt.Println(string(resultJSON))

	// Example 3: Command that fails
	fmt.Println("\n=== Example 3: Command that fails ===")
	params = map[string]interface{}{
		"command": "exit 1",
	}

	result, err = bashTool.Execute(context.Background(), params)
	if err != nil {
		log.Fatalf("Error executing command: %v", err)
	}

	// Pretty print the result
	resultJSON, _ = json.MarshalIndent(result, "", "  ")
	fmt.Println(string(resultJSON))
}
