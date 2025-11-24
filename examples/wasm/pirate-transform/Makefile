.PHONY: build build-tinygo test clean

# Build using standard Go
build:
	GOOS=js GOARCH=wasm go build -o pirate-transform.wasm main.go

# Build using TinyGo (recommended for smaller binaries)
build-tinygo:
	tinygo build -o pirate-transform.wasm -target wasm main.go

# Test the WASM module locally
test:
	@echo "Testing with normal input..."
	echo '{"prompt": "Hello, how are you today?"}' | wasmtime pirate-transform.wasm
	@echo ""
	@echo "Testing with no input..."
	echo '{}' | wasmtime pirate-transform.wasm
	@echo ""
	@echo "Testing with empty input..."
	echo '' | wasmtime pirate-transform.wasm

# Clean build artifacts
clean:
	rm -f pirate-transform.wasm

# Build and test
all: build-tinygo test