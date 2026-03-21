# Advanced Processor WASM Module Example

This example demonstrates a more advanced WASM module with configurable operations for text processing.

## What it does

- Reads JSON input from stdin containing a "prompt" field and optional "options"
- Supports multiple operations: uppercase, lowercase, reverse, count
- Outputs JSON with the processed text and metadata

## Input Format

```json
{
  "prompt": "Hello World",
  "options": {
    "operation": "uppercase"
  }
}
```

### Available Operations

- `uppercase` - Convert text to uppercase (default)
- `lowercase` - Convert text to lowercase
- `reverse` - Reverse the text
- `count` - Return character count

## Building

```bash
GOOS=wasip1 GOARCH=wasm go build -o advanced-processor.wasm main.go
```

Or with TinyGo:

```bash
tinygo build -o advanced-processor.wasm -target wasm main.go
```

## Testing

```bash
# Test uppercase (default)
echo '{"prompt": "hello"}' | wasmtime advanced-processor.wasm
# Output: {"Message":"HELLO","ProcessedAt":"...","Metadata":null,"Options":null}

# Test with operation
echo '{"prompt": "hello", "options": {"operation": "reverse"}}' | wasmtime advanced-processor.wasm
```

## Files

- `main.go` - The WASM module source code
