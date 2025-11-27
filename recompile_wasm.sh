#!/bin/bash

# Script to recompile all WASM modules in the examples directory

echo "Recompiling all WASM modules..."

# Recompile the main WASM modules
echo "Compiling http_methods_demo.go..."
cd /data/jbutler/git/mule-ai/mule/examples/wasm
GOOS=wasip1 GOARCH=wasm go build -o http_methods_demo.wasm http_methods_demo.go

echo "Compiling network_demo.go..."
GOOS=wasip1 GOARCH=wasm go build -o network_demo.wasm network_demo.go

echo "Compiling network_example.go..."
GOOS=wasip1 GOARCH=wasm go build -o network_example.wasm network_example.go

# Recompile the http-request directory modules
echo "Compiling http-request/simple.go..."
cd /data/jbutler/git/mule-ai/mule/examples/wasm/http-request
GOOS=wasip1 GOARCH=wasm go build -o simple.wasm simple.go

# Copy the simple.wasm to http-request.wasm to maintain consistency
echo "Copying simple.wasm to http-request.wasm..."
cp simple.wasm http-request.wasm

echo "All WASM modules recompiled successfully!"