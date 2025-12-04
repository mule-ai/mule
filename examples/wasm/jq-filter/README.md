# JQ Filter WASM Module Example

This example demonstrates a WASM module that applies jq filters to JSON data. It takes JSON input and a jq query expression, then returns the filtered results.

## What it does

- Reads JSON input from stdin containing a "prompt" field with JSON data
- Applies a jq filter expression provided in the WASM module configuration
- Outputs the filtered results as JSON

## Prerequisites

- Go 1.24+
- wasmtime (for local testing)
- tinygo (optional, for smaller binaries)

## Building

### Using standard Go

```bash
go build -o jq-filter.wasm main.go
```

### Using TinyGo (recommended for smaller binaries)

```bash
tinygo build -o jq-filter.wasm -target wasi main.go
```

## Testing locally

```bash
# Test with simple object and query
echo '{"prompt": "{\"name\":\"John\",\"age\":30}", "query": ".name"}' | wasmtime jq-filter.wasm
# Output: {"result":"John","success":true}

# Test with array and query
echo '{"prompt": "[{\"name\":\"John\",\"age\":30},{\"name\":\"Jane\",\"age\":25}]", "query": ".[].name"}' | wasmtime jq-filter.wasm
# Output: {"result":["John","Jane"],"success":true}

# Test with complex query
echo '{"prompt": "{\"users\":[{\"name\":\"John\",\"scores\":[10,20,30]},{\"name\":\"Jane\",\"scores\":[15,25,35]}]}", "query": ".users[].scores | add"}' | wasmtime jq-filter.wasm
# Output: {"result":[60,75],"success":true}

# Test with no input
echo '{}' | wasmtime jq-filter.wasm
# Output: {"success":false,"error":"No jq query provided. Please provide a 'query' in the WASM module configuration."}
```

## Using in Mule

1. Build the WASM module:
   ```bash
   make build-tinygo
   ```

2. Upload to Mule with configuration:
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
     -d '{"prompt": "{\"users\":[{\"name\":\"John\",\"scores\":[10,20,30]}]}"}'
   ```

## Configuration

The jq filter expression is provided through the WASM module configuration. When uploading the module, include a "config" field with a "query" property:

```json
{
  "name": "jq-filter",
  "description": "Applies jq filters to JSON data",
  "module_data": "<base64-encoded-content-of-jq-filter.wasm>",
  "config": {
    "query": ".name"
  }
}
```

## Examples

Here are some example jq queries you can use:

1. Extract a field: `.name`
2. Extract nested fields: `.user.profile.email`
3. Array operations: `.items[]`
4. Array filtering: `.items[] | select(.price > 100)`
5. Aggregation: `[.items[].price] | add`
6. Transformation: `{name: .firstName, age: .userAge}`

## Files

- `main.go` - The WASM module source code
- `Makefile` - Build configuration
- `upload-payload.json` - Example payload for uploading the module
- `workflow.json` - Example workflow configuration