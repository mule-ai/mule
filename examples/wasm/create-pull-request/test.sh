#!/bin/bash

# Test script for the create pull request WASM module

set -e

echo "Building the WASM module..."
make build

echo "Testing with sample input (automatic branch detection)..."
cat > test-input-auto.json <<EOF
{
  "token": "your-github-token",
  "owner": "octocat",
  "repo": "Hello-World",
  "title": "Test pull request from WASM module (auto branch)",
  "base": "main",
  "body": "This is a test pull request created by the Mule AI WASM module with automatic branch detection.",
  "draft": false
}
EOF

echo "Testing with sample input (explicit branch)..."
cat > test-input-explicit.json <<EOF
{
  "token": "your-github-token",
  "owner": "octocat",
  "repo": "Hello-World",
  "title": "Test pull request from WASM module (explicit branch)",
  "head": "feature-branch",
  "base": "main",
  "body": "This is a test pull request created by the Mule AI WASM module with explicit branch name.",
  "draft": false
}
EOF

echo "Running the WASM module with auto-detection input..."
echo "Note: This will fail with a 401 error since we're using a dummy token."
echo "In a real scenario, you would use a valid GitHub token."

# Run the WASM module with the auto-detection input
echo "=== Testing automatic branch detection ==="
cat test-input-auto.json | wasmtime run main.wasm

echo ""
echo "Running the WASM module with explicit branch input..."
echo "=== Testing explicit branch name ==="
cat test-input-explicit.json | wasmtime run main.wasm

echo "Test completed."