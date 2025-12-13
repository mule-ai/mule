# Issues to Markdown WASM Module

This WASM module converts a list of GitHub issues (in a specific format) to a formatted markdown document.

## Input Format

The module expects a JSON object with a `result` field containing an array of issue objects:

```json
{
  "result": [
    {
      "body": "Issue description",
      "due_date": "2025-12-10",
      "filter": "Personal",
      "state": "open",
      "status": "Todo",
      "title": "Issue Title",
      "url": "https://api.github.com/repos/user/repo/issues/1"
    }
  ]
}
```

## Output Format

The module outputs a JSON object with a `markdown` field containing the formatted markdown:

```json
{
  "markdown": "# Issue Title\n\n* Link: https://api.github.com/repos/user/repo/issues/1\n* State: Todo\n* Due Date: 12/10/25\n* Description: Issue description\n\n-----\n\n..."
}
```

## Building

### Using Go Compiler

```bash
make build
```

### Using TinyGo (for smaller binaries)

```bash
make build-tinygo
```

## Testing

Create a `test-input.json` file with sample data, then run:

```bash
make test
```

Or manually with wasmtime:

```bash
wasmtime issues-to-markdown.wasm < test-input.json
```

## Usage in Mule

1. Build the WASM module
2. Encode the binary in base64
3. Upload to Mule using the API
4. Create a workflow that uses the module
5. Execute the workflow