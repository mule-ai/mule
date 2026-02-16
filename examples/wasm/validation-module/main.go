//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unsafe"
)

// Configuration represents the validation module configuration
type Configuration struct {
	ValidationCommand    string `json:"validation_command"`
	CorrectiveWorkflowID string `json:"corrective_workflow_id"`
	MaxAttempts          int    `json:"max_attempts"`
	WorkingDirectory     string `json:"working_directory,omitempty"`
}

// ValidationResult represents the result of a validation attempt
type ValidationResult struct {
	Success  bool   `json:"success"`
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	Attempt  int    `json:"attempt"`
	Error    string `json:"error,omitempty"`
}

// ValidationContext represents the context passed to corrective workflows
type ValidationContext struct {
	OriginalPrompt       string           `json:"original_prompt"`
	ValidationOutput     ValidationResult `json:"validation_output"`
	WorkingDirectory     string           `json:"working_directory"`
	RemainingAttempts    int              `json:"remaining_attempts"`
	CorrectiveWorkflowID string           `json:"corrective_workflow_id"`
}

// Host function declarations for WASM imports
//
//go:wasmimport env execute_target
func execute_target(operationTypePtr, operationTypeSize, idPtr, idSize, paramsPtr, paramsSize uint32) uint32

//
//go:wasmimport env wait_for_job_and_get_output
func wait_for_job_and_get_output(jobIDPtr, jobIDSize, bufferPtr, bufferSize uint32) uint32

//
//go:wasmimport env execute_bash_command
func execute_bash_command(commandPtr, commandSize, workingDirPtr, workingDirSize uint32) uint32

//
//go:wasmimport env get_last_operation_result
func get_last_operation_result(bufferPtr, bufferSize uint32) uint32

//
//go:wasmimport env get_last_operation_status
func get_last_operation_status() uint32

//
//go:wasmimport env get_working_directory
func get_working_directory(bufferPtr, bufferSize uint32) uint32

// Helper function to allocate memory and copy string to it
func stringToPtr(s string) (uint32, uint32, []byte) {
	if s == "" {
		return 0, 0, nil
	}
	// Create a byte slice and return it to prevent garbage collection
	bytes := []byte(s)
	ptr := uint32(uintptr(unsafe.Pointer(&bytes[0])))
	return ptr, uint32(len(bytes)), bytes
}

// Helper function to read string from memory
func readStringFromMemory(ptr uint32, size uint32) string {
	if size == 0 {
		return ""
	}
	// Create a byte slice from the pointer and size
	// Note: This is a simplified approach for demonstration
	// In a real WASM module, you'd use the actual WASM memory
	return string(make([]byte, size))
}

// executeCommand executes a command using the host function and returns the result
func executeCommand(command, workingDir string) ValidationResult {
	result := ValidationResult{
		Command: command,
	}

	// Validate inputs
	if command == "" {
		result.Error = "validation command is required"
		return result
	}

	// Convert strings to pointers for host function call
	commandPtr, commandSize, commandBytes := stringToPtr(command)
	workingDirPtr, workingDirSize, workingDirBytes := stringToPtr(workingDir)

	// Keep references to prevent garbage collection
	_ = commandBytes
	_ = workingDirBytes

	// Call the execute_bash_command host function
	errorCode := execute_bash_command(commandPtr, commandSize, workingDirPtr, workingDirSize)

	// Get the result output
	// First get the length by calling with a zero buffer
	length := get_last_operation_result(0, 0)
	if length >= 0xFFFFFFF0 {
		result.Error = fmt.Sprintf("failed to get result length: %d", length)
		result.ExitCode = -1
		return result
	}

	// If length is 0, set empty output
	if length == 0 {
		result.Stdout = ""
	} else {
		// Allocate a buffer of the required size
		buffer := make([]byte, length)

		// Call again with the buffer
		resultCode := get_last_operation_result(uint32(uintptr(unsafe.Pointer(&buffer[0]))), length)
		if resultCode >= 0xFFFFFFF0 {
			result.Error = fmt.Sprintf("failed to get result: %d", resultCode)
			result.ExitCode = -1
			return result
		}

		// Set output
		outputStr := string(buffer[:length])
		result.Stdout = strings.TrimSpace(outputStr)
	}

	// Set exit code and success based on error code from host function
	if errorCode != 0 {
		// Host function returned an error code, command didn't execute properly
		result.ExitCode = int(errorCode)
		result.Success = false

		// Map specific error codes to meaningful messages
		switch errorCode {
		case 0xFFFFFFFA:
			result.Error = "command execution was cancelled"
		case 0xFFFFFFF0:
			result.Error = "failed to read command from WASM memory"
		case 0xFFFFFFF1:
			result.Error = "failed to read working directory from WASM memory"
		case 0xFFFFFFF2:
			result.Error = "failed to get current working directory"
		case 0xFFFFFFF3:
			result.Error = fmt.Sprintf("working directory does not exist: %s - please ensure the directory exists before running validation", workingDir)
		case 0xFFFFFFF4:
			result.Error = "command timed out after 30 seconds"
		case 0xFFFFFFF5:
			result.Error = "command execution failed"
		default:
			result.Error = fmt.Sprintf("command execution failed with error code: %d", errorCode)
		}
	} else {
		// Host function returned success (0), but we need to check the operation status
		// to see if the command itself succeeded or failed
		status := get_last_operation_status()

		// Status of 0 means command succeeded, any other value means command failed
		result.Success = status == 0
		result.ExitCode = int(status)

		// If the command failed (status != 0), set an appropriate error message
		if !result.Success {
			result.Error = fmt.Sprintf("command exited with status %d", status)
		}
	}

	return result
}

// triggerCorrectiveWorkflow triggers the corrective workflow with validation context
func triggerCorrectiveWorkflow(config Configuration, validationResult ValidationResult, attempt int) error {
	// Create validation context
	context := ValidationContext{
		OriginalPrompt:       config.ValidationCommand,
		ValidationOutput:     validationResult,
		WorkingDirectory:     config.WorkingDirectory,
		RemainingAttempts:    config.MaxAttempts - attempt,
		CorrectiveWorkflowID: config.CorrectiveWorkflowID,
	}

	// Prepare parameters for execute_target
	params := map[string]interface{}{
		"validation_context": context,
		"async":              false, // Run synchronously
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("failed to marshal parameters: %w", err)
	}

	// Convert strings to pointers for host function call
	operationType := "workflow"
	operationTypePtr, operationTypeSize, operationTypeBytes := stringToPtr(operationType)
	idPtr, idSize, idBytes := stringToPtr(config.CorrectiveWorkflowID)
	paramsPtr, paramsSize, paramsBytes := stringToPtr(string(paramsJSON))

	// Keep references to prevent garbage collection
	_ = operationTypeBytes
	_ = idBytes
	_ = paramsBytes

	// Call the execute_target host function
	errorCode := execute_target(operationTypePtr, operationTypeSize, idPtr, idSize, paramsPtr, paramsSize)
	if errorCode != 0 {
		return fmt.Errorf("failed to execute corrective workflow: error code %d", errorCode)
	}

	// Get the result which should contain the job ID
	result, err := getLastOperationResult()
	if err != nil {
		return fmt.Errorf("failed to get operation result: %w", err)
	}

	// Parse the result to get the job ID
	var jobResult map[string]interface{}
	if err := json.Unmarshal(result, &jobResult); err != nil {
		return fmt.Errorf("failed to parse job result: %w", err)
	}

	jobID, ok := jobResult["job_id"].(string)
	if !ok {
		return fmt.Errorf("job ID not found in result: %s", string(result))
	}

	// Wait for job completion
	if err := waitForJobCompletion(jobID); err != nil {
		return fmt.Errorf("failed to wait for job completion: %w", err)
	}

	return nil
}

// getLastOperationResult retrieves the result of the last operation
func getLastOperationResult() ([]byte, error) {
	// First get the length by calling with a zero buffer
	length := get_last_operation_result(0, 0)
	if length >= 0xFFFFFFF0 {
		return nil, fmt.Errorf("failed to get result length: %d", length)
	}

	// Allocate a buffer for the result data
	buffer := make([]byte, length)

	// If length is 0, return empty buffer
	if length == 0 {
		return buffer, nil
	}

	// Get pointer to buffer
	bufferPtr := uint32(uintptr(unsafe.Pointer(&buffer[0])))

	// Call the function again with the actual buffer to read the data
	actualLength := get_last_operation_result(bufferPtr, uint32(length))
	if actualLength >= 0xFFFFFFF0 {
		return nil, fmt.Errorf("failed to read result data: %d", actualLength)
	}

	// Return the buffer with the actual data
	return buffer, nil
}

// waitForJobCompletion waits for a job to complete and retrieves its output
func waitForJobCompletion(jobID string) error {
	// Convert job ID to pointer and size
	jobIDPtr, jobIDSize, jobIDBytes := stringToPtr(jobID)
	_ = jobIDBytes // Prevent garbage collection

	// First get the length by calling with a zero buffer
	length := wait_for_job_and_get_output(jobIDPtr, jobIDSize, 0, 0)
	if length >= 0xFFFFFFF0 {
		return fmt.Errorf("failed to wait for job and get output length: %d", length)
	}

	// Allocate a buffer for the job output data
	buffer := make([]byte, length)

	// If length is 0, return empty buffer
	if length == 0 {
		return nil
	}

	// Get pointer to buffer
	bufferPtr := uint32(uintptr(unsafe.Pointer(&buffer[0])))

	// Call the function again with the actual buffer to read the data
	actualLength := wait_for_job_and_get_output(jobIDPtr, jobIDSize, bufferPtr, uint32(length))
	if actualLength >= 0xFFFFFFF0 {
		return fmt.Errorf("failed to wait for job and get output data: %d", actualLength)
	}

	// Parse the job output to check for errors
	var jobResponse map[string]interface{}
	if err := json.Unmarshal(buffer, &jobResponse); err != nil {
		return fmt.Errorf("failed to parse job response: %w, output: %s", err, string(buffer))
	}

	// Check job status
	status, ok := jobResponse["status"].(string)
	if !ok {
		return fmt.Errorf("failed to extract status from job response: %+v", jobResponse)
	}

	if status != "completed" {
		return fmt.Errorf("job did not complete successfully, status: %s", status)
	}

	return nil
}

// getWorkingDirectory gets the current working directory from the host
func getWorkingDirectory() (string, error) {
	// First get the length by calling with a zero buffer
	length := get_working_directory(0, 0)
	if length >= 0xFFFFFFF0 {
		return "", fmt.Errorf("failed to get working directory length: %d", length)
	}

	// If length is 0, return empty string
	if length == 0 {
		return "", nil
	}

	// Allocate a buffer of the required size
	buffer := make([]byte, length)

	// Call again with the buffer
	result := get_working_directory(uint32(uintptr(unsafe.Pointer(&buffer[0]))), length)
	if result >= 0xFFFFFFF0 {
		return "", fmt.Errorf("failed to get working directory: %d", result)
	}

	return string(buffer[:length]), nil
}

func main() {
	// Read input configuration from stdin
	var config Configuration
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&config); err != nil {
		output := map[string]interface{}{
			"success": false,
			"error":   fmt.Sprintf("Error reading configuration: %v", err),
		}

		outputJSON, _ := json.Marshal(output)
		fmt.Fprintln(os.Stderr, string(outputJSON))
		os.Exit(1)
	}

	// Validate required configuration
	if config.ValidationCommand == "" {
		output := map[string]interface{}{
			"success": false,
			"error":   "validation_command is required",
		}

		outputJSON, _ := json.Marshal(output)
		fmt.Fprintln(os.Stderr, string(outputJSON))
		os.Exit(1)
	}

	// If working directory is not specified in config, get it from host
	if config.WorkingDirectory == "" {
		wd, err := getWorkingDirectory()
		if err != nil {
			output := map[string]interface{}{
				"success": false,
				"error":   fmt.Sprintf("Error getting working directory: %v", err),
			}

			outputJSON, _ := json.Marshal(output)
			fmt.Fprintln(os.Stderr, string(outputJSON))
			os.Exit(1)
		}
		config.WorkingDirectory = wd
	}

	// Set default max attempts if not specified
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 3
	}

	// Validate that corrective workflow ID is provided if needed
	if config.CorrectiveWorkflowID == "" && config.MaxAttempts > 1 {
		fmt.Fprintf(os.Stderr, "Warning: max_attempts > 1 but no corrective_workflow_id provided. Failures will not be corrected.\n")
	}

	// Execute validation loop
	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		// Execute validation command
		result := executeCommand(config.ValidationCommand, config.WorkingDirectory)
		result.Attempt = attempt

		// If validation succeeded, return success
		if result.Success {
			output := map[string]interface{}{
				"success":  true,
				"result":   result,
				"message":  "Validation succeeded",
				"attempts": attempt,
			}

			outputJSON, err := json.Marshal(output)
			if err != nil {
				fmt.Fprintf(os.Stderr, "{\"success\":false,\"error\":\"Error marshaling output: %v\"}\n", err)
				os.Exit(1)
			}

			fmt.Println(string(outputJSON))
			return
		}

		// Log the failure
		fmt.Fprintf(os.Stderr, "Validation attempt %d failed with exit code %d\n", attempt, result.ExitCode)

		// If this was the last attempt, return failure
		if attempt >= config.MaxAttempts {
			output := map[string]interface{}{
				"success":  false,
				"result":   result,
				"message":  "Validation failed after maximum attempts",
				"attempts": attempt,
			}

			outputJSON, err := json.Marshal(output)
			if err != nil {
				fmt.Fprintf(os.Stderr, "{\"success\":false,\"error\":\"Error marshaling output: %v\"}\n", err)
				os.Exit(1)
			}

			fmt.Println(string(outputJSON))
			return
		}

		// Validation failed but we have more attempts left
		// Trigger corrective workflow if configured
		if config.CorrectiveWorkflowID != "" {
			err := triggerCorrectiveWorkflow(config, result, attempt)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: Error triggering corrective workflow: %v\n", err)
				// Continue with next attempt even if workflow failed
			} else {
				fmt.Fprintf(os.Stderr, "Corrective workflow triggered successfully\n")
			}
		} else {
			fmt.Fprintf(os.Stderr, "No corrective workflow configured, retrying validation\n")
		}
	}

	// This should never be reached due to the loop condition
	fmt.Fprintf(os.Stderr, "{\"success\":false,\"error\":\"Unexpected end of validation loop\"}\n")
	os.Exit(1)
}
