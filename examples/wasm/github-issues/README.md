# GitHub Issues WASM Module Example

This example demonstrates interacting with the GitHub Issues API from a WASM module.

## What it does

- Reads JSON input from stdin with GitHub authentication and issue details
- Makes HTTP requests to the GitHub API
- Returns issue data in JSON format

## Input Format

```json
{
  "token": "your-github-token",
  "owner": "owner-name",
  "repo": "repo-name",
  "issue_number": 123
}
```

## Building

```bash
GOOS=wasip1 GOARCH=wasm go build -o github-issues.wasm main.go
```

Or with TinyGo:

```bash
tinygo build -o github-issues.wasm -target wasm main.go
```

## Testing

```bash
echo '{"token": "ghp_xxx", "owner": "mule-ai", "repo": "mule", "issue_number": 1}' | wasmtime github-issues.wasm
```

## Host Functions Used

- `http_request_with_headers` - For making authenticated HTTP requests to the GitHub API
- `get_last_response_body` - For retrieving the response body
- `get_last_response_status` - For retrieving the status code

## Files

- `github_issues.go` - Main WASM module code
- `github_issues_projects.go` - Additional functionality for GitHub Projects
- `github_issues_projects_test.go` - Tests
- `github_issues_test.go` - Unit tests
