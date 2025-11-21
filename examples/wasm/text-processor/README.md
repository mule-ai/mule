# Text Processor WASM Module Example

This example demonstrates a WASM module that reads input from stdin, processes it, and outputs JSON to stdout.

## What it does

- Reads JSON input from stdin containing a "prompt" field
- Converts the prompt text to uppercase
- Outputs JSON with a "message" field containing the processed text

## Building

### Using standard Go

```bash
go build -o text-processor.wasm main.go
```

### Using TinyGo (recommended for smaller binaries)

```bash
tinygo build -o text-processor.wasm -target wasm main.go
```

## Testing locally

```bash
# Test with input
echo '{"prompt": "hello world"}' | wasmtime text-processor.wasm
# Output: {"message":"HELLO WORLD"}

# Test with no input
echo '{}' | wasmtime text-processor.wasm
# Output: {"message":"NO INPUT PROVIDED"}

# Test with empty input
echo '' | wasmtime text-processor.wasm
# Output: {"message":"NO INPUT PROVIDED"}
```

## Using in Mule

1. Build the WASM module:
   ```bash
   make build
   ```

2. Upload to Mule:
   ```bash
   curl -X POST http://localhost:8080/api/v1/wasm-modules \
     -H "Content-Type: application/json" \
     -d @upload-payload.json
   ```

3. Create a workflow using the module:
   ```bash
   curl -X POST http://localhost:8080/api/v1/workflows \
     -H "Content-Type: application/json" \
     -d @workflow.json
   ```

4. Execute the workflow:
   ```bash
   curl -X POST http://localhost:8080/api/v1/workflows/your-workflow-id/execute \
     -H "Content-Type: application/json" \
     -d '{"prompt": "process this text"}'
   ```

## Files

- `main.go` - The WASM module source code
- `Makefile` - Build configuration
- `upload-payload.json` - Example payload for uploading the module
- `workflow.json` - Example workflow configuration