# Makefile for issues-to-markdown WASM module

MODULE_NAME = issues-to-markdown
WASM_FILE = $(MODULE_NAME).wasm

# Default target
all: build

# Build the WASM module using Go compiler
build:
	GOOS=wasip1 GOARCH=wasm go build -o $(WASM_FILE) main.go

# Build the WASM module using TinyGo (smaller binary)
build-tinygo:
	tinygo build -o $(WASM_FILE) -target wasi main.go

# Test the module locally with wasmtime
test: build
	wasmtime $(WASM_FILE) < test-input.json

# Clean build artifacts
clean:
	rm -f $(WASM_FILE)

# Install wasmtime if not available
install-wasmtime:
ifeq ($(shell which wasmtime),)
	curl https://wasmtime.dev/install.sh -sSf | bash
endif

# Help
help:
	@echo "Available targets:"
	@echo "  build         - Build the WASM module using Go compiler"
	@echo "  build-tinygo  - Build the WASM module using TinyGo"
	@echo "  test          - Test the module with sample input"
	@echo "  clean         - Clean build artifacts"
	@echo "  install-wasmtime - Install wasmtime if not available"
	@echo "  help          - Show this help message"

.PHONY: all build build-tinygo test clean install-wasmtime help