# Unified Workflow and Agent Interface for WASM Modules

This document describes the unified interface available for WASM modules to trigger workflows and call agents directly from within Mule, bypassing HTTP requests.

## Interface: `trigger_workflow_or_agent`

This is the unified interface that supports both workflow triggering and agent calling.

### Function Signature
```go
//go:wasmimport env trigger_workflow_or_agent
func trigger_workflow_or_agent(operationTypePtr, operationTypeSize, idPtr, idSize, paramsPtr, paramsSize uintptr) uintptr
```

### Parameters
- `operationTypePtr` - Pointer to the operation type string ("workflow" or "agent")
- `operationTypeSize` - Size of the operation type string
- `idPtr` - Pointer to the workflow or agent ID/name string
- `idSize` - Size of the workflow or agent ID/name string
- `paramsPtr` - Pointer to the parameters JSON string
- `paramsSize` - Size of the parameters JSON string

### Example Usage
```go
// Triggering a workflow by ID
operationType := "workflow"
workflowID := "workflow-123"
params := `{"input_data": {"key": "value"}, "async": true}`
result := trigger_workflow_or_agent(
    uintptr(unsafe.Pointer(&[]byte(operationType)[0])), uintptr(len(operationType)),
    uintptr(unsafe.Pointer(&[]byte(workflowID)[0])), uintptr(len(workflowID)),
    uintptr(unsafe.Pointer(&[]byte(params)[0])), uintptr(len(params))
)

// Calling an agent by name
operationType := "agent"
agentName := "my-agent"
params := `{"messages": [{"role": "user", "content": "Hello, world!"}]}`
result := trigger_workflow_or_agent(
    uintptr(unsafe.Pointer(&[]byte(operationType)[0])), uintptr(len(operationType)),
    uintptr(unsafe.Pointer(&[]byte(agentName)[0])), uintptr(len(agentName)),
    uintptr(unsafe.Pointer(&[]byte(params)[0])), uintptr(len(params))
)
```

## Interface: `get_last_operation_result`

Retrieves the result of the last workflow or agent operation.

### Function Signature
```go
//go:wasmimport env get_last_operation_result
func get_last_operation_result(bufferPtr, bufferSize uintptr) uintptr
```

### Parameters
- `bufferPtr` - Pointer to the buffer where the result will be written
- `bufferSize` - Size of the buffer

### Return Value
Returns the size of the result data written to the buffer, or an error code.

## Interface: `get_last_operation_status`

Gets the status of the last workflow or agent operation.

### Function Signature
```go
//go:wasmimport env get_last_operation_status
func get_last_operation_status() uintptr
```

### Return Value
Returns the status code of the last operation:
- `0` - No operation performed
- `200` - Operation completed successfully
- Error codes for various failure conditions

## Return Values for `trigger_workflow_or_agent`

The function returns a 32-bit unsigned integer with the following meanings:

- `0` - Success
- `0xFFFFFFF0` - Failed to read operation type from WASM memory
- `0xFFFFFFF1` - Failed to read ID from WASM memory
- `0xFFFFFFF2` - Failed to read parameters from WASM memory
- `0xFFFFFFF3` - Invalid operation type
- `0xFFFFFFF4` - Failed to parse parameters JSON
- `0xFFFFFFFC` - Internal error during operation execution

## Workflow Parameters

When triggering a workflow, the following parameters are supported:

```json
{
  "input_data": {
    "key": "value"
  },
  "async": true
}
```

- `input_data` - Data to pass to the workflow (optional)
- `async` - If true, returns immediately after submitting the job (optional, defaults to false)

## Agent Parameters

When calling an agent, the following parameters are supported:

```json
{
  "messages": [
    {
      "role": "user",
      "content": "Hello, world!"
    }
  ],
  "stream": false
}
```

- `messages` - Array of message objects with role and content
- `stream` - If true, enables streaming responses (optional, defaults to false)
- `prompt` - Alternative to messages, a single prompt string (used when messages is not provided)

## Examples

See the following files for complete examples:
- `examples/wasm/workflow_agent_demo.go` - Basic demonstration
- `examples/wasm/realistic_workflow_agent_demo.go` - More realistic example with proper string handling