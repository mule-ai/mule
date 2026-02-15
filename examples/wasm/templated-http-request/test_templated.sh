#!/bin/bash

# Test the templated-http-request module
echo "Testing templated-http-request module..."

# Create test input with templating
cat > test_input.json << EOF
{
  "url": "https://httpbin.org/post",
  "method": "POST",
  "prompt": "Hello, this is a test message with templating!",
  "headers": {
    "User-Agent": "Mule-WASM-Test/1.0",
    "Content-Type": "application/json"
  },
  "data": {
    "message": "{{.MESSAGE}}",
    "timestamp": "2023-01-01T00:00:00Z",
    "source": "mule-wasm-templated-http-request"
  }
}
EOF

# Run the WASM module
echo "Running WASM module with input:"
cat test_input.json
echo ""
echo "Output:"
GOOS=wasip1 GOARCH=wasm go run simple.go < test_input.json