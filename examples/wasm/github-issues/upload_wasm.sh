#!/bin/bash

# Upload the GitHub issues fetcher WASM module to the Mule system
# Usage: ./upload_wasm.sh [wasm_file]

WASM_FILE=${1:-github_issues.wasm}
CONFIG_FILE="github_issues_config.json"
API_URL="http://localhost:8140/api/v1/wasm-modules"

# Check if files exist
if [ ! -f "$WASM_FILE" ]; then
    echo "Error: WASM file '$WASM_FILE' not found"
    exit 1
fi

if [ ! -f "$CONFIG_FILE" ]; then
    echo "Error: Config file '$CONFIG_FILE' not found"
    exit 1
fi

# Upload using curl with multipart form data
echo "Uploading WASM module..."
response=$(curl -s -w "%{http_code}" -X POST "$API_URL" \
    -F "name=github-issues-fetcher" \
    -F "description=Fetches issues from a GitHub repository" \
    -F "config=@$CONFIG_FILE" \
    -F "module_data=@$WASM_FILE")

# Extract HTTP status code (last 3 characters)
http_code="${response: -3}"

# Extract response body (everything except last 3 characters)
response_body="${response%???}"

echo "HTTP Status: $http_code"
echo "Response:"
echo "$response_body" | jq '.' 2>/dev/null || echo "$response_body"

if [ "$http_code" -eq 201 ]; then
    echo "✅ WASM module uploaded successfully!"
else
    echo "❌ Failed to upload WASM module"
    exit 1
fi