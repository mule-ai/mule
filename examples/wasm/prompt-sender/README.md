# Prompt Sender WASM Module

This WASM module sends a prompt to a specified service using the JSON format `{"message": "prompt content"}`.

## Features

- Sends POST requests with the required JSON format
- Configurable URL through module configuration
- Handles HTTP response parsing
- Error handling for common failure cases

## Configuration

The module expects a URL to be provided in the module configuration:

```json
{
  "url": "http://example.com/api/messages"
}
```

## Input Format

The module accepts input in the standard WASM module format:

```json
{
  "prompt": "The message content to send",
  "data": {
    // Optional additional data that will be merged with the message payload
  }
}
```

Any fields in the `data` object will be merged directly into the message payload that is sent to the target service. For example:

```json
{
  "prompt": "Hello, this is a test message",
  "data": {
    "option1": true,
    "option2": "results.md"
  }
}
```

Will result in the following payload being sent to the service:

```json
{
  "message": "Hello, this is a test message",
  "option1": true,
  "option2": "results.md"
}
```

## Output Format

The module returns results in the standard format:

```json
{
  "result": "Successfully sent prompt to http://example.com/api/messages",
  "data": {
    "response": {
      // Service response data
    }
  },
  "status_code": 200,
  "success": true
}
```

## Building

To build this module for use with Mule AI:

```bash
GOOS=wasip1 GOARCH=wasm go build -o prompt_sender.wasm main.go
```

## Usage in Workflows

1. Create a new WASM module in the Mule AI dashboard
2. Upload the compiled `.wasm` file
3. Set the configuration with your target URL
4. Use the module in a workflow step