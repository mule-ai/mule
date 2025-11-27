package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/tetratelabs/wazero"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run wasm_inspector.go <wasm_file>")
		os.Exit(1)
	}

	wasmFile := os.Args[1]
	
	// Read the WASM file
	wasmBytes, err := os.ReadFile(wasmFile)
	if err != nil {
		log.Fatalf("Failed to read WASM file: %v", err)
	}

	fmt.Printf("Inspecting WASM file: %s\n", wasmFile)
	fmt.Printf("File size: %d bytes\n\n", len(wasmBytes))

	// Create a context
	ctx := context.Background()

	// Create a runtime
	runtime := wazero.NewRuntime(ctx)
	defer runtime.Close(ctx)

	// Compile the module to inspect its imports
	compiledModule, err := runtime.CompileModule(ctx, wasmBytes)
	if err != nil {
		log.Fatalf("Failed to compile WASM module: %v", err)
	}

	// Show imported functions
	showImportedFunctions(compiledModule)
	
	// Show exported functions
	showExportedFunctions(compiledModule)
	
	// Show memory information
	showMemoryInfo(compiledModule)
}

func showImportedFunctions(compiledModule wazero.CompiledModule) {
	imports := compiledModule.ImportedFunctions()
	if len(imports) == 0 {
		fmt.Println("No imported functions found.")
		return
	}

	fmt.Printf("Found %d imported functions:\n", len(imports))
	
	envImports := 0
	for i, imp := range imports {
		moduleName, functionName, _ := imp.Import()
		
		fmt.Printf("%d. Module: %s, Function: %s\n", i+1, moduleName, functionName)
		
		// Show parameter and result types
		params := imp.ParamTypes()
		results := imp.ResultTypes()
		if len(params) > 0 || len(results) > 0 {
			fmt.Printf("   Params: %v, Results: %v\n", params, results)
		}
		
		if moduleName == "env" {
			envImports++
		}
	}
	
	fmt.Printf("\nSummary for imports:\n")
	fmt.Printf("- Total imports: %d\n", len(imports))
	fmt.Printf("- Imports from 'env' module: %d\n", envImports)
	
	if envImports > 0 {
		fmt.Println("\nWarning: This module imports functions from the 'env' module.")
		fmt.Println("Make sure these functions are properly provided during execution.")
	}
	fmt.Println()
}

func showExportedFunctions(compiledModule wazero.CompiledModule) {
	exports := compiledModule.ExportedFunctions()
	if len(exports) == 0 {
		fmt.Println("No exported functions found.")
		return
	}

	fmt.Printf("Found %d exported functions:\n", len(exports))
	
	i := 0
	for name, exp := range exports {
		i++
		fmt.Printf("%d. Function: %s\n", i, name)
		
		// Show parameter and result types
		params := exp.ParamTypes()
		results := exp.ResultTypes()
		if len(params) > 0 || len(results) > 0 {
			fmt.Printf("   Params: %v, Results: %v\n", params, results)
		}
	}
	fmt.Println()
}

func showMemoryInfo(compiledModule wazero.CompiledModule) {
	// Show imported memory
	importedMemories := compiledModule.ImportedMemories()
	if len(importedMemories) > 0 {
		fmt.Printf("Found %d imported memories:\n", len(importedMemories))
		for i, mem := range importedMemories {
			moduleName, memoryName, _ := mem.Import()
			minPages := mem.Min()
			if maxPages, hasMax := mem.Max(); hasMax {
				fmt.Printf("%d. Module: %s, Memory: %s\n", i+1, moduleName, memoryName)
				fmt.Printf("   Size: %d-%d pages (%d-%d KB)\n", minPages, maxPages, minPages*64, maxPages*64)
			} else {
				fmt.Printf("%d. Module: %s, Memory: %s\n", i+1, moduleName, memoryName)
				fmt.Printf("   Size: %d+ pages (%d+ KB)\n", minPages, minPages*64)
			}
		}
		fmt.Println()
	}

	// Show exported memory
	exportedMemories := compiledModule.ExportedMemories()
	if len(exportedMemories) > 0 {
		fmt.Printf("Found %d exported memories:\n", len(exportedMemories))
		i := 0
		for name, mem := range exportedMemories {
			i++
			minPages := mem.Min()
			if maxPages, hasMax := mem.Max(); hasMax {
				fmt.Printf("%d. Memory: %s\n", i, name)
				fmt.Printf("   Size: %d-%d pages (%d-%d KB)\n", minPages, maxPages, minPages*64, maxPages*64)
			} else {
				fmt.Printf("%d. Memory: %s\n", i, name)
				fmt.Printf("   Size: %d+ pages (%d+ KB)\n", minPages, minPages*64)
			}
		}
		fmt.Println()
	}

	if len(importedMemories) == 0 && len(exportedMemories) == 0 {
		fmt.Println("No imported or exported memories found.")
		fmt.Println()
	}
}