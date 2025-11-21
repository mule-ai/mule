.PHONY: build build-tinygo test clean

# Build using standard Go
build:
	GOOS=js GOARCH=wasm go build -o text-processor.wasm main.go

# Build using TinyGo (smaller binary)
build-tinygo:
	tinygo build -o text-processor.wasm -target wasm main.go

# Test the WASM module locally
test:
	@echo "Testing with input..."
	echo '{"prompt": "hello world"}' | wasmtime text-processor.wasm
	@echo ""
	@echo "Testing with no input..."
	echo '{}' | wasmtime text-processor.wasm
	@echo ""
	@echo "Testing with empty input..."
	echo '' | wasmtime text-processor.wasm

# Clean build artifacts
clean:
	rm -f text-processor.wasm

# Build and test
all: build-tinygo test