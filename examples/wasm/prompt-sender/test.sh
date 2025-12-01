#!/bin/bash

# Test script for the prompt sender WASM module

# Check if wasm file exists
if [ ! -f "prompt_sender.wasm" ]; then
    echo "Compiling WASM module..."
    GOOS=wasip1 GOARCH=wasm go build -o prompt_sender.wasm main.go
fi

echo "Testing prompt sender WASM module..."
echo "Input:"
cat test_input.json
echo ""
echo "Output:"
cat test_input.json | wasmtime --mapdir=/tmp::$PWD prompt_sender.wasm