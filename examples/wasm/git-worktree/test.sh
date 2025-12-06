#!/bin/bash

# Test script for the git worktree WASM module

# Check if we're in a git repository
if [ ! -d ".git" ]; then
    echo "Error: This script must be run from within a git repository"
    exit 1
fi

# Build the WASM module
echo "Building WASM module..."
make build

if [ ! -f "main.wasm" ]; then
    echo "Error: Failed to build WASM module"
    exit 1
fi

echo "WASM module built successfully!"

# Show usage example
echo
echo "To use this module in Mule AI:"
echo "1. Upload main.wasm through the API or CLI"
echo "2. Create a workflow step that uses this module"
echo "3. Provide input like:"
echo '   {'
echo '     "prompt": "{\"worktree_name\": \"test-feature\"}"'
echo '   }'
echo
echo "The module will create a worktree and set the working directory."