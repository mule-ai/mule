# Execute Target WASM Module Example

This example demonstrates how to trigger workflows or call agents from a WASM module using the `execute_target` host function provided by the Mule runtime.

## Overview

The module shows how to:
1. Read input data from stdin
2. Trigger workflows or call agents using the `execute_target` host function
3. Handle responses and errors properly
4. Output results in JSON format

## Host Function Interface

The module uses the `execute_target` host function which has the following signature:

```go
func execute_target(targetTypePtr, targetTypeSize, targetIDPtr, targetIDSize, paramsPtr, paramsSize uintptr) uintptr
```

Parameters:
- `targetTypePtr` - Pointer to the target type string ("workflow" or "agent")
- `targetTypeSize` - Size of the target type string
- `targetIDPtr` - Pointer to the target ID or name string
- `targetIDSize` - Size of the target ID or name string
- `paramsPtr` - Pointer to the parameters JSON string (can be null/empty)
- `paramsSize` - Size of the parameters JSON string (can be 0)

Returns:
- `errorCode`: Error code (0 for success, non-zero for errors)

After calling `execute_target`, you can retrieve the result using:
- `get_last_operation_result(resultPtr)` - Gets the result data
- `get_last_operation_status()` - Gets the operation status

## Error Codes

The `execute_target` host function returns the following error codes:
- `0x00000000`: Success
- `0xFFFFFFF0`: Failed to read target type from memory
- `0xFFFFFFF1`: Failed to read target ID from memory
- `0xFFFFFFF2`: Failed to read params from memory
- `0xFFFFFFF3`: Failed to parse params JSON
- `0xFFFFFFF4`: Invalid target type
- `0xFFFFFFF5`: Failed to execute target

The `get_last_operation_result` host function returns:
- `0xFFFFFFF0`: No operation result available
- `0xFFFFFFF1`: Failed to write result to WASM memory
- Length of result data (success)

The `get_last_operation_status` host function returns:
- `-1`: No operation status available
- Status code (0 for success, non-zero for errors)

## Usage

To compile and use this module:

1. Compile to WASM:
   ```bash
   # Create a separate go.mod file for the WASM module
   echo "module github.com/mule-ai/mule/examples/wasm/execute-target" > go.mod
   echo "go 1.25.4" >> go.mod

   # Build only the main.go file to avoid including other examples
   GOOS=wasip1 GOARCH=wasm go build -o execute-target.wasm main.go
   ```

2. Upload to Mule and execute with input like:
   ```json
   {
     "target_type": "workflow",
     "target_id": "my-workflow-id",
     "params": {
       "input": "data"
     }
   }
   ```

3. The module will trigger the specified workflow or agent and return the result