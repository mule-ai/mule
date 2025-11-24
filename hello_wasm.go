package main

import (
	"encoding/json"
	"fmt"
)

// main is the entry point that will be called from the WASM runtime
func main() {
	// This is a simple test function that returns a greeting
	result := map[string]interface{}{
		"message": "Hello World from WASM!",
		"success": true,
	}

	// Convert result to JSON and return
	resultJSON, _ := json.Marshal(result)
	fmt.Print(string(resultJSON))
}

// Export a function that can be called from the host
//
//go:wasmexport add
func add(a, b int32) int32 {
	return a + b
}

//go:wasmexport multiply
func multiply(a, b int32) int32 {
	return a * b
}

//go:wasmexport greet
func greet(ptr, size uint32) uint64 {
	// This is a simplified version - in a real implementation,
	// you'd read from memory and write back the result

	// Create a simple response
	response := "Hello from WASM greet function!"

	// For this example, we'll just return a pointer to a static string
	// In a real implementation, you'd allocate memory and write the response
	return uint64(uint32(len(response)))<<32 | uint64(0) // placeholder
}
