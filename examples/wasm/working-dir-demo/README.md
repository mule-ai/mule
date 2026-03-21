# Working Directory Demo WASM Module Example

This example demonstrates how WASM modules can interact with the working directory in Mule workflows.

## What it does

- Reads input from stdin
- Gets the current working directory
- Creates files in both the current directory and a subdirectory
- Demonstrates filesystem operations available to WASM modules

## Input Format

```json
{
  "prompt": "optional prompt text"
}
```

## Building

```bash
GOOS=wasip1 GOARCH=wasm go build -o working-dir-demo.wasm main.go
```

Or with TinyGo:

```bash
tinygo build -o working-dir-demo.wasm -target wasm main.go
```

## Testing

```bash
echo '{"prompt": "create files"}' | wasmtime working-dir-demo.wasm
```

## Output Format

```json
{
  "message": "Created files in directories:\n- /path/to/test_file.txt\n- /path/to/demo_subdir/subdir_file.txt"
}
```

## Working Directory in Mule

In Mule workflows, WASM modules have access to a working directory that can be:
1. The default directory when the workflow started
2. A directory set by a previous workflow step
3. Explicitly configured in the workflow step

The working directory is automatically managed by the Mule runtime and passed to WASM modules through host functions.

## Files

- `main.go` - The WASM module source code
