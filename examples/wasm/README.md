# WASM Module Examples

This directory contains various examples of WASM modules that can be used with Mule AI.

## Available Examples

1. [text-processor](text-processor/) - Simple text processing example that converts text to uppercase
2. [advanced-processor](advanced-processor/) - More advanced text processing with configurable operations
3. [jq-filter](jq-filter/) - Applies jq filters to JSON data (**NEW**)
4. [pirate-transform](pirate-transform/) - Transforms text to pirate speak
5. [prompt-sender](prompt-sender/) - Sends prompts to AI agents
6. [http-request](http-request/) - Makes HTTP requests
7. [http-request-with-headers](http-request-with-headers/) - Makes HTTP requests with custom headers
8. [github-issues](github-issues/) - Interacts with GitHub Issues API
9. [issues-to-markdown](issues-to-markdown/) - Converts GitHub issues to formatted markdown
10. [execute-target](execute-target/) - Executes targets in workflows
11. [run-default-workflow](run-default-workflow/) - Runs a default workflow
12. [array-workflow-launcher](array-workflow-launcher/) - Processes JSON arrays and launches multiple workflows in parallel

## Getting Started

Each example contains:

- `main.go` - The WASM module source code
- `Makefile` - Build and test instructions
- `README.md` - Detailed documentation for that specific example
- `upload-payload.json` - Example payload for uploading the module to Mule
- `workflow.json` - Example workflow configuration

## Building WASM Modules

Most examples can be built using either the standard Go compiler or TinyGo:

### Using standard Go

```bash
GOOS=wasip1 GOARCH=wasm go build -o module.wasm main.go
```

### Using TinyGo (for smaller binaries)

```bash
tinygo build -o module.wasm -target wasi main.go
```

## Testing Locally

Many examples include a `test` target in their Makefile:

```bash
make test
```

This requires `wasmtime` to be installed on your system.

## Using in Mule

1. Build the WASM module
2. Encode the binary in base64
3. Upload to Mule using the API
4. Create a workflow that uses the module
5. Execute the workflow

See individual example directories for specific instructions.