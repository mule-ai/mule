# WASM HTTP Interfaces in Mule

This document describes the HTTP function interface available for WASM modules in Mule.

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

## Return Values

The function returns a 32-bit unsigned integer with the following meanings:

- `0` - Success
- `0xFFFFFFFF` - Failed to read URL from WASM memory
- `0xFFFFFFFE` - URL not allowed
- `0xFFFFFFFD` - Failed to create HTTP request
- `0xFFFFFFFC` - Failed to make HTTP request
- `0xFFFFFFF0` - Failed to read HTTP method from WASM memory
- `0xFFFFFFF1` - Failed to read HTTP body from WASM memory

## Examples

See the following files for complete examples:
- `examples/wasm/http_methods_demo.go` - Uses the `http_request` interface
- `examples/wasm/http-request/simple.go` - Uses the `http_request` interface
- `examples/wasm/network_demo.go` - Uses the `http_request` interface
- `examples/wasm/network_example.go` - Uses the `http_request` interface