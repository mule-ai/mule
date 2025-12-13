# Makefile for building the worktree-name-generator WASM module

# Build the WASM module
build:
	GOOS=wasip1 GOARCH=wasm go build -o main.wasm .

# Clean build artifacts
clean:
	rm -f main.wasm

# Install dependencies (if any)
deps:
	go mod tidy

# Run tests (if any)
test:
	go test ./...

.PHONY: build clean deps test