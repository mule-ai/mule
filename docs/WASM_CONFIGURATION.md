# WASM Module Configuration

This document explains how to use the configuration feature for WASM modules in Mule AI.

## Overview

WASM modules in Mule can now have associated configuration data that is automatically merged with input data when the module executes. This allows you to store static configuration values like API keys, endpoints, timeouts, and other settings directly with the module.

## How Configuration Works

When a WASM module with configuration is executed in a workflow, the configuration data is merged with the input data before being passed to the module via stdin as JSON.

For example, if your module has this configuration:
```json
{
  "api_key": "secret123",
  "endpoint": "https://api.example.com",
  "timeout": 30
}
```

And the input data from the previous workflow step is:
```json
{
  "prompt": "Process this text"
}
```

Your WASM module will receive the following combined data via stdin:
```json
{
  "prompt": "Process this text",
  "api_key": "secret123",
  "endpoint": "https://api.example.com",
  "timeout": 30
}
```

## Setting Configuration in the UI

### WASM Modules Page

1. Navigate to the WASM Modules page in the Mule dashboard
2. Click "Upload Module" to create a new module or "Edit" to modify an existing one
3. Fill in the module details (name, description)
4. In the "Configuration" field, enter valid JSON with your static configuration values
5. Upload your WASM file
6. Click "Upload Module" to save

### WASM Code Editor

1. Navigate to the WASM Code Editor page
2. Either create a new module or select an existing one
3. Go to the "Configuration" tab
4. Enter your JSON configuration in the text area
5. When creating a new module, fill in the configuration in the creation modal
6. Click "Compile & Save" to save your changes

## Configuration Best Practices

1. **Use for Static Values**: Configuration is best for static values that don't change between executions, such as:
   - API keys and secrets
   - Service endpoints
   - Timeout values
   - Feature flags

2. **Keep It Secure**: Remember that configuration is stored in the database and is accessible through the API. Don't store highly sensitive information unless your Mule instance is properly secured.

3. **Validate Your JSON**: Always ensure your configuration is valid JSON. The UI will validate this for you, but it's good practice to double-check.

4. **Document Your Config**: Use descriptive keys in your configuration to make it clear what each value is for.

## Example Configuration

Here's an example configuration for a module that integrates with an external API:

```json
{
  "api_key": "your-api-key-here",
  "base_url": "https://api.example.com/v1",
  "timeout_seconds": 30,
  "retries": 3,
  "features": {
    "enable_caching": true,
    "log_requests": false
  }
}
```

## Accessing Configuration in Your WASM Module

In your WASM module code, you simply access the configuration values as if they were part of the input data. Here's an example in Go:

```go
package main

import (
    "encoding/json"
    "fmt"
    "os"
)

type InputData struct {
    Prompt    string                 `json:"prompt"`
    APIKey    string                 `json:"api_key"`
    BaseURL   string                 `json:"base_url"`
    Timeout   int                    `json:"timeout_seconds"`
    Data      map[string]interface{} `json:"data,omitempty"`
}

type OutputData struct {
    Result  string `json:"result"`
    Success bool   `json:"success"`
}

func main() {
    decoder := json.NewDecoder(os.Stdin)
    var input InputData
    
    if err := decoder.Decode(&input); err != nil {
        fmt.Fprintf(os.Stderr, "Error decoding input: %v\n", err)
        os.Exit(1)
    }
    
    // Use the configuration values
    result := processWithAPI(input.Prompt, input.APIKey, input.BaseURL, input.Timeout)
    
    output := OutputData{
        Result:  result,
        Success: true,
    }
    
    encoder := json.NewEncoder(os.Stdout)
    if err := encoder.Encode(output); err != nil {
        fmt.Fprintf(os.Stderr, "Error encoding output: %v\n", err)
        os.Exit(1)
    }
}

func processWithAPI(prompt, apiKey, baseURL string, timeout int) string {
    // Your API integration logic here
    // Use the configuration values as needed
    return fmt.Sprintf("Processed '%s' using API at %s", prompt, baseURL)
}
```

## Testing with Configuration

When testing your WASM module in the editor:

1. Go to the "Test" tab
2. Enter your test input JSON
3. The module will automatically receive both your test input and the saved configuration
4. Run the test to see the combined result

Note that in the test environment, the configuration is merged with your test input just like it would be in a real workflow execution.

## Updating Configuration

You can update the configuration for an existing module at any time:

1. Go to the WASM Modules page
2. Click "Edit" on the module you want to update
3. Modify the configuration JSON as needed
4. Click "Update Module" to save

Changes to configuration take effect immediately for new executions of the module.