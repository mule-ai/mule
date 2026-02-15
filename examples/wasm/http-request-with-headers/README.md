# HTTP Request with Headers WASM Module Example

This example demonstrates how to make HTTP requests from a WASM module using the enhanced `http_request_with_headers` host function that supports passing HTTP headers, and how to retrieve and return the response data.

## Overview

The module shows how to:
1. Read input data from stdin
2. Make HTTP requests using the `http_request_with_headers` host function
3. Pass HTTP headers to the request
4. Retrieve response data using `get_last_response_body` and `get_last_response_status`
5. Parse and return the response data in JSON format
6. Handle responses and errors properly

## Host Functions

The module uses several host functions:

### `http_request_with_headers`

Makes an HTTP request with headers:

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

### `get_last_response_body`

Retrieves the body of the last HTTP response:

```go
func get_last_response_body(bufferPtr, bufferSize uintptr) uint32
```

Parameters:
- `bufferPtr` - Pointer to buffer where response body will be written
- `bufferSize` - Size of the buffer

Returns:
- Number of bytes written to buffer, or required buffer size if `bufferSize` is 0

### `get_last_response_status`

Retrieves the status code of the last HTTP response:

```go
func get_last_response_status() uint32
```

Returns:
- HTTP status code (e.g., 200, 404, 500)

## Error Codes

The `http_request_with_headers` function returns the following error codes:
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

## Usage

To compile and use this module:

1. Compile to WASM:
   ```bash
   GOOS=wasip1 GOARCH=wasm go build -o http-request-with-headers.wasm .
   ```

2. Upload to Mule and execute with input like:
   ```json
   {
     "url": "https://httpbin.org/get",
     "method": "GET",
     "headers": {
       "Authorization": "Bearer token123",
       "Custom-Header": "custom-value"
     }
   }
   ```

3. The module will make an HTTP request with the specified headers and return:
   - Success status
   - HTTP status code
   - Response data (parsed as JSON if possible, otherwise as raw text)