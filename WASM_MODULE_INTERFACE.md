# WASM Module Interface Specification

## Overview

WASM modules in Mule can now receive input data via stdin and return results via stdout. This enables WASM modules to process dynamic input and integrate seamlessly into workflows.

## Input/Output Interface

### Input (stdin)

WASM modules receive input data as JSON via stdin. The input data contains the output from the previous workflow step, or the initial workflow input for the first step.

**Input Format:**
```json
{
  "prompt": "string",
  // ... other fields from previous step
}
```

**Example Input:**
```json
{
  "prompt": "Hello, process this text"
}
```

### Output (stdout)

WASM modules should write their output to stdout as JSON. The output will be parsed and passed to the next workflow step.

**Output Format:**
```json
{
  "message": "processed output string",
  // ... other fields
}
```

**Example Output:**
```json
{
  "message": "Processed: HELLO, PROCESS THIS TEXT"
}
```

## Implementation Guide

### Go WASM Modules

Here's a complete example of a Go WASM module that reads input and produces output:

```go
package main

import (
    "encoding/json"
    "fmt"
    "io"
    "os"
    "strings"
)

// Input represents the expected input structure
// The 'prompt' field contains the text to process
type Input struct {
    Prompt string `json:"prompt"`
}

// Output represents the output structure
// The 'message' field will be passed to the next step
type Output struct {
    Message string `json:"message"`
}

func main() {
    // Read input from stdin
    inputData, err := io.ReadAll(os.Stdin)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
        os.Exit(1)
    }

    // Parse input JSON
    var input Input
    if len(inputData) > 0 {
        if err := json.Unmarshal(inputData, &input); err != nil {
            fmt.Fprintf(os.Stderr, "Error parsing input JSON: %v\n", err)
            os.Exit(1)
        }
    }

    // Process the input (example: convert to uppercase)
    processedText := strings.ToUpper(input.Prompt)

    // Create output
    output := Output{
        Message: processedText,
    }

    // Serialize output to JSON
    outputData, err := json.Marshal(output)
    if err != nil {
        fmt.Fprintf(os.Stderr, "Error serializing output: %v\n", err)
        os.Exit(1)
    }

    // Write output to stdout
    fmt.Print(string(outputData))
}
```

### Building Go WASM Modules

To compile a Go program to WASM:

```bash
# Build for WebAssembly
GOOS=js GOARCH=wasm go build -o module.wasm main.go

# Or use tinygo for smaller binaries
tinygo build -o module.wasm -target wasm main.go
```

### Rust WASM Modules

Here's an example of a Rust WASM module:

```rust
use serde::{Deserialize, Serialize};
use std::io::{self, Read};

#[derive(Deserialize)]
struct Input {
    prompt: String,
}

#[derive(Serialize)]
struct Output {
    message: String,
}

fn main() {
    // Read input from stdin
    let mut input_data = String::new();
    io::stdin().read_to_string(&mut input_data).expect("Failed to read input");

    // Parse input JSON
    let input: Input = serde_json::from_str(&input_data).expect("Failed to parse input");

    // Process the input (example: convert to uppercase)
    let processed_text = input.prompt.to_uppercase();

    // Create output
    let output = Output {
        message: processed_text,
    };

    // Serialize and print output
    let output_json = serde_json::to_string(&output).expect("Failed to serialize output");
    println!("{}", output_json);
}
```

### Building Rust WASM Modules

```bash
# Add WASM target
rustup target add wasm32-wasi

# Build for WASM
cargo build --target wasm32-wasi --release

# The output will be in target/wasm32-wasi/release/your_module_name.wasm
```

## Workflow Integration

### Creating a WASM Module

1. Upload your compiled WASM module through the API:

```bash
curl -X POST http://localhost:8080/api/v1/wasm-modules \
  -H "Content-Type: application/json" \
  -d '{
    "name": "text-processor",
    "description": "Converts text to uppercase",
    "module_data": "<base64-encoded-wasm-binary>"
  }'
```

2. Note the returned module ID for use in workflows

### Using WASM Modules in Workflows

WASM modules can be used as steps in workflows:

```json
{
  "name": "Text Processing Workflow",
  "description": "Process text through WASM module",
  "steps": [
    {
      "step_order": 1,
      "step_type": "wasm_module",
      "wasm_module_id": "your-wasm-module-id",
      "description": "Process text with WASM"
    },
    {
      "step_order": 2,
      "step_type": "agent",
      "agent_id": "your-agent-id",
      "description": "Further process with AI agent"
    }
  ]
}
```

### Input Flow

1. **First Step**: Receives the workflow input data
   ```json
   {
     "prompt": "Initial user input"
   }
   ```

2. **Subsequent Steps**: Receive output from previous step
   ```json
   {
     "prompt": "Output from previous step"
   }
   ```

## Error Handling

### Input Errors

- **Empty Input**: If no input is provided, `inputData` will be empty
- **Invalid JSON**: Modules should handle JSON parsing errors gracefully
- **Missing Fields**: Check for required fields before processing

### Output Errors

- **Invalid JSON**: Output must be valid JSON to be parsed correctly
- **Missing "message" Field**: While not required, the "message" field is the standard way to pass text to the next step
- **Empty Output**: Empty output will result in empty input for the next step

### Error Reporting

Write error messages to stderr for debugging:

```go
fmt.Fprintf(os.Stderr, "Error: %v\n", err)
```

## Best Practices

### 1. Input Validation

Always validate input before processing:

```go
if input.Prompt == "" {
    fmt.Fprintf(os.Stderr, "Warning: empty prompt received\n")
    input.Prompt = "default value"
}
```

### 2. Error Handling

Handle all errors and write to stderr:

```go
if err != nil {
    fmt.Fprintf(os.Stderr, "Error: %v\n", err)
    os.Exit(1)
}
```

### 3. Output Format

Always output valid JSON with a "message" field:

```go
output := Output{
    Message: processedText,
}
outputData, _ := json.Marshal(output)
fmt.Print(string(outputData))
```

### 4. Logging

Use stderr for logging, stdout for output:

```go
fmt.Fprintf(os.Stderr, "Processing: %s\n", input.Prompt)
fmt.Print(string(outputData)) // Only the JSON output
```

### 5. Resource Cleanup

Clean up resources before exiting:

```go
defer func() {
    // Cleanup code
}()
```

## Testing WASM Modules

### Local Testing

Test your WASM module locally before uploading:

```bash
# Create test input
echo '{"prompt": "test input"}' | wasmtime your-module.wasm

# Or with a file
cat input.json | wasmtime your-module.wasm
```

### Integration Testing

Test within Mule:

1. Upload the WASM module
2. Create a test workflow with the module
3. Execute the workflow with test input
4. Verify the output

## Examples

### Example 1: Text Transformation

Converts input text to uppercase:

```go
package main

import (
    "encoding/json"
    "fmt"
    "io"
    "os"
    "strings"
)

type Input struct {
    Prompt string `json:"prompt"`
}

type Output struct {
    Message string `json:"message"`
}

func main() {
    inputData, _ := io.ReadAll(os.Stdin)
    
    var input Input
    json.Unmarshal(inputData, &input)
    
    output := Output{
        Message: strings.ToUpper(input.Prompt),
    }
    
    outputData, _ := json.Marshal(output)
    fmt.Print(string(outputData))
}
```

### Example 2: JSON Processing

Processes JSON input and adds metadata:

```go
package main

import (
    "encoding/json"
    "fmt"
    "io"
    "os"
    "time"
)

type Input struct {
    Prompt string `json:"prompt"`
}

type Output struct {
    Message      string            `json:"message"`
    ProcessedAt  string            `json:"processed_at"`
    Metadata     map[string]string `json:"metadata"`
}

func main() {
    inputData, _ := io.ReadAll(os.Stdin)
    
    var input Input
    json.Unmarshal(inputData, &input)
    
    output := Output{
        Message:     fmt.Sprintf("Processed: %s", input.Prompt),
        ProcessedAt: time.Now().Format(time.RFC3339),
        Metadata: map[string]string{
            "module": "json-processor",
            "version": "1.0",
        },
    }
    
    outputData, _ := json.Marshal(output)
    fmt.Print(string(outputData))
}
```

## Troubleshooting

### Common Issues

1. **"Error reading input"**
   - Check that the module is reading from stdin
   - Verify input is being passed correctly

2. **"Error parsing input JSON"**
   - Validate input JSON format
   - Check for proper field names and types

3. **No output from WASM module**
   - Ensure module writes to stdout
   - Check for errors in stderr
   - Verify JSON serialization works

4. **"WASM module exited with code: 0"**
   - This is normal for Go-compiled WASM
   - Check stdout for actual output

### Debugging

Enable debug logging in Mule to see WASM execution details:

```bash
./mule -db "postgres://..." -log-level debug
```

Check logs for:
- Input data being passed
- WASM module execution status
- stdout/stderr capture
- Output parsing

## Performance Considerations

### Binary Size

- Use `tinygo` instead of standard Go for smaller WASM binaries
- Strip debug symbols from binaries
- Optimize for size when compiling

### Execution Time

- WASM modules have startup overhead
- Keep modules small and focused
- Consider caching for repeated operations

### Memory Usage

- WASM modules run in isolated memory
- Monitor memory usage for large inputs
- Clean up resources properly
