#!/bin/bash

# Build script for the workflow-aggregator WASM module

# Create a separate go.mod file for the WASM module
echo "module github.com/mule-ai/mule/examples/wasm/workflow-aggregator" > go.mod
echo "go 1.25.4" >> go.mod

# Build only the main.go file to avoid including other examples
echo "Building workflow-aggregator.wasm..."
GOOS=wasip1 GOARCH=wasm go build -o workflow-aggregator.wasm main.go

if [ $? -eq 0 ]; then
    echo "Build successful! Created workflow-aggregator.wasm"
else
    echo "Build failed!"
    exit 1
fi