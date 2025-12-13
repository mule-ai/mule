# Issue State Tracker WASM Module

This WASM module is designed to track and manage issue states using GitHub labels. It ensures that only one state label is applied to an issue at any time by removing previous state labels when applying a new one.

## Functionality

- Accepts an array of valid state labels in the configuration
- Updates a GitHub issue by applying a new state label
- Automatically removes any existing state labels (from the configured list) before applying the new one
- Returns the input data unwrapped from the "prompt" field

## Input Format

The module expects the following input structure:

```json
{
  "prompt": "{\"issue\": \"https://api.github.com/repos/owner/repo/issues/123\", \"label\": \"in-progress\", \"comment\": \"Starting work on this issue\"}",
  "config": {
    "states": ["backlog", "todo", "in-progress", "review", "done"]
  },
  "token": "your-github-token"
}
```

### Fields

- `prompt`: A JSON string containing:
  - `issue`: The GitHub API URL for the issue to update
  - `label`: The new state label to apply
  - `comment`: (Optional) A comment to add to the issue (currently ignored)
- `config.states`: An array of valid state labels
- `token`: GitHub personal access token for authentication

## Output Format

On success, the module returns:

```json
{
  "success": true,
  "input": {
    "issue": "https://api.github.com/repos/owner/repo/issues/123",
    "label": "in-progress",
    "comment": "Starting work on this issue"
  }
}
```

On error, the module returns:

```json
{
  "success": false,
  "error": "Error message"
}
```

## Building

To build the WASM module:

```bash
make build
```

This will produce `issue_state_tracker.wasm` which can be registered with the Mule system.

## Usage in Workflows

In a workflow step, you would configure the module like this:

```json
{
  "type": "WASM",
  "wasm_module_id": "{module-id}",
  "config": {
    "states": ["backlog", "todo", "in-progress", "review", "done"]
  }
}
```

Then provide the input through the prompt field as shown in the input format section.