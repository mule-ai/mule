# GitHub Comment WASM Module

This WASM module allows you to post comments on GitHub issues using the GitHub API.

## Overview

The module takes an issue URL and comment content in the prompt field, and uses a GitHub token from the configuration to authenticate and post the comment.

## Features

- Posts comments to GitHub issues via the GitHub API
- Securely handles GitHub tokens through configuration
- Provides detailed error messages for troubleshooting
- Extracts comment URL from successful responses
- Follows Mule WASM module conventions

## Input Format

The module expects JSON input with the following structure, where the actual data is wrapped in a prompt field:

```json
{
  "prompt": "{\"issue\": \"https://api.github.com/repos/owner/repo/issues/123\", \"comment\": \"This is my comment content\"}",
  "token": "your-github-personal-access-token"
}
```

Field descriptions:
- `prompt`: JSON string containing the issue URL and comment content
- `issue`: Full URL to the GitHub issue API endpoint (inside the prompt)
- `comment`: Content of the comment to post (if empty string, module exits successfully without posting)
- `token`: GitHub personal access token with appropriate permissions (at top level)

## Output Format

On success:
```json
{
  "success": true,
  "message": "Comment posted successfully",
  "url": "https://github.com/owner/repo/issues/123#issuecomment-..."
}
```

On error:
```json
{
  "success": false,
  "error": "Detailed error message with status code and GitHub API error details if available"
}
```

## Building the Module

To compile the WASM module:

```bash
GOOS=wasip1 GOARCH=wasm go build -o github-comment.wasm main.go
```

## Using in Mule

1. Build the WASM module
2. Encode the binary in base64
3. Upload to Mule using the API
4. Create a workflow that uses the module with appropriate configuration
5. Execute the workflow with input containing the issue URL and comment

### Example Workflow Step

```json
{
  "type": "WASM",
  "wasm_module_id": "{module-id}",
  "config": {
    "token": "{{GITHUB_TOKEN}}"
  }
}
```

### Example Execution Payload

```json
{
  "prompt": "{\"issue\": \"https://api.github.com/repos/your-org/your-repo/issues/1\", \"comment\": \"This is an automated comment from Mule AI!\"}"
}
```

## Required Permissions

The GitHub token needs the following permissions:
- `public_repo` scope for public repositories
- `repo` scope for private repositories

Generate a personal access token at: https://github.com/settings/tokens

## Error Handling

The module provides detailed error messages for common issues:
- Missing or invalid input parameters
- Authentication failures
- Network errors
- GitHub API errors with status codes and detailed messages
- Buffer size limitations (512KB response size limit)

Special behavior:
- Empty comment string: Exits successfully without posting a comment

## Technical Details

### Memory Safety
The module uses helper functions to safely convert Go strings and data structures to pointers for the WASM host functions, reducing the complexity of memory management.

### Response Processing
The module attempts to parse GitHub API responses to extract useful information:
- On success: Extracts the HTML URL of the created comment
- On error: Parses detailed error messages from the GitHub API

### Buffer Limitations
The module allocates a 512KB buffer for response data, which should be sufficient for most GitHub API responses.