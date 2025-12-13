# Workflow Aggregator WASM Module

This WASM module executes multiple workflows in parallel and aggregates their outputs into a single result.

## Overview

The module demonstrates how to:
1. Read configuration data from stdin containing an array of workflow names
2. Execute each workflow in parallel using goroutines
3. Wait for each workflow to complete by polling job status
4. Collect and aggregate results from all executed workflows
5. Output combined results in JSON format

## Prerequisites

Before running this module, you need to:

1. Have a running Mule server
2. Ensure the workflows specified in the configuration exist
3. Register this WASM module with the Mule server

## Usage

To compile and use this module:

1. Compile to WASM:
   ```bash
   cd examples/wasm/workflow-aggregator
   ./build.sh
   ```

2. Upload to Mule and execute with input like:
   ```json
   {
     "workflow_names": ["workflow-1", "workflow-2", "workflow-3"],
     "prompt": "Process this input with all workflows",
     "working_directory": "/path/to/working/directory"
   }
   ```

3. The module will execute each workflow in parallel and return aggregated results.

## Configuration

The module accepts the following parameters in the input data:

- `workflow_names` (required): An array of workflow names to execute
- `prompt` (optional): A prompt string that will be passed to all workflows
- `working_directory` (optional): A working directory that will be passed to all workflows

Configuration can also be provided when registering the WASM module in Mule, which will be merged with the input data.

## How It Works

The module accepts a `workflow_names` parameter in the input data that specifies which workflows to execute. Each workflow is executed in parallel using goroutines:

```go
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
```

Each workflow is triggered using the `execute_target` host function, and then the module waits for job completion using the `wait_for_job_and_get_output` host function:

```go
// Call the execute_target host function to trigger the workflow
errorCode := execute_target(targetTypePtr, targetTypeSize, targetIDPtr, targetIDSize, paramsPtr, paramsSize)

// Wait for job completion using the host function
output, err := waitForJobCompletion(jobID)
```

The module uses the `wait_for_job_and_get_output` host function to wait for job completion and retrieve job output directly from the database, eliminating the need for HTTP requests or manual polling:

```go
// Wait for job completion and get output using the host function
output, err := waitForJobAndGetObject(jobID)
```

## Expected Output

The module will return a JSON object with results from all workflows and an aggregated output:

```json
{
  "message": "[workflow-1]: Result from workflow 1\n[workflow-2]: Result from workflow 2\n",
  "results": [
    {
      "name": "workflow-1",
      "success": true,
      "output": {
        "result": "Result from workflow 1"
      }
    },
    {
      "name": "workflow-2",
      "success": true,
      "output": {
        "result": "Result from workflow 2"
      }
    }
  ],
  "success": true
}
```

## Testing Locally

You can test the module locally with sample input:

```bash
# Create test input
cat > test-input.json << EOF
{
  "workflow_names": ["test-workflow-1", "test-workflow-2"],
  "prompt": "Test input for workflows",
  "working_directory": "/tmp/mule-test"
}
EOF

# Test the module (this requires a WASM runtime like wasmtime)
cat test-input.json | wasmtime workflow-aggregator.wasm
```

Note: The host functions (`execute_target`, `wait_for_job_and_get_output`, etc.) will only work when the module is executed within the Mule runtime.