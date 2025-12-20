#!/bin/bash

# Test script for git-branch-push WASM module

echo "Building the git-branch-push WASM module..."

# Clean up any existing build
rm -f main.wasm

# Build the WASM module
make build

if [ $? -ne 0 ]; then
    echo "Failed to build the WASM module"
    exit 1
fi

echo "WASM module built successfully"

# Test with wasmtime if available
if command -v wasmtime &> /dev/null; then
    echo "Testing with wasmtime..."
    
    # Test with explicit branch name
    echo "Test 1: Using explicit branch name"
    echo '{"prompt": "{\"branch_name\": \"test-branch\", \"remote_name\": \"origin\"}", "token": "test-token"}' | wasmtime run main.wasm
    
    echo ""
    
    # Test with automatic branch name (this would normally get the worktree name)
    echo "Test 2: Using automatic branch name (would use worktree name in actual usage)"
    echo '{"token": "test-token"}' | wasmtime run main.wasm
else
    echo "wasmtime not found, skipping runtime tests"
fi

echo "Test script completed"