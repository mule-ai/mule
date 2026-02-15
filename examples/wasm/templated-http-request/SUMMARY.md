# Templated HTTP Request WASM Module

## Summary

This module is a enhanced version of the basic HTTP request WASM module that adds support for templating. It allows users to include `{{.MESSAGE}}` in their request data, which will be replaced with the value from the `prompt` field in the input.

## Key Features

1. **Template Support**: The module processes the `data` field and replaces any occurrences of `{{.MESSAGE}}` with the value from the `prompt` field.
2. **Backward Compatibility**: The module maintains full compatibility with the original HTTP request module - if no `prompt` field is provided or no `{{.MESSAGE}}` is found in the data, it behaves exactly like the original.
3. **Flexible Usage**: Users can include the template variable anywhere in their request data structure.

## Usage Example

```json
{
  "url": "https://api.example.com/messages",
  "method": "POST",
  "prompt": "Hello, this is my message!",
  "headers": {
    "Content-Type": "application/json",
    "Authorization": "Bearer token123"
  },
  "data": {
    "message": "{{.MESSAGE}}",
    "timestamp": "2023-01-01T00:00:00Z",
    "source": "mule-wasm-module"
  }
}
```

In this example, the `{{.MESSAGE}}` in the `data.message` field will be replaced with "Hello, this is my message!" before the HTTP request is made.

## Implementation Details

The template processing is handled by the `processTemplate` function which:
1. Takes the `data` field and the `prompt` value as inputs
2. Marshals the data to JSON
3. Replaces all occurrences of `{{.MESSAGE}}` with the prompt value
4. Unmarshals the result back to an interface{}
5. Returns the processed data for use in the HTTP request

## Files

- `simple.go` - Main module implementation with template processing
- `README.md` - Documentation for the module
- `test_input.json` - Sample input demonstrating templating
- `test_templated.sh` - Test script for the module
- `test_template.go` - Standalone test for the template processing function
- `templated-http-request.wasm` - Compiled WASM module