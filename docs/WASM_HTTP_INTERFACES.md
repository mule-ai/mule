# WASM HTTP Interfaces in Mule

This document describes the HTTP function interfaces available for WASM modules in Mule.

## Interface: `http_request` (Standard)

This is the standard interface that supports different HTTP methods.

### Function Signature
```go
//go:wasmimport env http_request
func http_request(methodPtr, methodSize, urlPtr, urlSize, bodyPtr, bodySize uintptr) uintptr
```

### Parameters
- `methodPtr` - Pointer to the HTTP method string (GET, POST, PUT, DELETE, etc.)
- `methodSize` - Size of the HTTP method string
- `urlPtr` - Pointer to the URL string
- `urlSize` - Size of the URL string
- `bodyPtr` - Pointer to the request body string (can be null/empty)
- `bodySize` - Size of the request body string (can be 0)

### Example Usage
```go
// Making a GET request
method := "GET"
url := "https://api.example.com/data"
result := http_request(
    uintptr(unsafe.Pointer(&[]byte(method)[0])), uintptr(len(method)),
    uintptr(unsafe.Pointer(&[]byte(url)[0])), uintptr(len(url)),
    0, 0  // No body for GET request
)

// Making a POST request with JSON body
method := "POST"
url := "https://api.example.com/data"
body := `{"name": "John", "age": 30}`
result := http_request(
    uintptr(unsafe.Pointer(&[]byte(method)[0])), uintptr(len(method)),
    uintptr(unsafe.Pointer(&[]byte(url)[0])), uintptr(len(url)),
    uintptr(unsafe.Pointer(&[]byte(body)[0])), uintptr(len(body))
)
```

## Interface: `http_request_with_headers` (Enhanced)

This is the enhanced interface that supports HTTP headers in addition to the standard parameters.

### Function Signature
```go
//go:wasmimport env http_request_with_headers
func http_request_with_headers(methodPtr, methodSize, urlPtr, urlSize, bodyPtr, bodySize, headersPtr, headersSize uintptr) uintptr
```

### Parameters
- `methodPtr` - Pointer to the HTTP method string (GET, POST, PUT, DELETE, etc.)
- `methodSize` - Size of the HTTP method string
- `urlPtr` - Pointer to the URL string
- `urlSize` - Size of the URL string
- `bodyPtr` - Pointer to the request body string (can be null/empty)
- `bodySize` - Size of the request body string (can be 0)
- `headersPtr` - Pointer to the headers JSON string (can be null/empty)
- `headersSize` - Size of the headers JSON string (can be 0)

### Example Usage
```go
// Making a GET request with headers
method := "GET"
url := "https://api.example.com/data"
headers := `{"Authorization": "Bearer token123", "Content-Type": "application/json"}`
result := http_request_with_headers(
    uintptr(unsafe.Pointer(&[]byte(method)[0])), uintptr(len(method)),
    uintptr(unsafe.Pointer(&[]byte(url)[0])), uintptr(len(url)),
    0, 0,  // No body for GET request
    uintptr(unsafe.Pointer(&[]byte(headers)[0])), uintptr(len(headers))
)

// Making a POST request with JSON body and headers
method := "POST"
url := "https://api.example.com/data"
body := `{"name": "John", "age": 30}`
headers := `{"Authorization": "Bearer token123", "Content-Type": "application/json"}`
result := http_request_with_headers(
    uintptr(unsafe.Pointer(&[]byte(method)[0])), uintptr(len(method)),
    uintptr(unsafe.Pointer(&[]byte(url)[0])), uintptr(len(url)),
    uintptr(unsafe.Pointer(&[]byte(body)[0])), uintptr(len(body)),
    uintptr(unsafe.Pointer(&[]byte(headers)[0])), uintptr(len(headers))
)
```

## Response Handling Functions

After making an HTTP request, you can use the following functions to retrieve the response:

### Function: `get_last_response_body`

Retrieves the body of the last HTTP response.

#### Function Signature
```go
//go:wasmimport env get_last_response_body
func get_last_response_body(bufferPtr, bufferSize uintptr) uintptr
```

#### Parameters
- `bufferPtr` - Pointer to the buffer where the response body will be written
- `bufferSize` - Size of the buffer

#### Return Value
Returns the size of the response body written to the buffer, or an error code.

### Function: `get_last_response_status`

Gets the HTTP status code of the last response.

#### Function Signature
```go
//go:wasmimport env get_last_response_status
func get_last_response_status() uintptr
```

#### Return Value
Returns the HTTP status code of the last response, or an error code.

### Function: `get_last_response_header`

Gets the value of a specific header from the last response.

#### Function Signature
```go
//go:wasmimport env get_last_response_header
func get_last_response_header(headerNamePtr, headerNameSize, bufferPtr, bufferSize uintptr) uintptr
```

#### Parameters
- `headerNamePtr` - Pointer to the header name string
- `headerNameSize` - Size of the header name string
- `bufferPtr` - Pointer to the buffer where the header value will be written
- `bufferSize` - Size of the buffer

#### Return Value
Returns the size of the header value written to the buffer, or an error code.

## Return Values

The functions return 32-bit unsigned integers with the following meanings:

### Success
- `0` - Success

### Error Codes
- `0xFFFFFFFF` - Failed to read URL from WASM memory
- `0xFFFFFFFE` - URL not allowed
- `0xFFFFFFFD` - Failed to create HTTP request
- `0xFFFFFFFC` - Failed to make HTTP request
- `0xFFFFFFFB` - Failed to read response body
- `0xFFFFFFF0` - Failed to read HTTP method from WASM memory
- `0xFFFFFFF1` - Failed to read HTTP body from WASM memory
- `0xFFFFFFF2` - Failed to read HTTP headers from WASM memory
- `0xFFFFFFF3` - Failed to parse HTTP headers JSON
- `0xFFFFFFF4` - No response available
- `0xFFFFFFF5` - Buffer too small for response data
- `0xFFFFFFF6` - Failed to write response data to memory
- `0xFFFFFFF7` - Failed to read header name from memory

## Interface: `execute_target` (Target Execution)

This interface allows WASM modules to trigger workflows or call agents directly without making HTTP requests.

### Function Signature
```go
//go:wasmimport env execute_target
func execute_target(targetTypePtr, targetTypeSize, targetIDPtr, targetIDSize, paramsPtr, paramsSize uintptr) uintptr
```

### Parameters
- `targetTypePtr` - Pointer to the target type string ("workflow" or "agent")
- `targetTypeSize` - Size of the target type string
- `targetIDPtr` - Pointer to the target ID or name string
- `targetIDSize` - Size of the target ID or name string
- `paramsPtr` - Pointer to the parameters JSON string (can be null/empty)
- `paramsSize` - Size of the parameters JSON string (can be 0)

### Example Usage
```go
// Triggering a workflow by ID
targetType := "workflow"
workflowID := "workflow-123"
params := `{"input": "data"}`
result := execute_target(
    uintptr(unsafe.Pointer(&[]byte(targetType)[0])), uintptr(len(targetType)),
    uintptr(unsafe.Pointer(&[]byte(workflowID)[0])), uintptr(len(workflowID)),
    uintptr(unsafe.Pointer(&[]byte(params)[0])), uintptr(len(params))
)

// Calling an agent by name
targetType := "agent"
agentName := "text-processor"
params := `{"messages": [{"role": "user", "content": "Hello"}]}`
result := execute_target(
    uintptr(unsafe.Pointer(&[]byte(targetType)[0])), uintptr(len(targetType)),
    uintptr(unsafe.Pointer(&[]byte(agentName)[0])), uintptr(len(agentName)),
    uintptr(unsafe.Pointer(&[]byte(params)[0])), uintptr(len(params))
)
```

## Response Handling Functions for Target Execution

After calling `execute_target`, you can use the following functions to retrieve the result:

### Function: `get_last_operation_result`

Retrieves the result of the last target execution operation.

#### Function Signature
```go
//go:wasmimport env get_last_operation_result
func get_last_operation_result(bufferPtr, bufferSize uintptr) uintptr
```

#### Parameters
- `bufferPtr` - Pointer to the buffer where the result will be written (0 to just get length)
- `bufferSize` - Size of the buffer

#### Return Value
Returns the size of the result written to the buffer, or an error code.

### Function: `get_last_operation_status`

Gets the status code of the last target execution operation.

#### Function Signature
```go
//go:wasmimport env get_last_operation_status
func get_last_operation_status() int32
```

#### Return Value
Returns the status code of the last operation (0 for success, non-zero for errors), or -1 if no status available.

## Return Values for Target Execution

The `execute_target` function returns 32-bit unsigned integers with the following meanings:

### Success
- `0` - Success

### Error Codes
- `0xFFFFFFF0` - Failed to read target type from WASM memory
- `0xFFFFFFF1` - Failed to read target ID from WASM memory
- `0xFFFFFFF2` - Failed to read params from WASM memory
- `0xFFFFFFF3` - Failed to parse params JSON
- `0xFFFFFFF4` - Invalid target type
- `0xFFFFFFF5` - Failed to execute target

The `get_last_operation_result` function returns:
- `0xFFFFFFF0` - No operation result available
- `0xFFFFFFF1` - Buffer too small for result data
- `0xFFFFFFF2` - Failed to write result to WASM memory
- Length of result data (success)

The `get_last_operation_status` function returns:
- `-1` - No operation status available
- Status code (0 for success, non-zero for errors)

## Examples

See the following files for complete examples:
- `examples/wasm/execute-target/main.go` - Uses the `execute_target` interface