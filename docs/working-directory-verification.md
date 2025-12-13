# Working Directory Verification Guide

This document describes how to verify that agents and WASM modules operate in the correct directories.

## Prerequisites

1. A running Mule AI instance with the working directory functionality implemented
2. The working directory test workflow created by `examples/wasm/working_directory_test.go`
3. The WASM module compiled from `examples/wasm/working-dir-demo/`

## Verification Steps

### 1. Compile and Add the WASM Module

```bash
# Navigate to the working directory demo
cd examples/wasm/working-dir-demo/

# Build the WASM module
make build

# Add the WASM module to Mule AI through the API or CLI
# Example using curl:
curl -X POST http://localhost:8080/api/v1/wasm-modules \
  -H "Content-Type: multipart/form-data" \
  -F "name=working_dir_demo" \
  -F "description=WASM module that demonstrates working directory changes" \
  -F "module=@main.wasm"
```

### 2. Update the Test Workflow

Update the test workflow to use the WASM module for the first step instead of an agent.

### 3. Execute the Workflow

Run the workflow and monitor the execution:

```bash
# Execute the workflow through the API
curl -X POST http://localhost:8080/api/v1/workflows/{workflow_id}/execute \
  -H "Content-Type: application/json" \
  -d '{"input": {"prompt": "Test working directory functionality"}}'
```

### 4. Verification Points

1. **Initial Directory**: Verify that the first step (WASM module) runs in the job's initial working directory
2. **Directory Change**: Verify that the WASM module can successfully change the working directory
3. **Propagation**: Verify that the new working directory is correctly propagated to subsequent steps
4. **File Operations**: Verify that file operations in subsequent steps occur in the correct directory
5. **Isolation**: Verify that each job maintains its own working directory and doesn't interfere with others

### 5. Monitoring and Logging

Check the logs for:
- Directory change notifications
- File operation paths
- Working directory propagation between steps

### 6. Expected Behavior

- Files created by the WASM module should be in the directory it sets
- Files created by agents in subsequent steps should be in the same directory
- Different jobs should operate in different directories even if running concurrently
- The working directory should be persisted in the job record in the database

## Troubleshooting

If working directory changes aren't working correctly:

1. Check that the `set_working_directory` host function is properly implemented
2. Verify that the WASM module is correctly calling the host function
3. Ensure that the workflow engine is properly propagating the working directory
4. Confirm that the FilesystemTool is using the correct working directory
5. Check database records to ensure the working directory is being stored correctly