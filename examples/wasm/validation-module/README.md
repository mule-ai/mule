# Validation Module

A WASM module that executes validation commands and automatically triggers corrective workflows on failure.

## Configuration

The module expects a JSON configuration object as input:

```json
{
  "validation_command": "command to run for validation",
  "corrective_workflow_id": "ID of workflow to run on failure",
  "max_attempts": 3,
  "working_directory": "optional working directory"
}
```

## Host Functions Used

- `execute_target`: To trigger corrective workflows
- `wait_for_job_and_get_output`: To wait for workflow completion
- `get_working_directory`: To get the current working directory

## Behavior

1. Executes the validation command in the specified working directory
2. If successful, returns the result immediately
3. If failed:
   - Triggers the corrective workflow with context including:
     - Original validation command
     - Validation output (stdout, stderr, exit code)
     - Working directory
     - Remaining attempts
   - Waits for corrective workflow to complete
   - Retries validation
4. Respects the `max_attempts` limit

## Error Handling

- Validates configuration parameters
- Checks working directory existence
- Implements 30-second command timeout
- Provides detailed error messages in JSON format
- Handles edge cases gracefully

## Example Usage

```json
{
  "validation_command": "go test ./...",
  "corrective_workflow_id": "fix-test-failures",
  "max_attempts": 3,
  "working_directory": "/path/to/project"
}
```

## Output Format

On success:
```json
{
  "success": true,
  "result": {
    "success": true,
    "command": "go test ./...",
    "exit_code": 0,
    "stdout": "...",
    "stderr": "",
    "attempt": 1
  },
  "message": "Validation succeeded",
  "attempts": 1
}
```

On failure after max attempts:
```json
{
  "success": false,
  "result": {
    "success": false,
    "command": "go test ./...",
    "exit_code": 1,
    "stdout": "...",
    "stderr": "...",
    "attempt": 3,
    "error": "exit status 1"
  },
  "message": "Validation failed after maximum attempts",
  "attempts": 3
}
```