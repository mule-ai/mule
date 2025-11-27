package main

import (
	"context"
	"fmt"
	"os"

	"github.com/tetratelabs/wazero"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run wasm_inspector.go <wasm_file>")
		os.Exit(1)
	}

	wasmFile := os.Args[1]
	wasmBytes, err := os.ReadFile(wasmFile)
	if err != nil {
		fmt.Printf("Error reading WASM file: %v\n", err)
		os.Exit(1)
	}

	// Create a runtime just to inspect the module
	ctx := context.Background()
	runtime := wazero.NewRuntime(ctx)
	defer runtime.Close(ctx)

	// Compile the module to inspect it
	module, err := runtime.CompileModule(ctx, wasmBytes)
	if err != nil {
		fmt.Printf("Error compiling WASM module: %v\n", err)
		os.Exit(1)
	}

	// Print imports
	imports := module.ImportedFunctions()
	if len(imports) > 0 {
		fmt.Println("Imported Functions:")
		for _, imp := range imports {
			moduleName, functionName, _ := imp.Import()
			fmt.Printf("  Module: %s, Function: %s\n", moduleName, functionName)
			
			// Show parameter and result types
			params := imp.ParamTypes()
			results := imp.ResultTypes()
			fmt.Printf("    Params: %v, Results: %v\n", params, results)
		}
	} else {
		fmt.Println("No imported functions found")
	}

	// Check specifically for env imports
	envImports := 0
	for _, imp := range imports {
		moduleName, _, _ := imp.Import()
		if moduleName == "env" {
			envImports++
		}
	}
	
	if envImports > 0 {
		fmt.Printf("\nFound %d imports from 'env' module\n", envImports)
	} else {
		fmt.Println("\nNo imports from 'env' module found")
	}
}