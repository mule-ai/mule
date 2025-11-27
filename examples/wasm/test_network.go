package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/mule-ai/mule/internal/engine"
	_ "github.com/lib/pq"
)

func main() {
	// Connect to database
	db, err := sql.Open("postgres", "postgres://mule:mule@localhost:5432/mulev2?sslmode=disable")
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Read the WASM module
	wasmData, err := os.ReadFile("/tmp/wasm-network-test/network_example.wasm")
	if err != nil {
		log.Fatalf("Failed to read WASM module: %v", err)
	}

	// Create a test module in the database
	ctx := context.Background()
	moduleID := "test-network-module"
	
	// Insert the module into the database
	_, err = db.ExecContext(ctx, `
		INSERT INTO wasm_modules (id, name, description, module_data, created_at) 
		VALUES ($1, $2, $3, $4, NOW()) 
		ON CONFLICT (id) DO UPDATE SET 
		name = $2, description = $3, module_data = $4`,
		moduleID, "Network Test Module", "Test module for network functionality", wasmData)
	if err != nil {
		log.Fatalf("Failed to insert WASM module: %v", err)
	}

	// Create WASM executor
	executor := engine.NewWASMExecutor(db)
	
	// Set URL allow list to allow httpbin.org for testing
	executor.SetURLAllowList([]string{"https://httpbin.org/", "http://httpbin.org/"})

	// Test input data with a URL
	inputData := map[string]interface{}{
		"url": "https://httpbin.org/get",
		"data": map[string]interface{}{
			"test": "value",
		},
	}

	// Execute the WASM module
	result, err := executor.Execute(ctx, moduleID, inputData)
	if err != nil {
		log.Fatalf("Failed to execute WASM module: %v", err)
	}

	// Print the result
	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	fmt.Printf("WASM Execution Result:\n%s\n", resultJSON)
}