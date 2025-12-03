#!/bin/bash

# This script demonstrates how to use the jq-filter WASM module with Mule

# Step 1: Build the WASM module
echo "Building jq-filter WASM module..."
cd /data/jbutler/git/mule-ai/mule/examples/wasm/jq-filter
GOOS=wasip1 GOARCH=wasm go build -o jq-filter.wasm main.go

# Step 2: Encode the WASM module in base64
echo "Encoding WASM module..."
MODULE_DATA=$(base64 -i jq-filter.wasm | tr -d '\n')

# Step 3: Create the upload payload with a sample jq query
cat > upload-payload.json <<EOF
{
  "name": "jq-filter",
  "description": "Applies jq filters to JSON data using WASM",
  "module_data": "$MODULE_DATA",
  "config": {
    "query": ".name"
  }
}
EOF

echo "Created upload-payload.json with base64-encoded WASM module"

# Step 4: Example curl command to upload the module (uncomment to use)
# curl -X POST http://localhost:8080/api/v1/wasm-modules \
#   -H "Content-Type: application/json" \
#   -d @upload-payload.json

echo "Example workflow input:"
echo '{"prompt": "{\"name\":\"John\",\"age\":30,\"city\":\"New York\"}}'

echo ""
echo "To use this module:"
echo "1. Start your Mule server"
echo "2. Upload the module using the upload-payload.json file"
echo "3. Create a workflow using the workflow.json template"
echo "4. Execute the workflow with JSON data in the prompt field"