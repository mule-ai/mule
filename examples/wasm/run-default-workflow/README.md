# Run Default Workflow WASM Module Example

This example demonstrates how to trigger the default workflow from a WASM module using the `execute_target` host function.

## Overview

The module shows how to:
1. Read input data from stdin
2. Trigger the default workflow using the `execute_target` host function
3. Pass input data to the workflow
4. Handle the workflow response
5. Output results in JSON format

## Prerequisites

Before running this example, you need to:

1. Have a running Mule server
2. Ensure the default workflow exists (it's created automatically when Mule starts)
3. Optionally, add steps to the default workflow for it to do something meaningful

## Setting Up a Complete Workflow

If you want to create a complete workflow with steps, you can use the provided setup script:

```bash
./setup-workflow.sh
```

This script will:
1. Create a provider
2. Create an agent
3. Create a workflow
4. Add a step to the workflow

## Usage

To compile and use this module:

1. Compile to WASM:
   ```bash
   # Create a separate go.mod file for the WASM module
   echo "module github.com/mule-ai/mule/examples/wasm/run-default-workflow" > go.mod
   echo "go 1.25.4" >> go.mod

   # Build only the main.go file to avoid including other examples
   GOOS=wasip1 GOARCH=wasm go build -o run-default-workflow.wasm main.go
   ```

2. Upload to Mule and execute with input like:
   ```json
   {
     "prompt": "Hello, process this text"
   }
   ```

3. The module will trigger the default workflow with the provided input and return the result

## How It Works

The module uses the `execute_target` host function to directly trigger workflows without making HTTP requests:

```go
// Target type is "workflow"
targetType := "workflow"
// Target ID is "Default" for the default workflow
targetID := "Default"
// Parameters contain the input data
params := map[string]interface{}{
    "prompt": "Hello, world!",
}

// Call the execute_target host function
errorCode := execute_target(targetTypePtr, targetTypeSize, targetIDPtr, targetIDSize, paramsPtr, paramsSize)
```

## Expected Output

The module will return a JSON object with the workflow result:

```json
{
  "success": true,
  "data": {
    "result": "Workflow execution result"
  },
  "status": 0
}
```

## Testing Locally

You can test the module locally with sample input:

```bash
# Create test input
echo '{"prompt": "Hello, process this text"}' > test-input.json

# Test the module (this requires a WASM runtime like wasmtime)
cat test-input.json | wasmtime run-default-workflow.wasm
```

Note: The host functions will only work when the module is executed within the Mule runtime.