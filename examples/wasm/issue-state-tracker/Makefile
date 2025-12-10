# Makefile for building the issue-state-tracker WASM module

# Default target
.PHONY: build clean

# Build the WASM module
build:
	@echo "Building issue-state-tracker WASM module..."
	GOOS=wasip1 GOARCH=wasm go build -o issue_state_tracker.wasm main.go
	@echo "Build complete: issue_state_tracker.wasm"

# Clean build artifacts
clean:
	rm -f issue_state_tracker.wasm

# Install dependencies (if any)
deps:
	@echo "No dependencies to install"

# Run tests (if any)
test:
	@echo "No tests to run"