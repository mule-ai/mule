# Git Worktree WASM Module

This WASM module creates a new git worktree and sets the working directory to the worktree path. It's designed to be used in Mule AI workflows where you need to isolate changes in a separate worktree.

## Usage

The module expects input in the following format:

```json
{
  "prompt": "{\"worktree_name\": \"feature-branch\"}",
  "repository": "/path/to/repository"  // Optional, will use current directory if not provided
}
```

Or with direct object format:

```json
{
  "prompt": {
    "worktree_name": "feature-branch"
  },
  "repository": "/path/to/repository"  // Optional
}
```

## Building

To build the WASM module:

```bash
make build
```

This will create a `main.wasm` file that can be uploaded to Mule AI.

## How it works

1. Parses the input to extract the worktree name
2. Calls the `create_git_worktree` host function to create or use a git worktree
3. The host function:
   - Validates that the base path is a git repository
   - Checks if the worktree already exists
   - If it exists, simply uses it without error
   - If it doesn't exist, creates a new worktree using `git worktree add`
   - Sets the working directory for subsequent workflow steps
4. Returns success/failure information

## Error Handling

The module validates:
- Worktree name is provided and valid (no path traversal characters)
- Base path is a git repository (if provided)
- Git worktree command succeeds (when creating a new worktree)

## Host Functions Used

- `create_git_worktree` - To create a proper git worktree and set the working directory
- `set_working_directory` - Backup function for setting the working directory

## Example Workflow

1. Step 1: This WASM module creates or uses a worktree named "feature-xyz"
2. Step 2: An agent or another WASM module operates in the worktree directory
3. Step 3: Another module could commit and push changes from the worktree

This allows for isolated development in separate worktrees without affecting the main repository checkout. If the same worktree name is used multiple times, it will simply reuse the existing worktree rather than failing.

## Implementation Details

The module uses a host function implemented in the Mule WASM executor that:
1. Validates the base path is a git repository
2. Uses the `git worktree add` command to create a proper worktree
3. Automatically sets the working directory for subsequent steps
4. Returns appropriate error codes for different failure scenarios