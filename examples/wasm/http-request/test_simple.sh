#!/bin/bash

# Test the simple.wasm module with headers
echo "Testing simple.wasm with headers..."

# Create test input
cat > test_input.json << EOF
{
  "url": "https://httpbin.org/get",
  "method": "GET",
  "headers": {
    "User-Agent": "Mule-WASM-Test/1.0",
    "X-Test-Header": "test-value"
  }
}
EOF

# Run the WASM module
echo "Running WASM module with input:"
cat test_input.json
echo ""
echo "Output:"
GOOS=wasip1 GOARCH=wasm go run simple.go < test_input.json