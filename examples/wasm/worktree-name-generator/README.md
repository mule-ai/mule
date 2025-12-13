# Worktree Name Generator WASM Module

This WASM module generates a worktree name based on the current date and issue title. It's designed to be used in Mule AI workflows where you need to create standardized worktree names for Git branches.

## Usage

The module expects input in the following format:

```json
{
  "prompt": "{\"title\": \"Feature: Add MCP client support\"}"
}
```

Or alternatively:

```json
{
  "prompt": "{\"title\": \"Feature: Add MCP client support\", \"other_field\": \"value\"}"
}
```

## Output

The module outputs a worktree name in the format:

```json
{
  "worktree_name": "feature-add-mcp-client-support"
}
```

The worktree name follows these rules:
- Based on the issue title, converted to lowercase
- Spaces replaced with dashes
- Special characters removed
- Limited to 64 characters total

## Building

To build the WASM module:

```bash
make build
```

This will create a `main.wasm` file that can be uploaded to Mule AI.

## Example Workflow

1. Step 1: This WASM module generates a worktree name based on an issue
2. Step 2: Another module (like git-worktree) uses this name to create a worktree
3. Step 3: Subsequent workflow steps operate in the new worktree

This allows for consistent naming of worktrees based on the issues they address.