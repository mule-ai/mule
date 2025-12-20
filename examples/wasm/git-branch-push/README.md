# Git Branch Push WASM Module

This WASM module pushes the current worktree as a git branch to the remote origin. It's designed to be used in Mule AI workflows where you need to create and push branches as part of your automation pipeline.

## Usage

The module expects input in the following format:

```json
{
  "token": "your-git-token",      // Authentication token for git operations
  "repository": "/path/to/repository",  // Optional, will use current directory if not provided
  "user_name": "Your Name",       // Git user name for commit (optional)
  "user_email": "you@example.com" // Git user email for commit (optional)
}
```

## How it works

1. The module calls a single host function `push_current_branch` that handles everything:
   - Sets git user config if name/email provided
   - Stages all changes in the repository
   - Commits the changes with a default message
   - Gets the current working directory
   - Uses the directory name (worktree name) as the branch name
   - Switches to an existing branch or creates a new branch with that name
   - Pushes the branch to the remote repository (assumes "origin")
   - Handles authentication using the provided token

## Building

To build the WASM module:

```bash
make build
```

This will create a `main.wasm` file that can be uploaded to Mule AI.

## Error Handling

The module validates:
- Base path is a git repository (if provided)
- Worktree name is a valid branch name
- Git commands succeed

## Host Functions Used

- `push_current_branch` - A single host function that does everything

## Authentication

The module accepts a `token` parameter for authentication with the git remote. This token is passed to the host function which uses it to authenticate git operations.

## Git User Configuration

The module accepts optional `user_name` and `user_email` parameters to set the git user configuration for the commit. If not provided, it will use the repository's existing configuration.

## Example Workflow

1. Step 1: A previous step (like git-worktree) sets up the working directory in a worktree
2. Step 2: This WASM module stages, commits, and pushes the current worktree as a branch to the remote (creating the branch if it doesn't exist, or updating it if it does)
3. Step 3: Subsequent steps can work with the new/existing branch

This allows for automated branch creation and publishing as part of your CI/CD or automation workflows.

## Error Codes

- `0x00000000` - Success
- `0xFFFFFFF0` - Failed to read token from memory
- `0xFFFFFFF1` - Failed to read parameter from memory
- `0xFFFFFFF2` - Failed to get current working directory
- `0xFFFFFFF3` - Base path is not a git repository
- `0xFFFFFFF4` - Invalid branch name derived from worktree
- `0xFFFFFFF5` - Failed to create git branch
- `0xFFFFFFF6` - Failed to push git branch
- `0xFFFFFFF7` - Failed to stage changes
- `0xFFFFFFF8` - Failed to commit changes
- `0xFFFFFFF9` - Failed to set git user config