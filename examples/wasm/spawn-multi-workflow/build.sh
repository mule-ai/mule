#!/bin/bash

# Build script for the spawn-multi-workflow WASM module

# Create a separate go.mod file for the WASM module
echo "module github.com/mule-ai/mule/examples/wasm/spawn-multi-workflow" > go.mod
echo "go 1.25.4" >> go.mod

# Build only the main.go file to avoid including other examples
echo "Building spawn-multi-workflow.wasm..."
GOOS=wasip1 GOARCH=wasm go build -o spawn-multi-workflow.wasm main.go

if [ $? -eq 0 ]; then
    echo "Build successful! Created spawn-multi-workflow.wasm"
else
    echo "Build failed!"
    exit 1
fi
