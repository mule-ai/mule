# Create Pull Request WASM Module

This WASM module creates a new pull request on GitHub. It's designed to be used in Mule AI workflows where you need to automatically create pull requests as part of your automation pipeline.

## Usage

The module expects input in the following format:

```json
{
  "token": "your-github-token",     // GitHub token for authentication
  "owner": "repository-owner",      // Repository owner (e.g., "octocat")
  "repo": "repository-name",        // Repository name (e.g., "Hello-World")
  "title": "Pull request title",    // Title for the pull request
  "head": "branch-name",            // Head branch (source branch) - Optional, will be detected if not provided
  "base": "main",                   // Base branch (target branch)
  "body": "Description of changes", // Optional description of the pull request
  "draft": false                    // Optional, whether to create as draft (default: false)
}
```

If the `head` parameter is not provided, the module will automatically detect the current branch name from the working directory.

## How it works

1. The module uses the GitHub REST API to create a pull request
2. It makes a POST request to `https://api.github.com/repos/{owner}/{repo}/pulls`
3. It requires a GitHub personal access token with appropriate permissions
4. If no head branch is specified, it automatically detects the current branch using git commands
5. It returns the URL of the newly created pull request on success

## Building

To build the WASM module:

```bash
make build
```

This will create a `main.wasm` file that can be uploaded to Mule AI.

## Error Handling

The module validates:
- All required parameters are provided
- GitHub API requests succeed
- Response status codes indicate success
- Current branch can be detected if head is not provided

## Host Functions Used

- `http_request_with_headers` - For making authenticated HTTP requests to the GitHub API
- `get_last_response_body` - For retrieving the response body from HTTP requests
- `get_last_response_status` - For retrieving the status code from HTTP requests
- `get_current_branch` - For automatically detecting the current branch name

## Authentication

The module requires a GitHub personal access token with appropriate permissions to create pull requests in the target repository. The token should be passed in the `token` field of the input.

## Automatic Branch Detection

If the `head` parameter is not provided, the module will automatically detect the current branch name by:
1. Using the current working directory
2. Verifying it's a git repository
3. Running `git rev-parse --abbrev-ref HEAD` to get the current branch name

## Example Workflow

1. Step 1: Previous steps create changes and push a branch to GitHub
2. Step 2: This WASM module creates a pull request for the new branch
3. Step 3: Subsequent steps can work with the created pull request

This allows for automated pull request creation as part of your CI/CD or automation workflows.

## Error Codes

The module uses the standard HTTP error codes from the GitHub API:
- 201 - Pull request created successfully
- 400 - Bad request (invalid parameters)
- 401 - Unauthorized (invalid token)
- 403 - Forbidden (insufficient permissions)
- 404 - Not found (repository or branch not found)
- 422 - Unprocessable entity (validation errors)

Additionally, for branch detection:
- 0xFFFFFFF1 - Failed to read base path from memory
- 0xFFFFFFF2 - Failed to get current working directory
- 0xFFFFFFF3 - Base path is not a git repository
- 0xFFFFFFF4 - Failed to get current branch name
- 0xFFFFFFF5 - Buffer too small for branch name
- 0xFFFFFFF6 - Failed to write branch name to memory