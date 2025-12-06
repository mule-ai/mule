# Simple Makefile for building the WASM module

# Build the WASM module
build:
	GOOS=wasip1 GOARCH=wasm go build -o main.wasm .

# Clean build artifacts
clean:
	rm -f main.wasm

# Default target
all: build

.PHONY: build clean all