# Makefile for http-request-with-headers WASM module

.PHONY: build test clean

# Default target
all: build

# Build the WASM module
build:
	@echo "Building http-request-with-headers WASM module..."
	GOOS=wasip1 GOARCH=wasm go build -o http-request-with-headers.wasm .
	@echo "Build successful!"

# Test the module locally (requires wasmtime)
test: build
	@echo "Testing http-request-with-headers WASM module..."
	cat test-input.json | wasmtime --mapdir=/tmp::$$PWD http-request-with-headers.wasm
	@echo "Test completed."

# Clean build artifacts
clean:
	rm -f http-request-with-headers.wasm
	@echo "Cleaned build artifacts."