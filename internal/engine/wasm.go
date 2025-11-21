package engine

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"runtime/debug"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
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

	log.Printf("Executing WASM module %s (size: %d bytes)", moduleID, len(moduleData))

	// Add panic recovery for WASI-related issues
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from WASM execution panic: %v", r)
			// Log stack trace for debugging
			log.Printf("Stack trace: %s", debug.Stack())
		}
	}()

	// Create buffers to capture stdout and stderr
	var stdoutBuf, stderrBuf bytes.Buffer

	// Create a fresh runtime for each execution to avoid "randinit twice" error
	// This is necessary for Go-compiled WASM modules which have single-execution lifecycle
	runtime := wazero.NewRuntime(ctx)

	// Instantiate WASI - provides system functions for Go WASM
	// This sets up clock_time_get, random_get, and other system functions
	// The standard Instantiate function properly configures all system functions for wazero 1.10.1
	_, err = wasi_snapshot_preview1.Instantiate(ctx, runtime)
	if err != nil {
		runtime.Close(ctx)
		return nil, fmt.Errorf("failed to instantiate WASI: %w", err)
	}

	// Configure module with captured stdout/stderr and start function
	// WithStartFunctions("_initialize") is CRITICAL for Go-compiled WASM
	// It ensures the Go runtime is properly initialized before main() runs
	config := wazero.NewModuleConfig().
		WithStdout(&stdoutBuf).
		WithStderr(&stderrBuf).
		WithStartFunctions("_initialize")

	// Compile the module first
	compiledModule, err := runtime.CompileModule(ctx, moduleData)
	if err != nil {
		runtime.Close(ctx)
		return nil, fmt.Errorf("failed to compile WASM module: %w", err)
	}

	log.Printf("WASM module compiled successfully, instantiating...")

	// Instantiate the module WITHOUT auto-starting
	instance, err := runtime.InstantiateModule(ctx, compiledModule, config)
	if err != nil {
		runtime.Close(ctx)
		return nil, fmt.Errorf("failed to instantiate WASM module: %w", err)
	}

	log.Printf("WASM module instantiated successfully")

	// Call _initialize to set up Go runtime
	if initFunc := instance.ExportedFunction("_initialize"); initFunc != nil {
		log.Printf("Calling _initialize...")
		_, err = initFunc.Call(ctx)
		if err != nil {
			runtime.Close(ctx)
			return nil, fmt.Errorf("error calling _initialize: %w", err)
		}
		log.Printf("_initialize executed successfully")
	}

	// Call _start to run main() - capture output during this call
	log.Printf("Calling _start to run main()...")
	if startFunc := instance.ExportedFunction("_start"); startFunc != nil {
		_, err = startFunc.Call(ctx)
		// Check if we got a sys.ExitError (which is normal for Go-compiled WASM)
		if exitErr, ok := err.(*sys.ExitError); ok {
			// This is expected for Go-compiled WASM modules - they call proc_exit after main()
			log.Printf("WASM module exited with code: %d (normal for Go WASM)", exitErr.ExitCode())
		} else if err != nil {
			runtime.Close(ctx)
			return nil, fmt.Errorf("error calling _start: %w", err)
		}
		log.Printf("_start executed successfully")
	} else {
		runtime.Close(ctx)
		return nil, fmt.Errorf("_start function not found")
	}

	// Close instance
	instance.Close(ctx)

	// Note: The main() function should have executed and produced output during _start

	// Close the runtime to ensure all resources are cleaned up
	runtime.Close(ctx)

	// Log the captured output for debugging
	stdoutStr := stdoutBuf.String()
	stderrStr := stderrBuf.String()
	log.Printf("WASM module execution - stdout: '%s', stderr: '%s'", stdoutStr, stderrStr)

	// Try to parse stdout as JSON and extract the message field
	// If it's valid JSON with a "message" field, return just that value
	// Otherwise, return the raw stdout
	var resultValue map[string]interface{}
	var output interface{}

	if stdoutStr != "" {
		if err := json.Unmarshal([]byte(stdoutStr), &resultValue); err == nil {
			// Successfully parsed as JSON
			if msg, ok := resultValue["message"]; ok {
				// Return just the message field
				output = msg
			} else {
				// No message field, return the whole parsed object
				output = resultValue
			}
		} else {
			// Not valid JSON, return as string
			output = stdoutStr
		}
	} else {
		output = ""
	}

	// Return the extracted output
	result := map[string]interface{}{
		"output":  output,
		"stdout":  stdoutStr,
		"stderr":  stderrStr,
		"message": "WASM module executed successfully",
		"success": true,
	}

	return result, nil
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

// getModuleData retrieves WASM module data from cache or database
func (e *WASMExecutor) getModuleData(ctx context.Context, moduleID string) ([]byte, error) {
	// Check if module is already cached
	if data, ok := e.modules[moduleID]; ok {
		return data, nil
	}

	// Load from database
	var moduleData []byte
	err := e.db.QueryRowContext(ctx, "SELECT module_data FROM wasm_modules WHERE id = $1", moduleID).Scan(&moduleData)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch WASM module from database: %w", err)
	}

	// Cache the module data
	e.modules[moduleID] = moduleData

	return moduleData, nil
}

// Close closes the WASM executor and cleans up cached modules
func (e *WASMExecutor) Close(ctx context.Context) error {
	// Clear the cache
	e.modules = make(map[string][]byte)
	return nil
}
