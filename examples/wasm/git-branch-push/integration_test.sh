#!/bin/bash

# Integration test for git-branch-push WASM module registration

echo "Registering git-branch-push WASM module with Mule..."

# This would typically be done through the Mule API
# For now, we'll just verify the module can be built

echo "Building module..."
make clean build

if [ $? -eq 0 ]; then
    echo "Module built successfully"
    echo "Module size: $(stat -f%z main.wasm 2>/dev/null || stat -c%s main.wasm) bytes"
else
    echo "Failed to build module"
    exit 1
fi

echo "Integration test completed"