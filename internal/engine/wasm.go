package engine

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// WASMExecutor handles WebAssembly module execution
type WASMExecutor struct {
	db         *sql.DB
	modules    map[string][]byte // Store compiled module bytes instead of instantiated modules
	urlAllowed []string          // List of allowed URL prefixes for HTTP requests
	// Store the last response for each module instance
	lastResponse map[string]*http.Response
	lastResponseBody map[string][]byte
}

// Modules returns the internal modules map for testing purposes
func (e *WASMExecutor) Modules() map[string][]byte {
	return e.modules
}

// NewWASMExecutor creates a new WASM executor
func NewWASMExecutor(db *sql.DB) *WASMExecutor {
	return &WASMExecutor{
		db:         db,
		modules:    make(map[string][]byte),
		urlAllowed: []string{"https://", "http://"}, // Allow all URLs by default (can be configured)
		lastResponse: make(map[string]*http.Response),
		lastResponseBody: make(map[string][]byte),
	}
}

// SetURLAllowList sets the list of allowed URL prefixes for HTTP requests
func (e *WASMExecutor) SetURLAllowList(allowed []string) {
	e.urlAllowed = allowed
}

// Execute executes a WASM module with the given input data
func (e *WASMExecutor) Execute(ctx context.Context, moduleID string, inputData map[string]interface{}) (map[string]interface{}, error) {
	// Get module data from cache or load it
	moduleData, err := e.getModuleData(ctx, moduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get WASM module data: %w", err)
	}

	log.Printf("Executing WASM module %s (size: %d bytes) with input data: %+v", moduleID, len(moduleData), inputData)

	// Add panic recovery for WASI-related issues
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from WASM execution panic: %v", r)
			// Log stack trace for debugging
			log.Printf("Stack trace: %s", debug.Stack())
		}
	}()

	// Serialize input data to JSON for passing to WASM module via stdin
	var stdinData []byte
	if len(inputData) > 0 {
		stdinData, err = json.Marshal(inputData)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize input data: %w", err)
		}
		log.Printf("Passing %d bytes of input data to WASM module via stdin: %s", len(stdinData), string(stdinData))
	} else {
		log.Printf("No input data provided to WASM module (inputData: %+v)", inputData)
	}

	// Create buffers for stdin, stdout, and stderr
	stdinBuf := bytes.NewReader(stdinData)
	var stdoutBuf, stderrBuf bytes.Buffer

	// Create a fresh runtime for each execution to avoid "randinit twice" error
	// This is necessary for Go-compiled WASM modules which have single-execution lifecycle
	runtime := wazero.NewRuntime(ctx)

	// Instantiate WASI - provides system functions for Go WASM
	// This sets up clock_time_get, random_get, and other system functions
	// The standard Instantiate function properly configures all system functions for wazero 1.10.1
	_, err = wasi_snapshot_preview1.Instantiate(ctx, runtime)
	if err != nil {
		func() {
			if closeErr := runtime.Close(ctx); closeErr != nil {
				log.Printf("Failed to close runtime: %v", closeErr)
			}
		}()
		return nil, fmt.Errorf("failed to instantiate WASI: %w", err)
	}

	// Register HTTP host function for making requests
	// This allows WASM modules to make HTTP requests to allowed URLs
	hostModule := runtime.NewHostModuleBuilder("env")

	// Generic HTTP function that supports different methods
	hostModule.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, methodPtr, methodSize, urlPtr, urlSize, bodyPtr, bodySize uint32) uint32 {
			// Get memory from the module
			mem := module.Memory()

			// Read method from WASM memory
			method, err := readStringFromMemory(ctx, mem, methodPtr, methodSize)
			if err != nil {
				log.Printf("Failed to read HTTP method from WASM memory: %v", err)
				// Return error code (0xFFFFFFF0)
				return 0xFFFFFFF0
			}

			// Read URL from WASM memory
			urlStr, err := readStringFromMemory(ctx, mem, urlPtr, urlSize)
			if err != nil {
				log.Printf("Failed to read URL from WASM memory: %v", err)
				// Return error code (0xFFFFFFFF)
				return 0xFFFFFFFF
			}

			// Validate URL
			if !e.isURLAllowed(urlStr) {
				log.Printf("URL not allowed: %s", urlStr)
				// Return error code (0xFFFFFFFE)
				return 0xFFFFFFFE
			}

			// Read body from WASM memory (can be empty for GET requests)
			var bodyReader io.Reader
			if bodySize > 0 {
				bodyStr, err := readStringFromMemory(ctx, mem, bodyPtr, bodySize)
				if err != nil {
					log.Printf("Failed to read HTTP body from WASM memory: %v", err)
					// Return error code (0xFFFFFFF1)
					return 0xFFFFFFF1
				}
				bodyReader = strings.NewReader(bodyStr)
			}

			// Make HTTP request with timeout
			client := &http.Client{
				Timeout: 30 * time.Second,
			}

			req, err := http.NewRequestWithContext(ctx, method, urlStr, bodyReader)
			if err != nil {
				log.Printf("Failed to create HTTP request for URL %s: %v", urlStr, err)
				// Return error code (0xFFFFFFFD)
				return 0xFFFFFFFD
			}

			// Set Content-Type header for POST/PUT requests with body
			if bodyReader != nil && (method == "POST" || method == "PUT") {
				req.Header.Set("Content-Type", "application/json")
			}

			resp, err := client.Do(req)
			if err != nil {
				log.Printf("Failed to make HTTP request to %s: %v", urlStr, err)
				// Return error code (0xFFFFFFFC)
				return 0xFFFFFFFC
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					log.Printf("Failed to close response body: %v", err)
				}
			}()

			// Read response body
			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Printf("Failed to read response body from %s: %v", urlStr, err)
				// Return error code (0xFFFFFFFB)
				return 0xFFFFFFFB
			}

			// Store response data for retrieval by the module
			// Use a unique key for this execution context
			key := fmt.Sprintf("%p", module)
			e.lastResponse[key] = resp
			e.lastResponseBody[key] = respBody

			// For this simplified interface, we'll just log that the request was successful
			log.Printf("HTTP %s request to %s completed successfully with status %d", method, urlStr, resp.StatusCode)

			// Return 0 for success
			return 0
		}).
		Export("http_request")

	// Enhanced HTTP function that supports headers
	hostModule.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, methodPtr, methodSize, urlPtr, urlSize, bodyPtr, bodySize, headersPtr, headersSize uint32) uint32 {
			// Get memory from the module
			mem := module.Memory()

			// Read method from WASM memory
			method, err := readStringFromMemory(ctx, mem, methodPtr, methodSize)
			if err != nil {
				log.Printf("Failed to read HTTP method from WASM memory: %v", err)
				// Return error code (0xFFFFFFF0)
				return 0xFFFFFFF0
			}

			// Read URL from WASM memory
			urlStr, err := readStringFromMemory(ctx, mem, urlPtr, urlSize)
			if err != nil {
				log.Printf("Failed to read URL from WASM memory: %v", err)
				// Return error code (0xFFFFFFFF)
				return 0xFFFFFFFF
			}

			// Validate URL
			if !e.isURLAllowed(urlStr) {
				log.Printf("URL not allowed: %s", urlStr)
				// Return error code (0xFFFFFFFE)
				return 0xFFFFFFFE
			}

			// Read body from WASM memory (can be empty for GET requests)
			var bodyReader io.Reader
			if bodySize > 0 {
				bodyStr, err := readStringFromMemory(ctx, mem, bodyPtr, bodySize)
				if err != nil {
					log.Printf("Failed to read HTTP body from WASM memory: %v", err)
					// Return error code (0xFFFFFFF1)
					return 0xFFFFFFF1
				}
				bodyReader = strings.NewReader(bodyStr)
			}

			// Read headers from WASM memory (can be empty)
			var headers map[string]string
			if headersSize > 0 {
				headersStr, err := readStringFromMemory(ctx, mem, headersPtr, headersSize)
				if err != nil {
					log.Printf("Failed to read HTTP headers from WASM memory: %v", err)
					// Return error code (0xFFFFFFF2)
					return 0xFFFFFFF2
				}

				// Parse headers JSON
				if err := json.Unmarshal([]byte(headersStr), &headers); err != nil {
					log.Printf("Failed to parse HTTP headers JSON: %v", err)
					// Return error code (0xFFFFFFF3)
					return 0xFFFFFFF3
				}
			}

			// Make HTTP request with timeout
			client := &http.Client{
				Timeout: 30 * time.Second,
			}

			req, err := http.NewRequestWithContext(ctx, method, urlStr, bodyReader)
			if err != nil {
				log.Printf("Failed to create HTTP request for URL %s: %v", urlStr, err)
				// Return error code (0xFFFFFFFD)
				return 0xFFFFFFFD
			}

			// Set headers
			for key, value := range headers {
				req.Header.Set(key, value)
			}

			// Set Content-Type header for POST/PUT requests with body if not already set
			if bodyReader != nil && (method == "POST" || method == "PUT") {
				if req.Header.Get("Content-Type") == "" {
					req.Header.Set("Content-Type", "application/json")
				}
			}

			resp, err := client.Do(req)
			if err != nil {
				log.Printf("Failed to make HTTP request to %s: %v", urlStr, err)
				// Return error code (0xFFFFFFFC)
				return 0xFFFFFFFC
			}
			defer func() {
				if err := resp.Body.Close(); err != nil {
					log.Printf("Failed to close response body: %v", err)
				}
			}()

			// Read response body
			respBody, err := io.ReadAll(resp.Body)
			if err != nil {
				log.Printf("Failed to read response body from %s: %v", urlStr, err)
				// Return error code (0xFFFFFFFB)
				return 0xFFFFFFFB
			}

			// Store response data for retrieval by the module
			// Use a unique key for this execution context
			key := fmt.Sprintf("%p", module)
			e.lastResponse[key] = resp
			e.lastResponseBody[key] = respBody

			// For this simplified interface, we'll just log that the request was successful
			log.Printf("HTTP %s request to %s completed successfully with status %d", method, urlStr, resp.StatusCode)

			// Return 0 for success
			return 0
		}).
		Export("http_request_with_headers")

	// Function to get the last response body
	hostModule.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, bufferPtr uint32, bufferSize uint32) uint32 {
			// Get memory from the module
			mem := module.Memory()

			// Get the response body for this module instance
			key := fmt.Sprintf("%p", module)
			respBody, ok := e.lastResponseBody[key]
			if !ok {
				log.Printf("No response body available for module %s", key)
				// Return error code (0xFFFFFFF4)
				return 0xFFFFFFF4
			}

			// Check if buffer is large enough
			if bufferSize < uint32(len(respBody)) {
				log.Printf("Buffer too small for response body: %d < %d", bufferSize, len(respBody))
				// Return error code (0xFFFFFFF5)
				return 0xFFFFFFF5
			}

			// Write response body to WASM memory
			ok = mem.Write(bufferPtr, respBody)
			if !ok {
				log.Printf("Failed to write response body to WASM memory")
				// Return error code (0xFFFFFFF6)
				return 0xFFFFFFF6
			}

			// Return the size of the response body
			return uint32(len(respBody))
		}).
		Export("get_last_response_body")

	// Function to get the last response status code
	hostModule.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module) uint32 {
			// Get the response for this module instance
			key := fmt.Sprintf("%p", module)
			resp, ok := e.lastResponse[key]
			if !ok {
				log.Printf("No response available for module %s", key)
				// Return error code (0xFFFFFFF4)
				return 0xFFFFFFF4
			}

			// Return the status code
			return uint32(resp.StatusCode)
		}).
		Export("get_last_response_status")

	// Function to get the last response header value
	hostModule.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, headerNamePtr, headerNameSize, bufferPtr, bufferSize uint32) uint32 {
			// Get memory from the module
			mem := module.Memory()

			// Read header name from WASM memory
			headerName, err := readStringFromMemory(ctx, mem, headerNamePtr, headerNameSize)
			if err != nil {
				log.Printf("Failed to read header name from WASM memory: %v", err)
				// Return error code (0xFFFFFFF7)
				return 0xFFFFFFF7
			}

			// Get the response for this module instance
			key := fmt.Sprintf("%p", module)
			resp, ok := e.lastResponse[key]
			if !ok {
				log.Printf("No response available for module %s", key)
				// Return error code (0xFFFFFFF4)
				return 0xFFFFFFF4
			}

			// Get the header value
			headerValue := resp.Header.Get(headerName)
			if headerValue == "" {
				log.Printf("Header %s not found in response", headerName)
				// Return 0 to indicate header not found
				return 0
			}

			// Check if buffer is large enough
			if bufferSize < uint32(len(headerValue)) {
				log.Printf("Buffer too small for header value: %d < %d", bufferSize, len(headerValue))
				// Return error code (0xFFFFFFF5)
				return 0xFFFFFFF5
			}

			// Write header value to WASM memory
			ok = mem.Write(bufferPtr, []byte(headerValue))
			if !ok {
				log.Printf("Failed to write header value to WASM memory")
				// Return error code (0xFFFFFFF6)
				return 0xFFFFFFF6
			}

			// Return the size of the header value
			return uint32(len(headerValue))
		}).
		Export("get_last_response_header")

	// Instantiate the host module
	hostModuleInstance, err := hostModule.Instantiate(ctx)
	if err != nil {
		func() {
			if closeErr := runtime.Close(ctx); closeErr != nil {
				log.Printf("Failed to close runtime: %v", closeErr)
			}
		}()
		log.Printf("Failed to instantiate host module: %v", err)
		return nil, fmt.Errorf("failed to instantiate host module: %w", err)
	}
	// Don't forget to close the host module instance
	defer func() {
		if closeErr := hostModuleInstance.Close(ctx); closeErr != nil {
			log.Printf("Failed to close host module instance: %v", closeErr)
		}
	}()

	// Configure module with captured stdin/stdout/stderr and start function
	// WithStartFunctions("_initialize") is CRITICAL for Go-compiled WASM
	// It ensures the Go runtime is properly initialized before main() runs
	config := wazero.NewModuleConfig().
		WithStdin(stdinBuf).
		WithStdout(&stdoutBuf).
		WithStderr(&stderrBuf).
		WithStartFunctions("_initialize")

	// Compile the module first
	compiledModule, err := runtime.CompileModule(ctx, moduleData)
	if err != nil {
		func() {
			if closeErr := runtime.Close(ctx); closeErr != nil {
				log.Printf("Failed to close runtime: %v", closeErr)
			}
		}()
		return nil, fmt.Errorf("failed to compile WASM module: %w", err)
	}

	log.Printf("WASM module compiled successfully, instantiating...")

	// Instantiate the module WITHOUT auto-starting
	instance, err := runtime.InstantiateModule(ctx, compiledModule, config)
	if err != nil {
		func() {
			if closeErr := runtime.Close(ctx); closeErr != nil {
				log.Printf("Failed to close runtime: %v", closeErr)
			}
		}()
		return nil, fmt.Errorf("failed to instantiate WASM module: %w", err)
	}

	log.Printf("WASM module instantiated successfully")

	// Call _initialize to set up Go runtime
	if initFunc := instance.ExportedFunction("_initialize"); initFunc != nil {
		log.Printf("Calling _initialize...")
		_, err = initFunc.Call(ctx)
		if err != nil {
			func() {
				if closeErr := runtime.Close(ctx); closeErr != nil {
					log.Printf("Failed to close runtime: %v", closeErr)
				}
			}()
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
			func() {
				if closeErr := runtime.Close(ctx); closeErr != nil {
					log.Printf("Failed to close runtime: %v", closeErr)
				}
			}()
			return nil, fmt.Errorf("error calling _start: %w", err)
		}
		log.Printf("_start executed successfully")
	} else {
		func() {
			if closeErr := runtime.Close(ctx); closeErr != nil {
				log.Printf("Failed to close runtime: %v", closeErr)
			}
		}()
		return nil, fmt.Errorf("_start function not found")
	}

	// Close instance
	func() {
		if err := instance.Close(ctx); err != nil {
			log.Printf("Failed to close instance: %v", err)
		}
	}()

	// Note: The main() function should have executed and produced output during _start

	// Close the runtime to ensure all resources are cleaned up
	func() {
		if err := runtime.Close(ctx); err != nil {
			log.Printf("Failed to close runtime: %v", err)
		}
	}()

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

// isURLAllowed checks if a URL is allowed based on the allowlist
func (e *WASMExecutor) isURLAllowed(urlStr string) bool {
	// Parse the URL to validate it
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	// Only allow http and https schemes
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return false
	}

	// Check if the URL matches any allowed prefix
	for _, allowed := range e.urlAllowed {
		if strings.HasPrefix(urlStr, allowed) {
			return true
		}
	}

	return false
}

// readStringFromMemory reads a string from WASM memory
func readStringFromMemory(ctx context.Context, memory api.Memory, ptr uint32, size uint32) (string, error) {
	// Read the bytes from memory
	bytes, ok := memory.Read(ptr, size)
	if !ok {
		return "", fmt.Errorf("failed to read memory at offset %d with size %d", ptr, size)
	}

	// Convert bytes to string
	return string(bytes), nil
}

// writeStringToMemory writes a string to WASM memory and returns the pointer and size
func writeStringToMemory(ctx context.Context, memory api.Memory, str string) (uint32, uint32, error) {
	// Convert string to bytes
	data := []byte(str)
	size := uint32(len(data))

	// Check if we have enough memory, if not, try to grow it
	// For simplicity, we'll allocate at the end of current memory
	// In a production system, you'd want a proper memory allocator

	// Get current memory size
	currentSize := memory.Size()

	// Try to grow memory if needed (add some extra space for safety)
	if currentSize < size {
		// Calculate pages needed (64KB per page)
		pagesNeeded := (size - currentSize + 65535) / 65536
		_, ok := memory.Grow(pagesNeeded)
		if !ok {
			return 0, 0, fmt.Errorf("failed to grow memory")
		}
	}

	// Allocate at the end of current memory
	ptr := currentSize

	// Write the data to memory
	ok := memory.Write(ptr, data)
	if !ok {
		return 0, 0, fmt.Errorf("failed to write data to memory")
	}

	return ptr, size, nil
}
