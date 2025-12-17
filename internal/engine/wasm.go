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
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"

	"github.com/mule-ai/mule/internal/agent"
	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/pkg/job"
)

// isValidBranchName validates that the branch name is safe to use
func isValidBranchName(name string) bool {
	// Check for empty name
	if name == "" {
		return false
	}

	// Check for invalid characters
	invalidChars := []string{"~", "^", ":", "?", "*", "[", "\\\\", ".."}
	for _, char := range invalidChars {
		if strings.Contains(name, char) {
			return false
		}
	}

	// Check for reserved names
	reservedNames := []string{".", "..", "@", "HEAD", "ORIG_HEAD", "FETCH_HEAD", "MERGE_HEAD", "CHERRY_PICK_HEAD"}
	for _, reserved := range reservedNames {
		if strings.HasPrefix(name, reserved) || strings.HasSuffix(name, reserved) {
			return false
		}
	}

	// Check for double dots
	if strings.Contains(name, "..") {
		return false
	}

	// Check for ending slash
	if strings.HasSuffix(name, "/") {
		return false
	}

	// Check for consecutive dots
	if strings.Contains(name, "..") {
		return false
	}

	// Check for control characters
	for _, r := range name {
		if r < 32 || r == 127 {
			return false
		}
	}

	return true
}

// WASMExecutor handles WebAssembly module execution
type WASMExecutor struct {
	db             *sql.DB
	store          primitive.PrimitiveStore
	agentRuntime   *agent.Runtime
	WorkflowEngine *Engine
	modules        map[string][]byte // Store compiled module bytes instead of instantiated modules
	urlAllowed     []string          // List of allowed URL prefixes for HTTP requests
	workingDir     string            // Current working directory for this execution context
	// Store the last response for each module instance
	lastResponse     map[string]*http.Response
	lastResponseBody map[string][]byte
	// Store the last workflow/agent execution result for each module instance
	lastOperationResult map[string][]byte
	lastOperationStatus map[string]int
	// Track new working directory set by modules
	newWorkingDir map[string]string
	// Temporary storage for new working directory from current execution
	currentNewWorkingDir string
}

// Modules returns the internal modules map for testing purposes
func (e *WASMExecutor) Modules() map[string][]byte {
	return e.modules
}

// NewWASMExecutor creates a new WASM executor
func NewWASMExecutor(db *sql.DB, store primitive.PrimitiveStore, agentRuntime *agent.Runtime, workflowEngine *Engine) *WASMExecutor {
	return &WASMExecutor{
		db:                   db,
		store:                store,
		agentRuntime:         agentRuntime,
		WorkflowEngine:       workflowEngine,
		modules:              make(map[string][]byte),
		urlAllowed:           []string{"https://", "http://"}, // Allow all URLs by default (can be configured)
		lastResponse:         make(map[string]*http.Response),
		lastResponseBody:     make(map[string][]byte),
		lastOperationResult:  make(map[string][]byte),
		lastOperationStatus:  make(map[string]int),
		newWorkingDir:        make(map[string]string),
		currentNewWorkingDir: "",
	}
}

// get_current_branch_impl is the actual implementation of the get_current_branch host function
// It's separated to allow for better error handling and to ensure we always return our custom error codes
func (e *WASMExecutor) get_current_branch_impl(ctx context.Context, module api.Module, basePathPtr, basePathSize, bufferPtr, bufferSize uint32) uint32 {
	// Get memory from the module
	mem := module.Memory()

	// Read base path from WASM memory (optional, can be empty)
	var basePath string
	if basePathSize > 0 {
		var err error
		basePath, err = readStringFromMemory(ctx, mem, basePathPtr, basePathSize)
		if err != nil {
			log.Printf("Failed to read base path from WASM memory: %v", err)
			// Return error code (0xFFFFFFE1) - Failed to read base path
			return 0xFFFFFFE1
		}
	}

	// If no base path provided, use current working directory
	if basePath == "" {
		basePath = e.workingDir
		if basePath == "" {
			cwd, err := os.Getwd()
			if err != nil {
				log.Printf("Failed to get current working directory: %v", err)
				// Return error code (0xFFFFFFE2) - Failed to get current working directory
				return 0xFFFFFFE2
			}
			basePath = cwd
		}
	}

	// Log the working directory being used
	log.Printf("Getting current branch for repository at: %s", basePath)

	// Validate that base path is a git repository
	gitPath := filepath.Join(basePath, ".git")
	if _, err := os.Stat(gitPath); os.IsNotExist(err) {
		log.Printf("Base path is not a git repository: %s", basePath)
		// Return error code (0xFFFFFFE3) - Path is not a git repository
		return 0xFFFFFFE3
	}

	// Validate that base path is accessible
	if _, err := os.Stat(basePath); err != nil {
		log.Printf("Base path is not accessible: %s, error: %v", basePath, err)
		// Return error code (0xFFFFFFE7) - Path is not accessible
		return 0xFFFFFFE7
	}

	// Log additional information about the repository
	log.Printf("Repository validation passed for: %s", basePath)

	// Check if we can read the .git directory
	if gitInfo, err := os.Stat(gitPath); err != nil {
		log.Printf("Cannot stat .git directory: %v", err)
		// Return error code (0xFFFFFFE7) - Path is not accessible
		return 0xFFFFFFE7
	} else {
		log.Printf(".git directory info: mode=%v, modTime=%v", gitInfo.Mode(), gitInfo.ModTime())
	}

	// Try to run a simple git command to check if git is working
	versionCmd := exec.CommandContext(ctx, "git", "version")
	if versionOutput, err := versionCmd.Output(); err != nil {
		log.Printf("Failed to run git version command: %v", err)
	} else {
		log.Printf("Git version: %s", strings.TrimSpace(string(versionOutput)))
	}

	// Get current branch name using git command with enhanced error handling
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Dir = basePath

	// Log the command being executed
	log.Printf("Executing git command: %s in directory: %s",
		strings.Join(cmd.Args, " "), cmd.Dir)

	// Log environment information
	log.Printf("Environment variables: HOME=%s, USER=%s",
		os.Getenv("HOME"), os.Getenv("USER"))

	output, err := cmd.Output()
	if err != nil {
		// Try to get more detailed error information
		var stderr []byte
		var exitCode = -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			stderr = exitErr.Stderr
			exitCode = exitErr.ExitCode()
		}

		log.Printf("Failed to get current branch name with 'git rev-parse --abbrev-ref HEAD': %v, exit code: %d, stderr: %s, working dir: %s",
			err, exitCode, string(stderr), basePath)

		// Try fallback method to get branch information
		fallbackCmd := exec.CommandContext(ctx, "git", "symbolic-ref", "--short", "HEAD")
		fallbackCmd.Dir = basePath

		log.Printf("Trying fallback command: %s in directory: %s",
			strings.Join(fallbackCmd.Args, " "), fallbackCmd.Dir)

		// Log environment for fallback command
		log.Printf("Fallback command environment: HOME=%s, USER=%s",
			os.Getenv("HOME"), os.Getenv("USER"))

		fallbackOutput, fallbackErr := fallbackCmd.Output()
		if fallbackErr != nil {
			var fallbackStderr []byte
			var fallbackExitCode = -1
			if exitErr, ok := fallbackErr.(*exec.ExitError); ok {
				fallbackStderr = exitErr.Stderr
				fallbackExitCode = exitErr.ExitCode()
			}

			log.Printf("Fallback command also failed: %v, exit code: %d, stderr: %s", fallbackErr, fallbackExitCode, string(fallbackStderr))

			// Check if we're in a detached HEAD state
			detachedCheckCmd := exec.CommandContext(ctx, "git", "rev-parse", "--short", "HEAD")
			detachedCheckCmd.Dir = basePath

			log.Printf("Checking for detached HEAD with command: %s in directory: %s",
				strings.Join(detachedCheckCmd.Args, " "), detachedCheckCmd.Dir)

			if detachedOutput, detachedErr := detachedCheckCmd.Output(); detachedErr == nil {
				commitHash := strings.TrimSpace(string(detachedOutput))
				log.Printf("Repository appears to be in detached HEAD state at commit: %s", commitHash)

				// For detached HEAD, we'll return a special indicator
				detachedIndicator := fmt.Sprintf("DETACHED_HEAD_%s", commitHash)
				branchNameLen := uint32(len(detachedIndicator))

				// Handle the two-call pattern expected by WASM module:
				// 1. First call with buffer size 0: return required size for success, 0 for error
				// 2. Second call with actual buffer: return 0 for success, error code for error
				if bufferSize == 0 {
					return branchNameLen // Return required size for success
				}

				// This is the second call with actual buffer
				// Check if buffer is large enough
				if bufferSize < branchNameLen {
					log.Printf("Buffer too small for detached HEAD indicator: %d < %d", bufferSize, branchNameLen)
					// Return error code (0xFFFFFFE5) - Buffer too small
					return 0xFFFFFFE5
				}

				// Write detached HEAD indicator to WASM memory
				if branchNameLen > 0 {
					ok := mem.Write(bufferPtr, []byte(detachedIndicator))
					if !ok {
						log.Printf("Failed to write detached HEAD indicator to WASM memory")
						// Return error code (0xFFFFFFE6) - Failed to write to WASM memory
						return 0xFFFFFFE6
					}
				}

				// Return 0 for success (this is what the WASM module expects)
				return 0
			}

			// Log detailed error information before returning error code
			log.Printf("All branch detection methods failed for repository at: %s", basePath)
			// Return our custom error code if all methods fail
			// Return error code (0xFFFFFFE4)
			return 0xFFFFFFE4
		}

		// Use fallback output if fallback command succeeded
		output = fallbackOutput
	}

	// Trim whitespace from output
	branchName := strings.TrimSpace(string(output))
	branchNameLen := uint32(len(branchName))

	// Validate branch name
	if branchName == "" {
		log.Printf("Empty branch name detected in repository: %s", basePath)
		// Return error code (0xFFFFFFE8) - Empty branch name
		return 0xFFFFFFE8
	}

	log.Printf("Current branch detected: %s", branchName)

	// Handle the two-call pattern expected by WASM module:
	// 1. First call with buffer size 0: return required size for success, 0 for error
	// 2. Second call with actual buffer: return 0 for success, error code for error
	if bufferSize == 0 {
		return branchNameLen // Return required size for success
	}

	// This is the second call with actual buffer
	// Check if buffer is large enough
	if bufferSize < branchNameLen {
		log.Printf("Buffer too small for branch name: %d < %d", bufferSize, branchNameLen)
		// Return error code (0xFFFFFFE5) - Buffer too small
		return 0xFFFFFFE5
	}

	// Write branch name to WASM memory
	if branchNameLen > 0 {
		ok := mem.Write(bufferPtr, []byte(branchName))
		if !ok {
			log.Printf("Failed to write branch name to WASM memory")
			// Return error code (0xFFFFFFE6) - Failed to write to WASM memory
			return 0xFFFFFFE6
		}
	}

	// Return 0 for success (this is what the WASM module expects)
	return 0
}

// SetURLAllowList sets the list of allowed URL prefixes for HTTP requests
func (e *WASMExecutor) SetURLAllowList(allowed []string) {
	e.urlAllowed = allowed
}

// Execute executes a WASM module with the given input data and working directory
func (e *WASMExecutor) Execute(ctx context.Context, moduleID string, inputData map[string]interface{}, workingDir string) (map[string]interface{}, error) {
	// Store the working directory for use by triggerWorkflow
	e.workingDir = workingDir

	// Get module data from cache or load it
	moduleData, err := e.getModuleData(ctx, moduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get WASM module data: %w", err)
	}

	// Get module configuration from primitive store
	module, err := e.store.GetWasmModule(ctx, moduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to get WASM module: %w", err)
	}

	// Merge configuration with input data
	mergedInputData := make(map[string]interface{})

	// Add configuration data if present
	if len(module.Config) > 0 {
		// Add all config fields to merged input
		for k, v := range module.Config {
			mergedInputData[k] = v
		}
	}

	// Add input data fields (these override config if there are conflicts)
	for k, v := range inputData {
		mergedInputData[k] = v
	}

	log.Printf("Executing WASM module %s (size: %d bytes) with merged input data: %+v", moduleID, len(moduleData), mergedInputData)

	// Add panic recovery for WASI-related issues
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered from WASM execution panic: %v", r)
			// Log stack trace for debugging
			log.Printf("Stack trace: %s", debug.Stack())
		}
	}()

	// Serialize merged input data to JSON for passing to WASM module via stdin
	var stdinData []byte
	if len(mergedInputData) > 0 {
		stdinData, err = json.Marshal(mergedInputData)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize input data: %w", err)
		}
		log.Printf("Passing %d bytes of input data to WASM module via stdin: %s", len(stdinData), string(stdinData))
	} else {
		log.Printf("No input data provided to WASM module (mergedInputData: %+v)", mergedInputData)
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
			// Check for context cancellation before processing
			select {
			case <-ctx.Done():
				// Return error code for cancellation
				return 0xFFFFFFFA
			default:
			}

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
			// Check for context cancellation before processing
			select {
			case <-ctx.Done():
				// Return error code for cancellation
				return 0xFFFFFFFA
			default:
			}

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
	// Add host function for triggering workflows or calling agents
	// This function can handle both workflows and agents based on the target type
	hostModule.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, targetTypePtr, targetTypeSize, targetIDPtr, targetIDSize, paramsPtr, paramsSize uint32) uint32 {
			// Check for context cancellation before processing
			select {
			case <-ctx.Done():
				// Return error code for cancellation
				return 0xFFFFFFFA
			default:
			}

			// Get memory from the module
			mem := module.Memory()

			// Read target type from WASM memory
			targetType, err := readStringFromMemory(ctx, mem, targetTypePtr, targetTypeSize)
			if err != nil {
				log.Printf("Failed to read target type from WASM memory: %v", err)
				// Return error code (0xFFFFFFF0)
				return 0xFFFFFFF0
			}

			// Read target ID from WASM memory
			targetID, err := readStringFromMemory(ctx, mem, targetIDPtr, targetIDSize)
			if err != nil {
				log.Printf("Failed to read target ID from WASM memory: %v", err)
				// Return error code (0xFFFFFFF1)
				return 0xFFFFFFF1
			}

			// Read params from WASM memory
			paramsJSON, err := readStringFromMemory(ctx, mem, paramsPtr, paramsSize)
			if err != nil {
				log.Printf("Failed to read params from WASM memory: %v", err)
				// Return error code (0xFFFFFFF2)
				return 0xFFFFFFF2
			}

			// Parse params JSON
			var params map[string]interface{}
			if paramsJSON != "" {
				if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
					log.Printf("Failed to parse params JSON: %v", err)
					// Return error code (0xFFFFFFF3)
					return 0xFFFFFFF3
				}
			} else {
				params = make(map[string]interface{})
			}

			// Execute based on target type
			var result []byte
			switch strings.ToLower(targetType) {
			case "workflow":
				result, err = e.triggerWorkflow(ctx, targetID, params)
			case "agent":
				result, err = e.callAgent(ctx, targetID, params)
			default:
				log.Printf("Invalid target type: %s", targetType)
				// Return error code (0xFFFFFFF4)
				return 0xFFFFFFF4
			}

			if err != nil {
				log.Printf("Failed to execute %s %s: %v", targetType, targetID, err)
				// Return error code (0xFFFFFFF5)
				return 0xFFFFFFF5
			}

			// Store result for retrieval by the module
			// Use a unique key for this execution context
			key := fmt.Sprintf("%p", module)
			e.lastOperationResult[key] = result
			e.lastOperationStatus[key] = 0 // Success

			// Return 0 for success
			return 0
		}).
		Export("execute_target").
		// Add host function for retrieving the last operation result
		// Function to get the last operation result
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, bufferPtr uint32, bufferSize uint32) uint32 {
			// Get memory from the module
			mem := module.Memory()

			// Get the operation result for this module instance
			key := fmt.Sprintf("%p", module)
			result, ok := e.lastOperationResult[key]
			if !ok {
				log.Printf("No operation result available for module %s", key)
				// Return error code (0xFFFFFFF4)
				return 0xFFFFFFF4
			}

			// If buffer size is 0, return the required size without writing data
			if bufferSize == 0 {
				return uint32(len(result))
			}

			// Check if buffer is large enough
			if bufferSize < uint32(len(result)) {
				log.Printf("Buffer too small for operation result: %d < %d", bufferSize, len(result))
				// Return error code (0xFFFFFFF5)
				return 0xFFFFFFF5
			}

			// Write result to WASM memory
			ok = mem.Write(bufferPtr, result)
			if !ok {
				log.Printf("Failed to write operation result to WASM memory")
				// Return error code (0xFFFFFFF6)
				return 0xFFFFFFF6
			}

			// Return the size of the result
			return uint32(len(result))
		}).
		Export("get_last_operation_result").
		// Add host function for retrieving the last operation status
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module) uint32 {
			// Get the operation status for this module instance
			key := fmt.Sprintf("%p", module)
			status, ok := e.lastOperationStatus[key]
			if !ok {
				log.Printf("No operation status available for module %s", key)
				// Return 0 to indicate no operation has been performed
				return 0
			}

			// Return the status code
			return uint32(status)
		}).
		Export("get_last_operation_status")

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

			// If buffer size is 0, return the required size without writing data
			if bufferSize == 0 {
				return uint32(len(respBody))
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

	// Function to create a git branch
	hostModule.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, branchNamePtr, branchNameSize, basePathPtr, basePathSize uint32) uint32 {
			// Check for context cancellation before processing
			select {
			case <-ctx.Done():
				// Return error code for cancellation
				return 0xFFFFFFFA
			default:
			}

			// Get memory from the module
			mem := module.Memory()

			// Read branch name from WASM memory
			branchName, err := readStringFromMemory(ctx, mem, branchNamePtr, branchNameSize)
			if err != nil {
				log.Printf("Failed to read branch name from WASM memory: %v", err)
				// Return error code (0xFFFFFFF0)
				return 0xFFFFFFF0
			}

			// Read base path from WASM memory (optional, can be empty)
			var basePath string
			if basePathSize > 0 {
				basePath, err = readStringFromMemory(ctx, mem, basePathPtr, basePathSize)
				if err != nil {
					log.Printf("Failed to read base path from WASM memory: %v", err)
					// Return error code (0xFFFFFFF1)
					return 0xFFFFFFF1
				}
			}

			// If no base path provided, use current working directory
			if basePath == "" {
				basePath = e.workingDir
				if basePath == "" {
					cwd, err := os.Getwd()
					if err != nil {
						log.Printf("Failed to get current working directory: %v", err)
						// Return error code (0xFFFFFFF2)
						return 0xFFFFFFF2
					}
					basePath = cwd
				}
			}

			// Validate that base path is a git repository
			gitPath := filepath.Join(basePath, ".git")
			if _, err := os.Stat(gitPath); os.IsNotExist(err) {
				log.Printf("Base path is not a git repository: %s", basePath)
				// Return error code (0xFFFFFFF3)
				return 0xFFFFFFF3
			}

			// Create branch using git command
			// We'll use the git branch command to create a new branch
			cmd := exec.CommandContext(ctx, "git", "checkout", "-b", branchName)
			cmd.Dir = basePath

			output, err := cmd.CombinedOutput()
			if err != nil {
				log.Printf("Failed to create git branch: %v, output: %s", err, string(output))
				// Return error code (0xFFFFFFF4)
				return 0xFFFFFFF4
			}

			log.Printf("Created git branch '%s' in repository: %s", branchName, basePath)
			// Return 0 for success
			return 0
		}).
		Export("create_git_branch")

	// Function to push a git branch to remote
	hostModule.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, branchNamePtr, branchNameSize, remoteNamePtr, remoteNameSize, tokenPtr, tokenSize, basePathPtr, basePathSize uint32) uint32 {
			// Check for context cancellation before processing
			select {
			case <-ctx.Done():
				// Return error code for cancellation
				return 0xFFFFFFFA
			default:
			}

			// Get memory from the module
			mem := module.Memory()

			// Read branch name from WASM memory
			branchName, err := readStringFromMemory(ctx, mem, branchNamePtr, branchNameSize)
			if err != nil {
				log.Printf("Failed to read branch name from WASM memory: %v", err)
				// Return error code (0xFFFFFFF0)
				return 0xFFFFFFF0
			}

			// Read remote name from WASM memory
			remoteName, err := readStringFromMemory(ctx, mem, remoteNamePtr, remoteNameSize)
			if err != nil {
				log.Printf("Failed to read remote name from WASM memory: %v", err)
				// Return error code (0xFFFFFFF7)
				return 0xFFFFFFF7
			}

			// Read token from WASM memory (optional, can be empty)
			var token string
			if tokenSize > 0 {
				token, err = readStringFromMemory(ctx, mem, tokenPtr, tokenSize)
				if err != nil {
					log.Printf("Failed to read token from WASM memory: %v", err)
					// Return error code (0xFFFFFFF8)
					return 0xFFFFFFF8
				}
			}

			// Read base path from WASM memory (optional, can be empty)
			var basePath string
			if basePathSize > 0 {
				basePath, err = readStringFromMemory(ctx, mem, basePathPtr, basePathSize)
				if err != nil {
					log.Printf("Failed to read base path from WASM memory: %v", err)
					// Return error code (0xFFFFFFF1)
					return 0xFFFFFFF1
				}
			}

			// If no base path provided, use current working directory
			if basePath == "" {
				basePath = e.workingDir
				if basePath == "" {
					cwd, err := os.Getwd()
					if err != nil {
						log.Printf("Failed to get current working directory: %v", err)
						// Return error code (0xFFFFFFF2)
						return 0xFFFFFFF2
					}
					basePath = cwd
				}
			}

			// Validate that base path is a git repository
			gitPath := filepath.Join(basePath, ".git")
			if _, err := os.Stat(gitPath); os.IsNotExist(err) {
				log.Printf("Base path is not a git repository: %s", basePath)
				// Return error code (0xFFFFFFF3)
				return 0xFFFFFFF3
			}

			// Set up environment variables for authentication if token is provided
			env := os.Environ()
			if token != "" {
				// Add token to environment for git operations
				env = append(env, fmt.Sprintf("GIT_ASKPASS=echo %s", token))
				env = append(env, "GIT_TERMINAL_PROMPT=0")
			}

			// Push branch to remote using git command
			// We'll use the git push command to push the branch
			cmd := exec.CommandContext(ctx, "git", "push", "-u", remoteName, branchName)
			cmd.Dir = basePath
			cmd.Env = env

			output, err := cmd.CombinedOutput()
			if err != nil {
				log.Printf("Failed to push git branch: %v, output: %s", err, string(output))
				// Return error code (0xFFFFFFF9)
				return 0xFFFFFFF9
			}

			log.Printf("Pushed git branch '%s' to remote '%s' in repository: %s", branchName, remoteName, basePath)
			// Return 0 for success
			return 0
		}).
		Export("push_git_branch")

	// Function to create a git worktree
	hostModule.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, namePtr, nameSize, basePathPtr, basePathSize uint32) uint32 {
			// Check for context cancellation before processing
			select {
			case <-ctx.Done():
				// Return error code for cancellation
				return 0xFFFFFFFA
			default:
			}

			// Get memory from the module
			mem := module.Memory()

			// Read worktree name from WASM memory
			name, err := readStringFromMemory(ctx, mem, namePtr, nameSize)
			if err != nil {
				log.Printf("Failed to read worktree name from WASM memory: %v", err)
				// Return error code (0xFFFFFFF0)
				return 0xFFFFFFF0
			}

			// Read base path from WASM memory (optional, can be empty)
			var basePath string
			if basePathSize > 0 {
				basePath, err = readStringFromMemory(ctx, mem, basePathPtr, basePathSize)
				if err != nil {
					log.Printf("Failed to read base path from WASM memory: %v", err)
					// Return error code (0xFFFFFFF1)
					return 0xFFFFFFF1
				}
			}

			// If no base path provided, use current working directory
			if basePath == "" {
				basePath = e.workingDir
				if basePath == "" {
					cwd, err := os.Getwd()
					if err != nil {
						log.Printf("Failed to get current working directory: %v", err)
						// Return error code (0xFFFFFFF2)
						return 0xFFFFFFF2
					}
					basePath = cwd
				}
			}

			// Validate that base path is a git repository
			gitPath := filepath.Join(basePath, ".git")
			if _, err := os.Stat(gitPath); os.IsNotExist(err) {
				log.Printf("Base path is not a git repository: %s", basePath)
				// Return error code (0xFFFFFFF3)
				return 0xFFFFFFF3
			}

			// Determine worktree path - this should be a sibling directory to the main repo
			// or in a location specified by the user
			worktreePath := filepath.Join(basePath, "..", name)

			// If the above would put it inside the repo, put it as a sibling
			if strings.HasPrefix(worktreePath, basePath) {
				worktreePath = filepath.Join(basePath, "..", name)
			}

			// Check if worktree already exists
			if _, err := os.Stat(worktreePath); err == nil {
				// Worktree already exists, use it
				log.Printf("Git worktree '%s' already exists at: %s", name, worktreePath)

				// Store the worktree path in the module's last operation result
				// This allows the workflow engine to retrieve it after execution
				key := fmt.Sprintf("%p", module)
				e.lastOperationResult[key] = []byte(worktreePath)
				e.lastOperationStatus[key] = 0      // Success
				e.newWorkingDir[key] = worktreePath // Store new working directory

				// Also store in currentNewWorkingDir for this execution
				e.currentNewWorkingDir = worktreePath

				// Return 0 for success
				return 0
			}

			// Create worktree using git command
			// We'll use the git worktree add command to create a proper worktree
			cmd := exec.CommandContext(ctx, "git", "worktree", "add", worktreePath, "HEAD")
			cmd.Dir = basePath

			output, err := cmd.CombinedOutput()
			if err != nil {
				log.Printf("Failed to create git worktree: %v, output: %s", err, string(output))
				// Return error code (0xFFFFFFF4)
				return 0xFFFFFFF4
			}

			// Store the worktree path in the module's last operation result
			// This allows the workflow engine to retrieve it after execution
			key := fmt.Sprintf("%p", module)
			e.lastOperationResult[key] = []byte(worktreePath)
			e.lastOperationStatus[key] = 0      // Success
			e.newWorkingDir[key] = worktreePath // Store new working directory

			// Also store in currentNewWorkingDir for this execution
			e.currentNewWorkingDir = worktreePath

			log.Printf("Created git worktree '%s' at: %s", name, worktreePath)
			// Return 0 for success
			return 0
		}).
		Export("create_git_worktree")

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

			// If buffer size is 0, return the required size without writing data
			if bufferSize == 0 {
				return uint32(len(headerValue))
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
		Export("get_last_response_header").
		// Function to get job output by job ID
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, jobIDPtr, jobIDSize, bufferPtr, bufferSize uint32) uint32 {
			// Check for context cancellation before processing
			select {
			case <-ctx.Done():
				// Return error code for cancellation
				return 0xFFFFFFFA
			default:
			}

			// Get memory from the module
			mem := module.Memory()

			// Read job ID from WASM memory
			jobID, err := readStringFromMemory(ctx, mem, jobIDPtr, jobIDSize)
			if err != nil {
				log.Printf("Failed to read job ID from WASM memory: %v", err)
				// Return error code (0xFFFFFFF0)
				return 0xFFFFFFF0
			}

			// Get job from database
			job, err := e.WorkflowEngine.jobStore.GetJob(jobID)
			if err != nil {
				log.Printf("Failed to get job %s: %v", jobID, err)
				// Return error code (0xFFFFFFF1)
				return 0xFFFFFFF1
			}

			// Create a response that includes both status and output data
			response := map[string]interface{}{
				"status": string(job.Status),
				"output": job.OutputData,
			}

			// Marshal response to JSON
			responseData, err := json.Marshal(response)
			if err != nil {
				log.Printf("Failed to marshal job response data for job %s: %v", jobID, err)
				// Return error code (0xFFFFFFF2)
				return 0xFFFFFFF2
			}

			// If buffer size is 0, return the required size without writing data
			if bufferSize == 0 {
				return uint32(len(responseData))
			}

			// Check if buffer is large enough
			if bufferSize < uint32(len(responseData)) {
				log.Printf("Buffer too small for job response data: %d < %d", bufferSize, len(responseData))
				// Return error code (0xFFFFFFF3)
				return 0xFFFFFFF3
			}

			// Write job response data to WASM memory
			ok := mem.Write(bufferPtr, responseData)
			if !ok {
				log.Printf("Failed to write job response data to WASM memory")
				// Return error code (0xFFFFFFF4)
				return 0xFFFFFFF4
			}

			// Return the size of the job response data
			return uint32(len(responseData))
		}).
		Export("get_job_output").
		// Function to wait for job completion and get the result
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, jobIDPtr, jobIDSize, bufferPtr, bufferSize uint32) uint32 {
			// Check for context cancellation before processing
			select {
			case <-ctx.Done():
				// Return error code for cancellation
				return 0xFFFFFFFA
			default:
			}

			// Get memory from the module
			mem := module.Memory()

			// Read job ID from WASM memory
			jobID, err := readStringFromMemory(ctx, mem, jobIDPtr, jobIDSize)
			if err != nil {
				log.Printf("Failed to read job ID from WASM memory: %v", err)
				// Return error code (0xFFFFFFF0)
				return 0xFFFFFFF0
			}

			// Wait for job completion with a 5-minute timeout
			timeoutCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			defer cancel()

			// Poll every 500ms for job completion
			ticker := time.NewTicker(500 * time.Millisecond)
			defer ticker.Stop()

			var jobItem *job.Job
			completed := false

			for !completed {
				select {
				case <-timeoutCtx.Done():
					log.Printf("Timeout waiting for job %s to complete", jobID)
					// Return error code (0xFFFFFFF5) for timeout
					return 0xFFFFFFF5
				case <-ticker.C:
					// Get job from database
					jobItem, err = e.WorkflowEngine.jobStore.GetJob(jobID)
					if err != nil {
						log.Printf("Failed to get job %s: %v", jobID, err)
						// Continue polling despite errors
						continue
					}

					// Check job status
					switch jobItem.Status {
					case job.StatusCompleted:
						completed = true
					case job.StatusFailed:
						log.Printf("Job %s failed", jobID)
						// Return error code (0xFFFFFFF6) for job failure
						return 0xFFFFFFF6
					case job.StatusCancelled:
						log.Printf("Job %s was cancelled", jobID)
						// Return error code (0xFFFFFFF7) for job cancellation
						return 0xFFFFFFF7
					case job.StatusRunning, job.StatusQueued:
						// Continue waiting
						continue
					default:
						log.Printf("Unknown job status for job %s: %s", jobID, jobItem.Status)
						// Continue waiting
						continue
					}
				}
			}

			// Job completed successfully, create response with output data
			response := map[string]interface{}{
				"status": string(jobItem.Status),
				"output": jobItem.OutputData,
			}

			// Marshal response to JSON
			responseData, err := json.Marshal(response)
			if err != nil {
				log.Printf("Failed to marshal job response data for job %s: %v", jobID, err)
				// Return error code (0xFFFFFFF2)
				return 0xFFFFFFF2
			}

			// If buffer size is 0, return the required size without writing data
			if bufferSize == 0 {
				return uint32(len(responseData))
			}

			// Check if buffer is large enough
			if bufferSize < uint32(len(responseData)) {
				log.Printf("Buffer too small for job response data: %d < %d", bufferSize, len(responseData))
				// Return error code (0xFFFFFFF3)
				return 0xFFFFFFF3
			}

			// Write job response data to WASM memory
			ok := mem.Write(bufferPtr, responseData)
			if !ok {
				log.Printf("Failed to write job response data to WASM memory")
				// Return error code (0xFFFFFFF4)
				return 0xFFFFFFF4
			}

			// Return the size of the job response data
			return uint32(len(responseData))
		}).
		Export("wait_for_job_and_get_output")

	// Function to trigger workflows or call agents
	hostModule.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, operationTypePtr, operationTypeSize, idPtr, idSize, paramsPtr, paramsSize uint32) uint32 {
			// Check for context cancellation before processing
			select {
			case <-ctx.Done():
				// Return error code for cancellation
				return 0xFFFFFFFA
			default:
			}

			// Get memory from the module
			mem := module.Memory()

			// Read operation type from WASM memory
			operationType, err := readStringFromMemory(ctx, mem, operationTypePtr, operationTypeSize)
			if err != nil {
				log.Printf("Failed to read operation type from WASM memory: %v", err)
				// Return error code (0xFFFFFFF0)
				return 0xFFFFFFF0
			}

			// Read ID from WASM memory
			id, err := readStringFromMemory(ctx, mem, idPtr, idSize)
			if err != nil {
				log.Printf("Failed to read ID from WASM memory: %v", err)
				// Return error code (0xFFFFFFF1)
				return 0xFFFFFFF1
			}

			// Read parameters from WASM memory
			paramsStr, err := readStringFromMemory(ctx, mem, paramsPtr, paramsSize)
			if err != nil {
				log.Printf("Failed to read parameters from WASM memory: %v", err)
				// Return error code (0xFFFFFFF2)
				return 0xFFFFFFF2
			}

			// Parse parameters JSON
			var params map[string]interface{}
			if err := json.Unmarshal([]byte(paramsStr), &params); err != nil {
				log.Printf("Failed to parse parameters JSON: %v", err)
				// Return error code (0xFFFFFFF4)
				return 0xFFFFFFF4
			}

			// Generate a unique key for this module instance
			key := fmt.Sprintf("%p", module)

			// Handle based on operation type
			switch operationType {
			case "workflow":
				// Trigger workflow
				result, err := e.triggerWorkflow(ctx, id, params)
				if err != nil {
					log.Printf("Failed to trigger workflow %s: %v", id, err)
					e.lastOperationStatus[key] = 0xFFFFFFFC // Internal error
					return 0xFFFFFFFC
				}
				e.lastOperationResult[key] = result
				e.lastOperationStatus[key] = 200
				return 0

			case "agent":
				// Call agent
				result, err := e.callAgent(ctx, id, params)
				if err != nil {
					log.Printf("Failed to call agent %s: %v", id, err)
					e.lastOperationStatus[key] = 0xFFFFFFFC // Internal error
					return 0xFFFFFFFC
				}
				e.lastOperationResult[key] = result
				e.lastOperationStatus[key] = 200
				return 0

			default:
				log.Printf("Invalid operation type: %s", operationType)
				// Return error code (0xFFFFFFF3)
				return 0xFFFFFFF3
			}
		}).
		Export("trigger_workflow_or_agent")

	// Function to get the last operation result
	hostModule.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, bufferPtr uint32, bufferSize uint32) uint32 {
			// Get memory from the module
			mem := module.Memory()

			// Get the operation result for this module instance
			key := fmt.Sprintf("%p", module)
			result, ok := e.lastOperationResult[key]
			if !ok {
				log.Printf("No operation result available for module %s", key)
				// Return error code (0xFFFFFFF4)
				return 0xFFFFFFF4
			}

			// If buffer size is 0, return the required size without writing data
			if bufferSize == 0 {
				return uint32(len(result))
			}

			// Check if buffer is large enough
			if bufferSize < uint32(len(result)) {
				log.Printf("Buffer too small for operation result: %d < %d", bufferSize, len(result))
				// Return error code (0xFFFFFFF5)
				return 0xFFFFFFF5
			}

			// Write result to WASM memory
			ok = mem.Write(bufferPtr, result)
			if !ok {
				log.Printf("Failed to write operation result to WASM memory")
				// Return error code (0xFFFFFFF6)
				return 0xFFFFFFF6
			}

			// Return the size of the result
			return uint32(len(result))
		}).
		Export("get_last_operation_result")

	// Function to get the last operation status
	hostModule.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module) uint32 {
			// Get the operation status for this module instance
			key := fmt.Sprintf("%p", module)
			status, ok := e.lastOperationStatus[key]
			if !ok {
				log.Printf("No operation status available for module %s", key)
				// Return 0 to indicate no operation has been performed
				return 0
			}

			// Return the status code
			return uint32(status)
		}).
		Export("get_last_operation_status")

	// Function to get the current working directory
	hostModule.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, bufferPtr uint32, bufferSize uint32) uint32 {
			// Get memory from the module
			mem := module.Memory()

			// If buffer size is 0, return the required size without writing data
			if bufferSize == 0 {
				return uint32(len(workingDir))
			}

			// Check if buffer is large enough
			if bufferSize < uint32(len(workingDir)) {
				log.Printf("Buffer too small for working directory: %d < %d", bufferSize, len(workingDir))
				// Return error code (0xFFFFFFF5)
				return 0xFFFFFFF5
			}

			// Write working directory to WASM memory
			if len(workingDir) > 0 {
				ok := mem.Write(bufferPtr, []byte(workingDir))
				if !ok {
					log.Printf("Failed to write working directory to WASM memory")
					// Return error code (0xFFFFFFF6)
					return 0xFFFFFFF6
				}
			}

			// Return the size of the working directory
			return uint32(len(workingDir))
		}).
		Export("get_working_directory")

	// Function to get the current git branch name
	hostModule.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, basePathPtr, basePathSize, bufferPtr, bufferSize uint32) uint32 {
			// Check for context cancellation before processing
			select {
			case <-ctx.Done():
				// Return error code for cancellation
				return 0xFFFFFFFA
			default:
			}

			// Call the actual implementation and ensure we always return our custom error codes
			result := e.get_current_branch_impl(ctx, module, basePathPtr, basePathSize, bufferPtr, bufferSize)

			// Check if result is one of our custom error codes or a valid size
			// Our custom error codes are in the range 0xFFFFFFE1 to 0xFFFFFFFA
			// Valid sizes are >= 0 and < 0xFFFFFFE1
			if result >= 0xFFFFFFE1 && result <= 0xFFFFFFFA {
				// This is one of our custom error codes, which is fine
				return result
			}
			if result < 0xFFFFFFE1 {
				// This is a valid size, which is fine
				return result
			}
			// If we get here, we have an unexpected return value
			// Log it and return our generic error code
			log.Printf("WARNING: get_current_branch function returning unexpected value: 0x%x. Converting to our error code.", result)
			return 0xFFFFFFE4
		}).
		Export("get_current_branch")

	// Function to push the current branch (using worktree name as branch name)
	hostModule.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, tokenPtr, tokenSize, basePathPtr, basePathSize, userNamePtr, userNameSize, userEmailPtr, userEmailSize uint32) uint32 {
			// Check for context cancellation before processing
			select {
			case <-ctx.Done():
				// Return error code for cancellation
				return 0xFFFFFFFA
			default:
			}

			// Get memory from the module
			mem := module.Memory()

			// Read token from WASM memory (optional, can be empty)
			var token string
			if tokenSize > 0 {
				var err error
				token, err = readStringFromMemory(ctx, mem, tokenPtr, tokenSize)
				if err != nil {
					log.Printf("Failed to read token from WASM memory: %v", err)
					// Return error code (0xFFFFFFF0)
					return 0xFFFFFFF0
				}
			}

			// Read base path from WASM memory (optional, can be empty)
			var basePath string
			if basePathSize > 0 {
				var err error
				basePath, err = readStringFromMemory(ctx, mem, basePathPtr, basePathSize)
				if err != nil {
					log.Printf("Failed to read base path from WASM memory: %v", err)
					// Return error code (0xFFFFFFF1)
					return 0xFFFFFFF1
				}
			}

			// Read user name from WASM memory (optional, can be empty)
			var userName string
			if userNameSize > 0 {
				var err error
				userName, err = readStringFromMemory(ctx, mem, userNamePtr, userNameSize)
				if err != nil {
					log.Printf("Failed to read user name from WASM memory: %v", err)
					// Return error code (0xFFFFFFF1)
					return 0xFFFFFFF1
				}
			}

			// Read user email from WASM memory (optional, can be empty)
			var userEmail string
			if userEmailSize > 0 {
				var err error
				userEmail, err = readStringFromMemory(ctx, mem, userEmailPtr, userEmailSize)
				if err != nil {
					log.Printf("Failed to read user email from WASM memory: %v", err)
					// Return error code (0xFFFFFFF1)
					return 0xFFFFFFF1
				}
			}

			// If no base path provided, use current working directory
			if basePath == "" {
				basePath = e.workingDir
				if basePath == "" {
					cwd, err := os.Getwd()
					if err != nil {
						log.Printf("Failed to get current working directory: %v", err)
						// Return error code (0xFFFFFFF2)
						return 0xFFFFFFF2
					}
					basePath = cwd
				}
			}

			// Validate that base path is a git repository
			gitPath := filepath.Join(basePath, ".git")
			if _, err := os.Stat(gitPath); os.IsNotExist(err) {
				log.Printf("Base path is not a git repository: %s", basePath)
				// Return error code (0xFFFFFFF3)
				return 0xFFFFFFF3
			}

			// Get the current working directory name (worktree name) to use as branch name
			branchName := filepath.Base(basePath)

			// Validate branch name
			if !isValidBranchName(branchName) {
				log.Printf("Invalid branch name derived from worktree: %s", branchName)
				// Return error code (0xFFFFFFF4)
				return 0xFFFFFFF4
			}

			// Set git user config if provided
			if userName != "" || userEmail != "" {
				if userName != "" {
					cmd := exec.CommandContext(ctx, "git", "config", "user.name", userName)
					cmd.Dir = basePath

					output, err := cmd.CombinedOutput()
					if err != nil {
						log.Printf("Failed to set git user name: %v, output: %s", err, string(output))
						// Return error code (0xFFFFFFF9)
						return 0xFFFFFFF9
					}
				}

				if userEmail != "" {
					cmd := exec.CommandContext(ctx, "git", "config", "user.email", userEmail)
					cmd.Dir = basePath

					output, err := cmd.CombinedOutput()
					if err != nil {
						log.Printf("Failed to set git user email: %v, output: %s", err, string(output))
						// Return error code (0xFFFFFFF9)
						return 0xFFFFFFF9
					}
				}
			}

			// Stage all changes
			cmd := exec.CommandContext(ctx, "git", "add", ".")
			cmd.Dir = basePath

			output, err := cmd.CombinedOutput()
			if err != nil {
				log.Printf("Failed to stage changes: %v, output: %s", err, string(output))
				// Return error code (0xFFFFFFF7)
				return 0xFFFFFFF7
			}

			// Commit changes with a default message
			cmd = exec.CommandContext(ctx, "git", "commit", "-m", fmt.Sprintf("Commit changes in worktree %s", branchName))
			cmd.Dir = basePath

			output, err = cmd.CombinedOutput()
			if err != nil {
				// Check if it's because there's nothing to commit
				if strings.Contains(string(output), "nothing to commit") {
					log.Printf("No changes to commit in worktree: %s", branchName)
					// This isn't necessarily an error, we can continue
				} else {
					log.Printf("Failed to commit changes: %v, output: %s", err, string(output))
					// Return error code (0xFFFFFFF8)
					return 0xFFFFFFF8
				}
			}

			// Create branch or switch to existing branch
			// First, try to checkout the branch (in case it already exists)
			cmd = exec.CommandContext(ctx, "git", "checkout", branchName)
			cmd.Dir = basePath

			_, err = cmd.CombinedOutput()
			if err != nil {
				// If checkout fails, try to create the branch
				cmd = exec.CommandContext(ctx, "git", "checkout", "-b", branchName)
				cmd.Dir = basePath

				output, err = cmd.CombinedOutput()
				if err != nil {
					log.Printf("Failed to checkout or create git branch '%s': %v, output: %s", branchName, err, string(output))
					// Return error code (0xFFFFFFF5)
					return 0xFFFFFFF5
				}
				log.Printf("Created new branch '%s'", branchName)
			} else {
				log.Printf("Switched to existing branch '%s'", branchName)
			}

			// Set up environment variables for authentication if token is provided
			env := os.Environ()
			if token != "" {
				// Add token to environment for git operations
				env = append(env, fmt.Sprintf("GIT_ASKPASS=echo %s", token))
				env = append(env, "GIT_TERMINAL_PROMPT=0")
			}

			// Push branch to remote using git command
			cmd = exec.CommandContext(ctx, "git", "push", "-u", "origin", branchName)
			cmd.Dir = basePath
			cmd.Env = env

			output, err = cmd.CombinedOutput()
			if err != nil {
				log.Printf("Failed to push git branch '%s': %v, output: %s", branchName, err, string(output))
				// Return error code (0xFFFFFFF6)
				return 0xFFFFFFF6
			}

			log.Printf("Staged, committed, and pushed git branch '%s' from worktree in repository: %s", branchName, basePath)
			// Return 0 for success
			return 0
		}).
		Export("push_current_branch")

	// Function to set the working directory for subsequent steps
	hostModule.NewFunctionBuilder().
		WithFunc(func(ctx context.Context, module api.Module, pathPtr, pathSize uint32) uint32 {
			// Check for context cancellation before processing
			select {
			case <-ctx.Done():
				// Return error code for cancellation
				return 0xFFFFFFFA
			default:
			}

			// Get memory from the module
			mem := module.Memory()

			// Read path from WASM memory
			path, err := readStringFromMemory(ctx, mem, pathPtr, pathSize)
			if err != nil {
				log.Printf("Failed to read path from WASM memory: %v", err)
				// Return error code (0xFFFFFFF0)
				return 0xFFFFFFF0
			}

			// Validate path - ensure it's an absolute path or relative to current working dir
			var fullPath string
			if filepath.IsAbs(path) {
				fullPath = path
			} else {
				// If workingDir is empty, use current directory as base
				if workingDir == "" {
					cwd, err := os.Getwd()
					if err != nil {
						log.Printf("Failed to get current working directory: %v", err)
						// Return error code (0xFFFFFFF1)
						return 0xFFFFFFF1
					}
					fullPath = filepath.Join(cwd, path)
				} else {
					fullPath = filepath.Join(workingDir, path)
				}
			}

			// Check if the directory exists
			if _, err := os.Stat(fullPath); os.IsNotExist(err) {
				// Try to create the directory
				if mkdirErr := os.MkdirAll(fullPath, 0755); mkdirErr != nil {
					log.Printf("Failed to create directory %s: %v", fullPath, mkdirErr)
					// Return error code (0xFFFFFFF2)
					return 0xFFFFFFF2
				}
				log.Printf("Created directory: %s", fullPath)
			}

			// Store the new working directory in the module's last operation result
			// This allows the workflow engine to retrieve it after execution
			key := fmt.Sprintf("%p", module)
			e.lastOperationResult[key] = []byte(fullPath)
			e.lastOperationStatus[key] = 0  // Success
			e.newWorkingDir[key] = fullPath // Store new working directory

			// Also store in currentNewWorkingDir for this execution
			e.currentNewWorkingDir = fullPath

			log.Printf("Set working directory to: %s", fullPath)
			// Return 0 for success
			return 0
		}).
		Export("set_working_directory")

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

	// Create the base module configuration
	config := wazero.NewModuleConfig().
		WithStdin(stdinBuf).
		WithStdout(&stdoutBuf).
		WithStderr(&stderrBuf).
		WithStartFunctions("_initialize")

	// If a working directory is provided, configure filesystem access
	if workingDir != "" {
		// Check if the directory exists, create it if it doesn't
		if _, err := os.Stat(workingDir); os.IsNotExist(err) {
			if mkdirErr := os.MkdirAll(workingDir, 0755); mkdirErr != nil {
				log.Printf("Warning: failed to create working directory %s: %v", workingDir, mkdirErr)
			} else {
				log.Printf("Created working directory: %s", workingDir)
			}
		}

		// Configure the module with filesystem access
		config = config.WithFS(os.DirFS(workingDir))
		log.Printf("Configured WASM module with filesystem access to directory: %s", workingDir)
	}

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
		// Create a channel to receive the result of the WASM execution
		done := make(chan error, 1)

		// Run the WASM execution in a goroutine so we can monitor for context cancellation
		go func() {
			_, err := startFunc.Call(ctx)
			done <- err
		}()

		// Wait for either the WASM execution to complete or the context to be cancelled
		select {
		case err = <-done:
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
		case <-ctx.Done():
			// Context was cancelled, clean up and return error
			func() {
				if closeErr := runtime.Close(ctx); closeErr != nil {
					log.Printf("Failed to close runtime: %v", closeErr)
				}
			}()
			return nil, fmt.Errorf("WASM execution cancelled: %w", ctx.Err())
		}
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
	success := true // Default to true unless explicitly set to false

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

			// Check for success field in the result
			if successField, ok := resultValue["success"]; ok {
				if successBool, ok := successField.(bool); ok {
					success = successBool
				}
			}
		} else {
			// Not valid JSON, return as string
			output = stdoutStr
		}
	} else {
		output = ""
	}

	// Reset the working directory after execution
	e.workingDir = ""

	// Return the extracted output
	result := map[string]interface{}{
		"output":  output,
		"stdout":  stdoutStr,
		"stderr":  stderrStr,
		"message": "WASM module executed successfully",
		"success": success,
	}

	// Check if a new working directory was set by the WASM module
	if e.currentNewWorkingDir != "" {
		result["new_working_directory"] = e.currentNewWorkingDir
		// Reset for next execution
		e.currentNewWorkingDir = ""
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

	// Load from primitive store
	module, err := e.store.GetWasmModule(ctx, moduleID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch WASM module from store: %w", err)
	}

	// Cache the module data
	e.modules[moduleID] = module.ModuleData

	return module.ModuleData, nil
}

// InvalidateModuleCache removes a specific module from the cache
func (e *WASMExecutor) InvalidateModuleCache(moduleID string) {
	delete(e.modules, moduleID)
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

// triggerWorkflow triggers a workflow execution
func (e *WASMExecutor) triggerWorkflow(ctx context.Context, workflowID string, params map[string]interface{}) ([]byte, error) {
	// Validate that we have a workflow engine
	if e.WorkflowEngine == nil {
		return nil, fmt.Errorf("workflow engine not available")
	}

	// Check for context cancellation before processing
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("workflow trigger cancelled: %w", ctx.Err())
	default:
	}

	// Check if the workflow exists by ID first
	_, err := e.store.GetWorkflow(ctx, workflowID)
	if err != nil {
		if err == primitive.ErrNotFound {
			// Try to find by name
			workflows, listErr := e.store.ListWorkflows(ctx)
			if listErr != nil {
				return nil, fmt.Errorf("failed to list workflows: %w", listErr)
			}

			found := false
			for _, w := range workflows {
				if strings.EqualFold(w.Name, workflowID) {
					workflowID = w.ID
					found = true
					break
				}
			}

			if !found {
				return nil, fmt.Errorf("workflow not found: %s", workflowID)
			}
		} else {
			return nil, fmt.Errorf("failed to get workflow: %w", err)
		}
	}

	// Check for async parameter
	async := false
	if asyncParam, ok := params["async"]; ok {
		if asyncBool, ok := asyncParam.(bool); ok {
			async = asyncBool
		}
	}

	// Check for working_directory parameter
	workingDir := ""
	if wdParam, ok := params["working_directory"]; ok {
		if wdStr, ok := wdParam.(string); ok {
			workingDir = wdStr
		}
	}

	// If no working directory was specified in params, use the executor's working directory
	// This ensures that workflows launched by WASM modules inherit the working directory context
	if workingDir == "" && e.workingDir != "" {
		workingDir = e.workingDir
	}

	// Submit job to workflow engine
	// If a working directory is specified, use SubmitJobWithWorkingDir
	var job *job.Job
	if workingDir != "" {
		job, err = e.WorkflowEngine.SubmitJobWithWorkingDir(ctx, workflowID, params, workingDir)
	} else {
		job, err = e.WorkflowEngine.SubmitJob(ctx, workflowID, params)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to submit workflow job: %w", err)
	}

	// If async, return immediately
	if async {
		result := map[string]interface{}{
			"job_id":  job.ID,
			"status":  string(job.Status),
			"message": "Workflow job submitted successfully",
		}
		return json.Marshal(result)
	}

	// For synchronous execution, we need to wait for completion
	// This is a simplified implementation - in a real system, you'd want to avoid blocking
	// For now, we'll just return the job ID and let the caller check the status
	result := map[string]interface{}{
		"job_id":  job.ID,
		"status":  string(job.Status),
		"message": "Workflow job started",
	}

	return json.Marshal(result)
}

// callAgent calls an agent with the provided parameters
func (e *WASMExecutor) callAgent(ctx context.Context, agentID string, params map[string]interface{}) ([]byte, error) {
	// Check for context cancellation before processing
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("agent call cancelled: %w", ctx.Err())
	default:
	}

	// Check if the agent exists
	agentModel, err := e.store.GetAgent(ctx, agentID)
	if err != nil {
		if err == primitive.ErrNotFound {
			// Try to find by name
			agents, listErr := e.store.ListAgents(ctx)
			if listErr != nil {
				return nil, fmt.Errorf("failed to list agents: %w", listErr)
			}

			found := false
			for _, a := range agents {
				if strings.EqualFold(a.Name, agentID) {
					agentModel = a
					agentID = a.ID
					found = true
					break
				}
			}

			if !found {
				return nil, fmt.Errorf("agent not found: %s", agentID)
			}
		} else {
			return nil, fmt.Errorf("failed to get agent: %w", err)
		}
	}

	// Prepare the chat completion request
	req := &agent.ChatCompletionRequest{
		Model: fmt.Sprintf("agent/%s", agentModel.Name),
	}

	// Handle messages parameter
	if messagesParam, ok := params["messages"]; ok {
		if messages, ok := messagesParam.([]interface{}); ok {
			for _, msg := range messages {
				if msgMap, ok := msg.(map[string]interface{}); ok {
					role, _ := msgMap["role"].(string)
					content, _ := msgMap["content"].(string)
					req.Messages = append(req.Messages, agent.ChatCompletionMessage{
						Role:    role,
						Content: content,
					})
				}
			}
		}
	} else {
		// If no messages, try to use prompt parameter
		if promptParam, ok := params["prompt"]; ok {
			if prompt, ok := promptParam.(string); ok {
				req.Messages = append(req.Messages, agent.ChatCompletionMessage{
					Role:    "user",
					Content: prompt,
				})
			}
		}
	}

	// Handle stream parameter
	if streamParam, ok := params["stream"]; ok {
		if stream, ok := streamParam.(bool); ok {
			req.Stream = stream
		}
	}

	// Execute the agent
	resp, err := e.agentRuntime.ExecuteAgent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute agent: %w", err)
	}

	// Return the response
	return json.Marshal(resp)
}
