.PHONY: build test clean

WASM_FILE=github-comment.wasm

build:
	GOOS=wasip1 GOARCH=wasm go build -o $(WASM_FILE) main.go

test: build
	@echo "Testing with sample payload..."
	@echo "Note: This requires wasmtime to be installed"
	echo '{"prompt":"{\"issue\": \"https://api.github.com/repos/octocat/Hello-World/issues/1\", \"comment\": \"Test comment\"}","token":"test-token"}' | wasmtime $(WASM_FILE)

clean:
	rm -f $(WASM_FILE)