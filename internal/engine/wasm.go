package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"

	"github.com/mule-ai/mule/internal/primitive"
)

// WASMExecutor handles WebAssembly module execution
type WASMExecutor struct {
	runtime wazero.Runtime
	store   primitive.PrimitiveStore
	modules map[string]api.Module
}

// NewWASMExecutor creates a new WASM executor
func NewWASMExecutor(store primitive.PrimitiveStore) *WASMExecutor {
	ctx := context.Background()
	runtime := wazero.NewRuntime(ctx)

	return &WASMExecutor{
		runtime: runtime,
		store:   store,
		modules: make(map[string]api.Module),
	}
}

// Execute executes a WASM module with the given input data
func (e *WASMExecutor) Execute(ctx context.Context, moduleID string, inputData map[string]interface{}) (map[string]interface{}, error) {
	// Get module from cache or load it
	module, err := e.getModule(ctx, moduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get WASM module: %w", err)
	}

	// Convert input data to JSON
	inputJSON, err := json.Marshal(inputData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input data: %w", err)
	}

	// Call the main function
	mainFunc := module.ExportedFunction("main")
	if mainFunc == nil {
		return nil, fmt.Errorf("module does not export 'main' function")
	}

	// Allocate memory for input string
	malloc := module.ExportedFunction("malloc")
	if malloc == nil {
		return nil, fmt.Errorf("module does not export 'malloc' function")
	}

	inputSize := uint64(len(inputJSON))
	ptr, err := malloc.Call(ctx, inputSize)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate memory: %w", err)
	}

	// Write input data to memory
	memory := module.ExportedMemory("memory")
	if memory == nil {
		return nil, fmt.Errorf("module does not export 'memory'")
	}

	if !memory.Write(uint32(ptr[0]), inputJSON) {
		return nil, fmt.Errorf("failed to write input data to memory")
	}

	// Call main function
	results, err := mainFunc.Call(ctx, ptr[0], inputSize)
	if err != nil {
		return nil, fmt.Errorf("failed to call main function: %w", err)
	}

	// Read result from memory
	resultPtr := uint32(results[0])

	// Find null terminator to determine string length
	var resultSize uint64 = 0
	for {
		b, ok := memory.ReadByte(resultPtr + uint32(resultSize))
		if !ok || b == 0 {
			break
		}
		resultSize++
	}

	// Read result string
	resultBytes, ok := memory.Read(resultPtr, uint32(resultSize))
	if !ok {
		return nil, fmt.Errorf("failed to read result from memory")
	}

	// Free allocated memory
	free := module.ExportedFunction("free")
	if free != nil {
		_, _ = free.Call(ctx, ptr[0])
	}

	// Parse result JSON
	var result map[string]interface{}
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return result, nil
}

// getModule retrieves a WASM module, loading it if necessary
func (e *WASMExecutor) getModule(ctx context.Context, moduleID string) (api.Module, error) {
	// Check cache first
	if module, exists := e.modules[moduleID]; exists {
		return module, nil
	}

	// Load module from database (this would need to be implemented)
	// For now, we'll return an error
	return nil, fmt.Errorf("WASM module loading not yet implemented")
}

// LoadModule loads a WASM module from the database
func (e *WASMExecutor) LoadModule(ctx context.Context, moduleID string) error {
	// This would load the module from the database and compile it
	// For now, it's a placeholder
	log.Printf("Loading WASM module %s", moduleID)
	return nil
}

// Close closes the WASM runtime
func (e *WASMExecutor) Close(ctx context.Context) error {
	return e.runtime.Close(ctx)
}
