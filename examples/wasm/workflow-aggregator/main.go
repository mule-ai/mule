//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
	"unsafe"
)

// Output represents the output structure
type Output struct {
	Message string           `json:"message,omitempty"`
	Results []WorkflowResult `json:"results,omitempty"`
	Success bool             `json:"success"`
	Error   string           `json:"error,omitempty"`
}

// execute_target is the host function for triggering workflows or calling agents
//
//go:wasmimport env execute_target
func execute_target(targetTypePtr, targetTypeSize, targetIDPtr, targetIDSize, paramsPtr, paramsSize uint32) uint32

// get_last_operation_result retrieves the result of the last operation
//
//go:wasmimport env get_last_operation_result
func get_last_operation_result(bufferPtr, bufferSize uint32) uint32

// get_last_operation_status retrieves the status of the last operation
//
//go:wasmimport env get_last_operation_status
func get_last_operation_status() int32

// http_request_with_headers makes an HTTP request with headers
//
//go:wasmimport env http_request_with_headers
func http_request_with_headers(methodPtr, methodSize, urlPtr, urlSize, bodyPtr, bodySize, headersPtr, headersSize uint32) uint32

// get_last_response_body retrieves the body of the last HTTP response
//
//go:wasmimport env get_last_response_body
func get_last_response_body(bufferPtr, bufferSize uint32) uint32

// get_last_response_status retrieves the status code of the last HTTP response
//
//go:wasmimport env get_last_response_status
func get_last_response_status() uint32

// get_job_output retrieves the output data of a job by ID
//
//go:wasmimport env get_job_output
func get_job_output(jobIDPtr, jobIDSize, bufferPtr, bufferSize uint32) uint32

// wait_for_job_and_get_output waits for a job to complete and retrieves its output
//
//go:wasmimport env wait_for_job_and_get_output
func wait_for_job_and_get_output(jobIDPtr, jobIDSize, bufferPtr, bufferSize uint32) uint32

// Mutex to serialize HTTP requests to avoid concurrency issues
// var httpMutex sync.Mutex

// Helper function to convert string to pointer and size
// Returns the pointer, size, and byte slice (to prevent garbage collection)
// Note: In a real WASM module, you would allocate in WASM linear memory
func stringToPtr(s string) (uint32, uint32, []byte) {
	if s == "" {
		return 0, 0, nil
	}
	// Create a byte slice and return it to prevent garbage collection
	bytes := []byte(s)
	ptr := uint32(uintptr(unsafe.Pointer(&bytes[0])))
	return ptr, uint32(len(bytes)), bytes
}

// Helper function to read the last operation result
// This reads the actual data from WASM memory
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

// Helper function to read the last HTTP response body
func getLastHTTPResponseBody() ([]byte, error) {
	// First get the length by calling with a zero buffer
	length := get_last_response_body(0, 0)
	if length >= 0xFFFFFFF0 {
		return nil, fmt.Errorf("failed to get response body length: %d", length)
	}

	// Allocate a buffer for the response data
	buffer := make([]byte, length)

	// If length is 0, return empty buffer
	if length == 0 {
		return buffer, nil
	}

	// Get pointer to buffer
	bufferPtr := uint32(uintptr(unsafe.Pointer(&buffer[0])))

	// Call the function again with the actual buffer to read the data
	actualLength := get_last_response_body(bufferPtr, uint32(length))
	if actualLength >= 0xFFFFFFF0 {
		return nil, fmt.Errorf("failed to read response body data: %d", actualLength)
	}

	// Return the buffer with the actual data
	return buffer, nil
}

// Helper function to read job output by job ID
func getJobOutput(jobID string) ([]byte, error) {
	// Convert job ID to pointer and size
	jobIDPtr, jobIDSize, jobIDBytes := stringToPtr(jobID)
	_ = jobIDBytes // Prevent garbage collection

	// First get the length by calling with a zero buffer
	length := get_job_output(jobIDPtr, jobIDSize, 0, 0)
	if length >= 0xFFFFFFF0 {
		return nil, fmt.Errorf("failed to get job output length: %d", length)
	}

	// Allocate a buffer for the job output data
	buffer := make([]byte, length)

	// If length is 0, return empty buffer
	if length == 0 {
		return buffer, nil
	}

	// Get pointer to buffer
	bufferPtr := uint32(uintptr(unsafe.Pointer(&buffer[0])))

	// Call the function again with the actual buffer to read the data
	actualLength := get_job_output(jobIDPtr, jobIDSize, bufferPtr, uint32(length))
	if actualLength >= 0xFFFFFFF0 {
		return nil, fmt.Errorf("failed to read job output data: %d", actualLength)
	}

	// Return the buffer with the actual data
	return buffer, nil
}

// Helper function to wait for job completion and get output
func waitForJobAndGetObject(jobID string) ([]byte, error) {
	// Convert job ID to pointer and size
	jobIDPtr, jobIDSize, jobIDBytes := stringToPtr(jobID)
	_ = jobIDBytes // Prevent garbage collection

	// First get the length by calling with a zero buffer
	length := wait_for_job_and_get_output(jobIDPtr, jobIDSize, 0, 0)
	if length >= 0xFFFFFFF0 {
		return nil, fmt.Errorf("failed to wait for job and get output length: %d", length)
	}

	// Allocate a buffer for the job output data
	buffer := make([]byte, length)

	// If length is 0, return empty buffer
	if length == 0 {
		return buffer, nil
	}

	// Get pointer to buffer
	bufferPtr := uint32(uintptr(unsafe.Pointer(&buffer[0])))

	// Call the function again with the actual buffer to read the data
	actualLength := wait_for_job_and_get_output(jobIDPtr, jobIDSize, bufferPtr, uint32(length))
	if actualLength >= 0xFFFFFFF0 {
		return nil, fmt.Errorf("failed to wait for job and get output data: %d", actualLength)
	}

	// Return the buffer with the actual data
	return buffer, nil
}

// WorkflowResult represents the result of executing a workflow
type WorkflowResult struct {
	Name    string      `json:"name"`
	Success bool        `json:"success"`
	Output  interface{} `json:"output,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// executeWorkflow executes a single workflow with the given parameters
func executeWorkflow(name string, params map[string]interface{}, wg *sync.WaitGroup, results chan<- WorkflowResult) {
	defer wg.Done()

	// Convert params to JSON
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		results <- WorkflowResult{
			Name:  name,
			Error: fmt.Sprintf("Error marshaling params: %v", err),
		}
		return
	}

	// Convert strings to pointers and sizes
	// Target type is "workflow"
	targetType := "workflow"
	targetTypePtr, targetTypeSize, targetTypeBytes := stringToPtr(targetType)

	// Target ID - use the workflow name
	targetID := name
	targetIDPtr, targetIDSize, targetIDBytes := stringToPtr(targetID)

	// Params JSON
	paramsPtr, paramsSize, paramsBytes := stringToPtr(string(paramsJSON))

	// Keep references to prevent garbage collection
	_ = targetTypeBytes
	_ = targetIDBytes
	_ = paramsBytes

	// Call the execute_target host function to trigger the workflow
	errorCode := execute_target(targetTypePtr, targetTypeSize, targetIDPtr, targetIDSize, paramsPtr, paramsSize)
	if errorCode != 0 {
		results <- WorkflowResult{
			Name:  name,
			Error: fmt.Sprintf("Error executing workflow: %d", errorCode),
		}
		return
	}

	// Get the status
	status := get_last_operation_status()
	if status != 0 {
		results <- WorkflowResult{
			Name:  name,
			Error: fmt.Sprintf("Workflow execution failed with status: %d", status),
		}
		return
	}

	// Get the result (should contain job ID)
	result, err := getLastOperationResult()
	if err != nil {
		results <- WorkflowResult{
			Name:  name,
			Error: fmt.Sprintf("Error getting result: %v", err),
		}
		return
	}

	// Parse the result to get the job ID
	var jobResult map[string]interface{}
	if err := json.Unmarshal(result, &jobResult); err != nil {
		results <- WorkflowResult{
			Name:  name,
			Error: fmt.Sprintf("Error parsing job result: %v", err),
		}
		return
	}

	jobID, ok := jobResult["job_id"].(string)
	if !ok {
		results <- WorkflowResult{
			Name:  name,
			Error: fmt.Sprintf("Job ID not found in result: %s", string(result)),
		}
		return
	}

	// Wait for job completion by polling
	output, err := waitForJobCompletion(jobID)
	if err != nil {
		results <- WorkflowResult{
			Name:  name,
			Error: fmt.Sprintf("Error waiting for job completion: %v", err),
		}
		return
	}

	results <- WorkflowResult{
		Name:    name,
		Success: true,
		Output:  output,
	}
}

// waitForJobCompletion waits for a job to complete and returns the output
func waitForJobCompletion(jobID string) (interface{}, error) {
	// Log start time for debugging
	startTime := time.Now()
	fmt.Fprintf(os.Stderr, "DEBUG: Starting to wait for job %s at %v\n", jobID, startTime)

	// Use the host function to wait for job completion and get the result
	output, err := waitForJobAndGetObject(jobID)
	if err != nil {
		elapsed := time.Since(startTime)
		fmt.Fprintf(os.Stderr, "DEBUG: Failed to wait for job %s after %v: %v\n", jobID, elapsed, err)
		return nil, fmt.Errorf("failed to wait for job completion: %v", err)
	}

	// Log the raw output for debugging
	fmt.Fprintf(os.Stderr, "DEBUG: Raw job output for job %s: %s\n", jobID, string(output))

	// Parse the job output to extract status and output data
	var jobResponse map[string]interface{}
	if err := json.Unmarshal(output, &jobResponse); err != nil {
		elapsed := time.Since(startTime)
		fmt.Fprintf(os.Stderr, "DEBUG: Failed to parse job response for job %s after %v: %v\n", jobID, elapsed, err)
		return nil, fmt.Errorf("failed to parse job response: %v, output: %s", err, string(output))
	}

	// Log the parsed job response for debugging
	fmt.Fprintf(os.Stderr, "DEBUG: Parsed job response for job %s: %+v\n", jobID, jobResponse)

	// Extract status and output from the response
	status, ok := jobResponse["status"].(string)
	if !ok {
		elapsed := time.Since(startTime)
		fmt.Fprintf(os.Stderr, "DEBUG: Failed to extract status from job response for job %s after %v: %+v\n", jobID, elapsed, jobResponse)
		return nil, fmt.Errorf("failed to extract status from job response: %+v", jobResponse)
	}

	fmt.Fprintf(os.Stderr, "DEBUG: Extracted status for job %s: %s\n", jobID, status)

	outputData, ok := jobResponse["output"]
	if !ok {
		elapsed := time.Since(startTime)
		fmt.Fprintf(os.Stderr, "DEBUG: Failed to extract output from job response for job %s after %v: %+v\n", jobID, elapsed, jobResponse)
		return nil, fmt.Errorf("failed to extract output from job response: %+v", jobResponse)
	}

	// Try to extract the actual output content from various possible fields
	var actualOutput interface{} = outputData

	// Check for "prompt" field (common for agent steps)
	if promptMap, ok := outputData.(map[string]interface{}); ok {
		if prompt, ok := promptMap["prompt"]; ok {
			actualOutput = prompt
			fmt.Fprintf(os.Stderr, "DEBUG: Found prompt field: %v\n", prompt)
		}

		// Check for "output" field (common for WASM steps)
		if outputField, ok := promptMap["output"]; ok {
			actualOutput = outputField
			fmt.Fprintf(os.Stderr, "DEBUG: Found output field: %v\n", outputField)
		}

		// Check for "message" field
		if message, ok := promptMap["message"]; ok {
			actualOutput = message
			fmt.Fprintf(os.Stderr, "DEBUG: Found message field: %v\n", message)
		}
	}

	fmt.Fprintf(os.Stderr, "DEBUG: Extracted actual output for job %s: %v\n", jobID, actualOutput)

	elapsed := time.Since(startTime)
	fmt.Fprintf(os.Stderr, "DEBUG: Job %s completed successfully after %v\n", jobID, elapsed)
	return actualOutput, nil
}

// getJobStatus gets the current status of a job
func getJobStatus(jobID string) (string, interface{}, error) {
	// Try to get job output directly using the host function
	output, err := getJobOutput(jobID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "DEBUG: Failed to get job output for job %s: %v\n", jobID, err)
		return "", nil, fmt.Errorf("failed to get job output: %v", err)
	}

	// Log the raw output for debugging
	fmt.Fprintf(os.Stderr, "DEBUG: Raw job output for job %s: %s\n", jobID, string(output))

	// Parse the job output to extract status and output data
	var jobResponse map[string]interface{}
	if err := json.Unmarshal(output, &jobResponse); err != nil {
		fmt.Fprintf(os.Stderr, "DEBUG: Failed to parse job response for job %s: %v\n", jobID, err)
		return "", nil, fmt.Errorf("failed to parse job response: %v, output: %s", err, string(output))
	}

	// Log the parsed job response for debugging
	fmt.Fprintf(os.Stderr, "DEBUG: Parsed job response for job %s: %+v\n", jobID, jobResponse)

	// Extract status and output from the response
	status, ok := jobResponse["status"].(string)
	if !ok {
		fmt.Fprintf(os.Stderr, "DEBUG: Failed to extract status from job response for job %s: %+v\n", jobID, jobResponse)
		return "", nil, fmt.Errorf("failed to extract status from job response: %+v", jobResponse)
	}

	fmt.Fprintf(os.Stderr, "DEBUG: Extracted status for job %s: %s\n", jobID, status)

	outputData, ok := jobResponse["output"]
	if !ok {
		fmt.Fprintf(os.Stderr, "DEBUG: Failed to extract output from job response for job %s: %+v\n", jobID, jobResponse)
		return "", nil, fmt.Errorf("failed to extract output from job response: %+v", jobResponse)
	}

	// Try to extract the actual output content from various possible fields
	var actualOutput interface{} = outputData

	// Check for "prompt" field (common for agent steps)
	if promptMap, ok := outputData.(map[string]interface{}); ok {
		if prompt, ok := promptMap["prompt"]; ok {
			actualOutput = prompt
			fmt.Fprintf(os.Stderr, "DEBUG: Found prompt field: %v\n", prompt)
		}

		// Check for "output" field (common for WASM steps)
		if outputField, ok := promptMap["output"]; ok {
			actualOutput = outputField
			fmt.Fprintf(os.Stderr, "DEBUG: Found output field: %v\n", outputField)
		}

		// Check for "message" field
		if message, ok := promptMap["message"]; ok {
			actualOutput = message
			fmt.Fprintf(os.Stderr, "DEBUG: Found message field: %v\n", message)
		}
	}

	fmt.Fprintf(os.Stderr, "DEBUG: Extracted actual output for job %s: %v\n", jobID, actualOutput)
	return status, actualOutput, nil
}

func main() {
	// Read input from stdin
	var inputData map[string]interface{}
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&inputData); err != nil {
		outputError(fmt.Errorf("error reading input"))
		return
	}

	// Extract workflow names from config
	workflowNames, ok := inputData["workflow_names"]
	if !ok {
		outputError(fmt.Errorf("missing workflow_names"))
		return
	}

	// Convert to string slice
	namesSlice, ok := workflowNames.([]interface{})
	if !ok {
		outputError(fmt.Errorf("missing workflow_names"))
		return
	}

	// Convert interface{} slice to string slice
	var workflowNameStrings []string
	for _, name := range namesSlice {
		if nameStr, ok := name.(string); ok {
			workflowNameStrings = append(workflowNameStrings, nameStr)
		}
	}

	// Extract prompt from input (will be passed to all workflows)
	prompt := inputData["prompt"]

	// Extract working directory from config (optional)
	workingDir := ""
	if wd, ok := inputData["working_directory"].(string); ok {
		workingDir = wd
	}

	// Process each workflow
	var wg sync.WaitGroup
	results := make(chan WorkflowResult, len(workflowNameStrings))

	// Launch workflows in parallel
	for _, name := range workflowNameStrings {
		// Prepare parameters for the workflow
		params := map[string]interface{}{
			"prompt": prompt,
		}

		// If a working directory is specified, add it to the params
		if workingDir != "" {
			params["working_directory"] = workingDir
		}

		// Launch workflow in a goroutine
		wg.Add(1)
		go executeWorkflow(name, params, &wg, results)
	}

	// Close results channel when all goroutines are done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect all results
	var allResults []WorkflowResult
	for result := range results {
		allResults = append(allResults, result)
	}

	// Aggregate all outputs into a single string
	var aggregatedOutput string
	successCount := 0
	for _, result := range allResults {
		if result.Success {
			successCount++
			// Convert output to string and append
			if result.Output != nil {
				if outputStr, ok := result.Output.(string); ok {
					aggregatedOutput += fmt.Sprintf("%s\n", outputStr)
				} else {
					// Convert non-string output to JSON string
					outputBytes, err := json.Marshal(result.Output)
					if err != nil {
						aggregatedOutput += fmt.Sprintf("%v\n", result.Output)
					} else {
						aggregatedOutput += fmt.Sprintf("%s\n", string(outputBytes))
					}
				}
			}
		}
	}
	// Create output
	output := Output{
		Message: aggregatedOutput,
		Results: allResults,
		Success: true,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		outputError(fmt.Errorf("failed to encode output: %w", err))
		return
	}
}

// outputError outputs an error message in the expected format
func outputError(err error) {
	output := Output{
		Success: false,
		Error:   err.Error(),
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	_ = encoder.Encode(output)

	os.Exit(1)
}
