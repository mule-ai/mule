# Makefile for building the git worktree WASM module

# Build the WASM module
build:
	GOOS=wasip1 GOARCH=wasm go build -o main.wasm -tags wasm

# Clean build artifacts
clean:
	rm -f main.wasm

# Default target
all: build

.PHONY: build clean all