#!/bin/bash

# Test script for issues-to-markdown WASM module

# Check if wasmtime is available
if ! command -v wasmtime &> /dev/null
then
    echo "wasmtime could not be found, installing..."
    make install-wasmtime
    export PATH="/root/.wasmtime/bin:$PATH"
fi

# Build the module
echo "Building WASM module..."
make build

# Test the module
echo "Testing WASM module..."
echo "==================== OUTPUT ===================="
wasmtime issues-to-markdown.wasm < test-input.json | jq -r '.message // .'
echo "================================================"

echo "Test completed successfully!"