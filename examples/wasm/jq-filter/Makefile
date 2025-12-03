.PHONY: build build-tinygo test clean

# Build using standard Go
build:
	GOOS=wasip1 GOARCH=wasm go build -o jq-filter.wasm main.go

# Build using TinyGo (smaller binary)
build-tinygo:
	tinygo build -o jq-filter.wasm -target wasi main.go

# Test the WASM module locally
test:
	@echo "Testing with simple object and query..."
	echo '{"prompt": "{\"name\":\"John\",\"age\":30}", "query": ".name"}' | wasmtime jq-filter.wasm
	@echo ""
	@echo "Testing with array and query..."
	echo '{"prompt": "[{\"name\":\"John\",\"age\":30},{\"name\":\"Jane\",\"age\":25}]", "query": ".[].name"}' | wasmtime jq-filter.wasm
	@echo ""
	@echo "Testing with complex query..."
	echo '{"prompt": "{\"users\":[{\"name\":\"John\",\"scores\":[10,20,30]},{\"name\":\"Jane\",\"scores\":[15,25,35]}]}", "query": ".users[].scores | add"}' | wasmtime jq-filter.wasm
	@echo ""
	@echo "Testing with no input..."
	echo '{}' | wasmtime jq-filter.wasm
	@echo ""

# Clean build artifacts
clean:
	rm -f jq-filter.wasm

# Build and test
all: build-tinygo test