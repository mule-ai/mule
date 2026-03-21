# Templated HTTP Request WASM Module Example

This example demonstrates using templates for HTTP requests in a WASM module.

## What it does

- Reads JSON input with template parameters
- Substitutes values into HTTP request templates
- Makes HTTP requests using the filled-in templates

## Input Format

```json
{
  "template": {
    "url": "https://api.example.com/users/{user_id}/posts/{post_id}",
    "method": "GET"
  },
  "params": {
    "user_id": "123",
    "post_id": "456"
  },
  "headers": {
    "Authorization": "Bearer token123"
  }
}
```

## Building

```bash
GOOS=wasip1 GOARCH=wasm go build -o templated-http-request.wasm main.go
```

## Testing

```bash
echo '{"template": {"url": "https://httpbin.org/get"}, "params": {}, "headers": {}}' | wasmtime templated-http-request.wasm
```

## Host Functions Used

- `http_request_with_headers` - For making HTTP requests with headers
- `get_last_response_body` - For retrieving the response body
- `get_last_response_status` - For retrieving the status code

## Files

- `main.go` - The WASM module source code
