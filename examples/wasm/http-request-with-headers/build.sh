#!/bin/bash

# Build script for the http-request-with-headers WASM module

echo "Building http-request-with-headers WASM module..."

# Compile to WASM
GOOS=wasip1 GOARCH=wasm go build -o http-request-with-headers.wasm .

if [ $? -eq 0 ]; then
    echo "Build successful!"
    echo "WASM module created: http-request-with-headers.wasm"
    
    # Show file size
    ls -lh http-request-with-headers.wasm
    
    echo ""
    echo "To test the module, you can run:"
    echo "  cat test-input.json | wasmtime --mapdir=/tmp::$PWD http-request-with-headers.wasm"
    echo ""
    echo "Or upload to Mule and execute with the test input."
else
    echo "Build failed!"
    exit 1
fi