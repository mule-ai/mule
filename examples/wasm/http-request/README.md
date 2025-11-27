# HTTP Request WASM Module Example

This example demonstrates how to make HTTP requests from a WASM module using the host functions provided by the Mule runtime.

## Overview

The module shows how to:
1. Read input data from stdin
2. Make HTTP requests using the `http_request` host function
3. Handle responses and errors properly
4. Output results in JSON format

## Host Function Interface

The module uses the `http_request` host function which has the following signature:

```go
func http_request(methodPtr, methodSize, urlPtr, urlSize, bodyPtr, bodySize uintptr) uintptr
```

There is also an enhanced version `http_request_with_headers` that supports passing HTTP headers:

```go
func http_request_with_headers(methodPtr, methodSize, urlPtr, urlSize, bodyPtr, bodySize, headersPtr, headersSize uintptr) uintptr
```

Parameters:
- `methodPtr` - Pointer to the HTTP method string (GET, POST, PUT, DELETE, etc.)
- `methodSize` - Size of the HTTP method string
- `urlPtr` - Pointer to the URL string
- `urlSize` - Size of the URL string
- `bodyPtr` - Pointer to the request body string (can be null/empty)
- `bodySize` - Size of the request body string (can be 0)
- `headersPtr` - Pointer to the headers JSON string (can be null/empty)
- `headersSize` - Size of the headers JSON string (can be 0)

Returns:
- `errorCode`: Error code (0 for success, non-zero for errors)

## Error Codes

The host function returns the following error codes:
- `0x00000000`: Success
- `0xFFFFFFFF`: Failed to read URL from memory
- `0xFFFFFFFE`: URL not allowed
- `0xFFFFFFFD`: Failed to create HTTP request
- `0xFFFFFFFC`: Failed to make HTTP request
- `0xFFFFFFFB`: Failed to read response body
- `0xFFFFFFF0`: Failed to read HTTP method from memory
- `0xFFFFFFF1`: Failed to read HTTP body from memory
- `0xFFFFFFF2`: Failed to read HTTP headers from memory
- `0xFFFFFFF3`: Failed to parse HTTP headers JSON
- `0xFFFFFFF4`: No response available
- `0xFFFFFFF5`: Buffer too small for response data
- `0xFFFFFFF6`: Failed to write response data to memory
- `0xFFFFFFF7`: Failed to read header name from memory

## Current Limitations

In this proof-of-concept implementation:
1. The WASM module needs to allocate sufficient buffer space to receive response data
2. A full implementation would include more comprehensive response handling functions
3. Headers can now be passed using the enhanced `http_request_with_headers` function

## Usage

To compile and use this module:

1. Compile to WASM:
   ```bash
   GOOS=wasip1 GOARCH=wasm go build -o http-request.wasm .
   ```

2. Upload to Mule and execute with input like:
   ```json
   {
     "url": "https://httpbin.org/get"
   }
   ```

3. The module will make an HTTP GET request and return the success status

For modules using the enhanced `http_request_with_headers` function, you can pass headers as well:
   ```json
   {
     "url": "https://httpbin.org/get",
     "headers": {
       "Authorization": "Bearer token123",
       "Custom-Header": "custom-value"
     }
   }
   ```