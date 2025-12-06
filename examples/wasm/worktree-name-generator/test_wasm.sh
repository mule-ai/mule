#!/bin/bash

# Test script for the worktree-name-generator WASM module

echo "Testing worktree-name-generator WASM module..."

# Test with simple input
echo "Test 1: Simple input"
echo '{"prompt": "{\"title\": \"Feature: Add MCP client support\"}"}' | wasmtime run main.wasm

# Test with complex input (similar to the one provided)
echo "Test 2: Complex input"
echo '{"prompt": "{\"assignee\":{\"name\":\"mule-bot\",\"url\":\"https://github.com/mule-bot\"},\"body\":\"A user should be able to add mcp servers as tools. There should be support for registering and calling multiple servers.\",\"comments\":[{\"body\":\"ack\",\"created_at\":\"2025-12-03T16:24:43Z\",\"updated_at\":\"2025-12-03T16:24:43Z\",\"user\":\"mule-bot\"}],\"state\":\"open\",\"title\":\"Feature: Add MCP client support\",\"url\":\"https://api.github.com/repos/mule-ai/mule/issues/7\"}"}' | wasmtime run main.wasm