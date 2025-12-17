#!/bin/bash

# Integration test script for the create pull request WASM module
# This script tests the module in a more realistic scenario

set -e

echo "Building the WASM module..."
make build

echo "Creating test repository structure..."
mkdir -p test-repo
cd test-repo

# Initialize a git repository
git init
echo "# Test Repository" > README.md
git add README.md
git commit -m "Initial commit"

# Create a feature branch
git checkout -b feature-branch
echo "## Feature Branch" >> README.md
git add README.md
git commit -m "Add feature branch content"

echo "Test repository created with main and feature-branch branches."
echo "In a real scenario, you would:"
echo "1. Push the branches to a GitHub repository"
echo "2. Run the WASM module to create a pull request"
echo "3. The module will automatically detect that we're on 'feature-branch'"
echo "4. Verify the pull request was created successfully"

cd ..
echo "Integration test setup completed."