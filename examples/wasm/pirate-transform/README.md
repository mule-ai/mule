# Pirate Transform WASM Module

This WASM module passes through the input text and appends a request to translate it to pirate speak.

## What it does

- Takes input text from the workflow
- Passes it through unchanged
- Appends: "\nSay this in pirate speak"
- Outputs as JSON for the next workflow step

## Example

**Input:**
```json
{"prompt": "Hello, how are you today?"}
```

**Output:**
```json
{"message": "Hello, how are you today?\nSay this in pirate speak"}
```

This output would then be passed to an AI agent which would translate it to pirate speak like:
"Ahoy, how be ye today?"

## Building

### Using standard Go

```bash
go build -o pirate-transform.wasm main.go
```

### Using TinyGo (recommended for smaller binaries)

```bash
tinygo build -o pirate-transform.wasm -target wasm main.go
```

## Testing locally

```bash
# Test with input
echo '{"prompt": "Hello, how are you today?"}' | wasmtime pirate-transform.wasm
# Output: {"message":"Hello, how are you today?\nSay this in pirate speak"}

# Test with no input
echo '{}' | wasmtime pirate-transform.wasm
# Output: {"message":"Arrr, I need something to say!\nSay this in pirate speak"}

# Test with empty input
echo '' | wasmtime pirate-transform.wasm
# Output: {"message":"Arrr, I need something to say!\nSay this in pirate speak"}
```

## Using in Mule

### 1. Build the WASM module

```bash
cd examples/wasm/pirate-transform
make build-tinygo
```

### 2. Upload to Mule

```bash
curl -X POST http://localhost:8080/api/v1/wasm-modules \
  -H "Content-Type: application/json" \
  -d @upload-payload.json
```

Note the returned module ID.

### 3. Create a workflow

Create a workflow that uses the pirate transform module followed by an AI agent:

```bash
curl -X POST http://localhost:8080/api/v1/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Pirate Speak Workflow",
    "description": "Transform text to pirate speak",
    "steps": [
      {
        "step_order": 1,
        "step_type": "wasm_module",
        "wasm_module_id": "<your-pirate-module-id>",
        "description": "Add pirate instruction"
      },
      {
        "step_order": 2,
        "step_type": "agent",
        "agent_id": "<your-agent-id>",
        "description": "Translate to pirate speak"
      }
    ]
  }'
```

### 4. Execute the workflow

```bash
curl -X POST http://localhost:8080/api/v1/workflows/<workflow-id>/execute \
  -H "Content-Type: application/json" \
  -d '{"prompt": "Hello, how are you today?"}'
```

The workflow will:
1. Take your input: "Hello, how are you today?"
2. WASM module transforms it to: "Hello, how are you today?\nSay this in pirate speak"
3. AI agent translates to: "Ahoy, how be ye today?"

## Files

- `main.go` - WASM module source code
- `Makefile` - Build configuration
- `upload-payload.json` - Example payload for uploading the module
- `workflow.json` - Example workflow configuration