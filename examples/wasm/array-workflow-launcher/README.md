# Array Workflow Launcher WASM Module Example

This example demonstrates how to process a JSON array and launch multiple workflows in parallel using the `execute_target` host function.

## Overview

The module shows how to:
1. Read input data from stdin containing a JSON array
2. Parse the array and extract relevant fields
3. Launch multiple workflows in parallel using goroutines
4. Collect and aggregate results from all launched workflows
5. Output results in JSON format

## Prerequisites

Before running this example, you need to:

1. Have a running Mule server
2. Ensure the default workflow exists (it's created automatically when Mule starts)
3. Optionally, add steps to the default workflow for it to do something meaningful

## Usage

To compile and use this module:

1. Compile to WASM:
   ```bash
   # Create a separate go.mod file for the WASM module
   echo "module github.com/mule-ai/mule/examples/wasm/array-workflow-launcher" > go.mod
   echo "go 1.25.4" >> go.mod

   # Build only the main.go file to avoid including other examples
   GOOS=wasip1 GOARCH=wasm go build -o array-workflow-launcher.wasm main.go
   ```

2. Upload to Mule and execute with input like:
   ```json
   {
     "prompt": {
       "result": [
         {
           "body": "- [x] Memory Tools\n- [ ] Meal planning workflow\n- [ ] Add daily tasks to morning message\n- [ ] Reminders about overdue tasks\n- [ ] Notification when task becomes due\n- [ ] GitHub workflow\n- [ ] Workflow visualizer",
           "due_date": "No Due Date",
           "filter": "Personal",
           "state": "open",
           "status": "In Progress",
           "title": "Improve Mule Assistant"
         },
         {
           "body": "https://www.joshbeckman.org/subscribe/",
           "due_date": "No Due Date",
           "filter": "Personal",
           "state": "open",
           "status": "Todo",
           "title": "Add Josh Beckman blog to mule RSS"
         }
       ]
     },
     "workflow": "my-custom-workflow",
     "working_directory": "/path/to/working/directory"
   }
   ```

3. The module will process each item in the array and trigger a workflow for each one, returning aggregated results. If no `workflow` field is provided, it defaults to using the "Default" workflow. If a `working_directory` is provided, it will be passed to each launched workflow as the working directory for that workflow execution.

## How It Works

The module accepts an optional `workflow` parameter in the input data that specifies which workflow to execute for each item. If not provided, it defaults to the "Default" workflow.

The module also accepts an optional `working_directory` parameter that specifies the working directory for all launched workflows. If provided, this directory will be passed to each workflow execution through the params.

Instead of extracting specific fields like "title" and "body", the module now passes the entire JSON representation of each array item to the workflow. This allows workflows to access all available data in the item, regardless of its structure.

Configuration can also be provided when registering the WASM module in Mule, which will be merged with the input data.

The module uses goroutines to launch multiple workflows in parallel:

```go
// Launch workflows in parallel
for i, item := range resultArray {
    // Convert the entire item to JSON string
    itemJSON, err := json.Marshal(item)
    if err != nil {
        results <- Result{
            Index: i,
            Error: fmt.Sprintf("Error marshaling item to JSON: %v", err),
        }
        continue
    }

    // Launch workflow in a goroutine, passing the entire JSON string and working directory
    wg.Add(1)
    go launchWorkflow(i, "", string(itemJSON), workflowName, workingDir, &wg, results)
}
```

Each workflow is triggered using the `execute_target` host function. If a working directory is specified, it's passed as part of the params:

```go
// Prepare parameters for the workflow
params := map[string]interface{}{
    "prompt": body,
}

// If a working directory is specified, add it to the params
if workingDir != "" {
    params["working_directory"] = workingDir
}

// Call the execute_target host function to trigger the workflow
errorCode := execute_target(targetTypePtr, targetTypeSize, targetIDPtr, targetIDSize, paramsPtr, paramsSize)
```

## Expected Output

The module will return a JSON object with results from all workflows:

```json
{
  "results": [
    {
      "index": 0,
      "success": true,
      "data": {
        "result": "Workflow execution result for first item"
      }
    },
    {
      "index": 1,
      "success": true,
      "data": {
        "result": "Workflow execution result for second item"
      }
    }
  ]
}
```

## Testing Locally

You can test the module locally with sample input:

```bash
# Create test input
cat > test-input.json << EOF
{
  "prompt": {
    "result": [
      {
        "body": "- [x] Memory Tools\n- [ ] Meal planning workflow\n- [ ] Add daily tasks to morning message\n- [ ] Reminders about overdue tasks\n- [ ] Notification when task becomes due\n- [ ] GitHub workflow\n- [ ] Workflow visualizer",
        "due_date": "No Due Date",
        "filter": "Personal",
        "state": "open",
        "status": "In Progress",
        "title": "Improve Mule Assistant"
      },
      {
        "body": "https://www.joshbeckman.org/subscribe/",
        "due_date": "No Due Date",
        "filter": "Personal",
        "state": "open",
        "status": "Todo",
        "title": "Add Josh Beckman blog to mule RSS"
      }
    ]
  },
  "working_directory": "/tmp/mule-test"
}
EOF

# Test the module (this requires a WASM runtime like wasmtime)
cat test-input.json | wasmtime array-workflow-launcher.wasm
```

Note: The host functions will only work when the module is executed within the Mule runtime.