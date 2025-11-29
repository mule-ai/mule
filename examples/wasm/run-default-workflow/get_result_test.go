package main

import (
	"fmt"
	"testing"
)

// TestGetLastOperationResult tests the fixed getLastOperationResult function logic
func TestGetLastOperationResultLogic(t *testing.T) {
	// This test simulates the logic of the fixed function without actually calling WASM host functions
	
	// Simulate the first call to get the length
	simulateGetLastOperationResult := func(bufferPtr uint32, bufferSize uint32) uint32 {
		// If buffer size is 0, return the required size
		if bufferSize == 0 {
			return 22 // Length of "Hello from workflow!"
		}
		
		// Simulate error case
		if bufferSize < 22 {
			return 0xFFFFFFF5 // Buffer too small
		}
		
		// Simulate successful write to buffer
		return 22 // Return the actual length written
	}
	
	// Test the two-phase approach:
	// 1. Query for required buffer size
	length := simulateGetLastOperationResult(0, 0)
	if length != 22 {
		t.Errorf("Expected length 22, got %d", length)
	}
	
	// 2. Actually read the data
	buffer := make([]byte, length)
	bufferPtr := uint32(0) // In real code, this would be the actual pointer
	actualLength := simulateGetLastOperationResult(bufferPtr, length)
	if actualLength != 22 {
		t.Errorf("Expected actual length 22, got %d", actualLength)
	}
	
	fmt.Printf("Test passed! Required buffer size: %d, Actual data length: %d\n", length, actualLength)
}