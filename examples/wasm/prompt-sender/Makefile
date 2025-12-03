# Makefile for prompt-sender WASM module

MODULE_NAME=prompt_sender
WASM_FILE=$(MODULE_NAME).wasm

.PHONY: build test server clean

build: $(WASM_FILE)

$(WASM_FILE): cmd/main.go
	GOOS=wasip1 GOARCH=wasm go build -o $(WASM_FILE) cmd/main.go

test: build
	cat test_input.json | wasmtime --mapdir=/tmp::$$PWD $(WASM_FILE)

server:
	go run test_server.go

clean:
	rm -f $(WASM_FILE)

install-deps:
	# Install wasmtime if not already installed
	which wasmtime || curl https://wasmtime.dev/install.sh -sSf | bash

help:
	@echo "Available targets:"
	@echo "  build    - Build the WASM module"
	@echo "  test     - Test the WASM module with sample input"
	@echo "  server   - Run a test server to receive messages"
	@echo "  clean    - Remove the compiled WASM file"
	@echo "  install-deps - Install wasmtime (if not already installed)"