package engine

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"runtime/debug"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

// WASMExecutor handles WebAssembly module execution
type WASMExecutor struct {
	db      *sql.DB
	modules map[string][]byte // Store compiled module bytes instead of instantiated modules
}

// NewWASMExecutor creates a new WASM executor
func NewWASMExecutor(db *sql.DB) *WASMExecutor {
	return &WASMExecutor{
		db:      db,
		modules: make(map[string][]byte),
	}
}

// Execute executes a WASM module with the given input data
func (e *WASMExecutor) Execute(ctx context.Context, moduleID string, inputData map[string]interface{}) (map[string]interface{}, error) {
	// Get module data from cache or load it
	moduleData, err := e.getModuleData(ctx, moduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get WASM module data: %w", err)
	}

	// Add panic recovery for WASI-related issues
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from WASM execution panic: %v", r)
			// Log stack trace for debugging
			log.Printf("Stack trace: %s", debug.Stack())
		}
	}()

	// Create a fresh runtime for each execution to avoid "randinit twice" error
	// This is necessary for Go-compiled WASM modules which have single-execution lifecycle
	runtime := wazero.NewRuntime(ctx)

	// Instantiate WASI with proper system walltime
	_, err = wasi_snapshot_preview1.Instantiate(ctx, runtime)
	if err != nil {
		runtime.Close(ctx)
		return nil, fmt.Errorf("failed to instantiate WASI: %w", err)
	}

	// Create module configuration with system walltime
	config := wazero.NewModuleConfig().WithSysWalltime().
		WithStdout(os.Stdout).
		WithStderr(os.Stderr)

	// Compile and instantiate the module
	module, err := runtime.InstantiateWithConfig(ctx, moduleData, config)
	if err != nil {
		runtime.Close(ctx)
		return nil, fmt.Errorf("failed to instantiate WASM module: %w", err)
	}
	defer func() {
		module.Close(ctx)
		runtime.Close(ctx)
	}() // Clean up after execution

	// Call the main function (or _start for WASI programs)
	mainFunc := module.ExportedFunction("_start")
	if mainFunc == nil {
		// Fall back to main for compatibility
		mainFunc = module.ExportedFunction("main")
	}
	if mainFunc == nil {
		return nil, fmt.Errorf("module does not export '_start' or 'main' function")
	}

	// For simple WASI programs, just call the entry point without memory management
	// TODO: Implement proper data passing mechanism for more complex modules
	_, err = mainFunc.Call(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to call main function: %w", err)
	}

	// For now, return a simple success result
	// TODO: Capture and return actual program output
	result := map[string]interface{}{
		"success": true,
		"message": "WASM module executed successfully",
	}

	return result, nil
}

// getModuleData retrieves WASM module data, loading it if necessary
func (e *WASMExecutor) getModuleData(ctx context.Context, moduleID string) ([]byte, error) {
	// Check cache first
	if moduleData, exists := e.modules[moduleID]; exists {
		return moduleData, nil
	}

	// Load module from database
	query := `SELECT module_data FROM wasm_modules WHERE id = $1`
	var moduleData []byte
	err := e.db.QueryRowContext(ctx, query, moduleID).Scan(&moduleData)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("WASM module not found: %s", moduleID)
		}
		return nil, fmt.Errorf("failed to load WASM module from database: %w", err)
	}

	// Cache the module data
	e.modules[moduleID] = moduleData

	return moduleData, nil
}

// LoadModule loads a WASM module from the database
func (e *WASMExecutor) LoadModule(ctx context.Context, moduleID string) error {
	// Pre-load the module data
	_, err := e.getModuleData(ctx, moduleID)
	if err != nil {
		return fmt.Errorf("failed to load WASM module: %w", err)
	}

	log.Printf("Pre-loaded WASM module %s", moduleID)
	return nil
}

// Close closes the WASM executor and cleans up cached modules
func (e *WASMExecutor) Close(ctx context.Context) error {
	// Clear the cache
	e.modules = make(map[string][]byte)
	return nil
}
