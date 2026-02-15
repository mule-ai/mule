#!/bin/bash

# Test script for the http-request-with-headers WASM module

echo "Testing http-request-with-headers WASM module..."

# Check if wasmtime is available
if ! command -v wasmtime &> /dev/null
then
    echo "wasmtime could not be found. Please install it to run this test."
    echo "You can install it with: curl https://wasmtime.dev/install.sh -sSf | bash"
    exit 1
fi

# Check if the WASM module exists
if [ ! -f "http-request-with-headers.wasm" ]; then
    echo "WASM module not found. Building it first..."
    ./build.sh
    if [ $? -ne 0 ]; then
        echo "Failed to build the WASM module."
        exit 1
    fi
fi

echo "Running the module with test input..."
echo ""

# Run the module with test input
cat test-input.json | wasmtime --mapdir=/tmp::$PWD http-request-with-headers.wasm

echo ""
echo "Test completed."