#!/bin/bash

# Build script for the array-workflow-launcher WASM module

# Check if we're in the right directory
if [ ! -f "main.go" ]; then
    echo "Error: main.go not found. Please run this script from the array-workflow-launcher directory."
    exit 1
fi

# Create go.mod if it doesn't exist
if [ ! -f "go.mod" ]; then
    echo "module github.com/mule-ai/mule/examples/wasm/array-workflow-launcher" > go.mod
    echo "go 1.25.4" >> go.mod
fi

# Build the WASM module
echo "Building WASM module..."
GOOS=wasip1 GOARCH=wasm go build -o array-workflow-launcher.wasm main.go

if [ $? -eq 0 ]; then
    echo "Successfully built array-workflow-launcher.wasm"
    ls -lh array-workflow-launcher.wasm
else
    echo "Error: Failed to build WASM module"
    exit 1
fi