# post-to-mdserve WASM Module

This WASM module is designed to integrate with the mdserve microservice. It receives a markdown document as input and posts it to a configured endpoint, returning only the URL of the created document.

## Configuration

The module expects the following configuration parameters:

- `endpoint`: The URL of the mdserve API endpoint to post documents to

## Input Format

The module expects a JSON input with the following structure:

```json
{
  "prompt": "# Markdown Title\n\nThis is a markdown document...",
  "endpoint": "https://md.butler.ooo/api/document"
}
```

Note:
- The `prompt` field contains the markdown content to be posted
- The `endpoint` parameter can be provided either through configuration or as part of the input. If both are provided, the input value takes precedence.
- A filename will be automatically generated as a hash of the content, ensuring deterministic filenames for identical content

## Output Format

The module outputs only the URL of the created document as a plain string:

```
https://md.butler.ooo/test
```

## API Response Format

The module expects the mdserve API to return a JSON response with the following structure:

```json
{
  "api_url": "https://md.butler.ooo/api/document?filename=test",
  "filename": "test",
  "message": "Document 'test' created successfully",
  "url": "https://md.butler.ooo/test"
}
```

Only the `url` field is returned by the module.

## Usage

1. Compile the module using the Mule WASM compiler
2. Configure the module with the required endpoint parameter
3. Execute the module with a markdown document as input
4. The module will return the URL of the created document