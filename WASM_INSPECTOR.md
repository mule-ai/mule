# WASM Inspector

A simple Go tool to inspect WebAssembly (WASM) files and list their imports, particularly looking for imports from the "env" module. This tool helps diagnose the "module[env] not instantiated" error by verifying what the WASM module is actually trying to import.

## Usage

```bash
# Simple inspection showing only imports
go run wasm_inspector.go <wasm_file>

# Detailed inspection showing imports, exports, and memory info
go run wasm_detailed_inspector.go <wasm_file>
```

## Example Output

For a WASM file with env imports:
```
Inspecting WASM file: example.wasm
File size: 1675318 bytes

Found 11 imported functions:
1. Module: wasi_snapshot_preview1, Function: sched_yield
2. Module: wasi_snapshot_preview1, Function: proc_exit
3. Module: wasi_snapshot_preview1, Function: args_get
4. Module: wasi_snapshot_preview1, Function: args_sizes_get
5. Module: wasi_snapshot_preview1, Function: clock_time_get
6. Module: wasi_snapshot_preview1, Function: environ_get
7. Module: wasi_snapshot_preview1, Function: environ_sizes_get
8. Module: wasi_snapshot_preview1, Function: fd_write
9. Module: wasi_snapshot_preview1, Function: random_get
10. Module: wasi_snapshot_preview1, Function: poll_oneoff
11. Module: env, Function: log_message

Summary:
- Total imports: 11
- Imports from 'env' module: 1

Warning: This module imports functions from the 'env' module.
Make sure these functions are properly provided during execution.
```

## Diagnosing "module[env] not instantiated" Errors

When you encounter a "module[env] not instantiated" error:

1. Use this tool to inspect your WASM file and identify what functions it imports from the "env" module
2. Ensure that all imported functions are properly provided when instantiating the WASM module
3. Check that the host environment registers all required functions in the "env" module namespace

## Compiling Go to WASM for Testing

To compile a Go program to WASM format for testing:

```bash
GOOS=wasip1 GOARCH=wasm go build -o output.wasm input.go
```

To import a function from the "env" module in Go:

```go
//go:wasmimport env function_name
func functionName(param1 type1, param2 type2) returnType
```