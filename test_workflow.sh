#!/bin/bash

# Test script for Mule workflow execution
# This tests workflow execution with both sync and async modes

echo "Testing Mule workflow execution..."
echo "Endpoint: http://10.10.199.96:8140/v1/chat/completions"
echo ""

# First, let's list available models to see what workflows exist
echo "1. Listing available models (agents and workflows)..."
curl -s http://10.10.199.96:8140/v1/models | jq .
echo ""
echo ""

# Test synchronous workflow execution (stream: false)
echo "2. Testing SYNCHRONOUS workflow execution..."
curl -X POST http://10.10.199.96:8140/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "workflow/test",
    "messages": [
      {
        "role": "user",
        "content": "Hello, this is a test workflow execution."
      }
    ],
    "stream": false
  }' | jq .

echo ""
echo ""

# Test asynchronous workflow execution (stream: true)
echo "3. Testing ASYNCHRONOUS workflow execution..."
RESPONSE=$(curl -s -X POST http://10.10.199.96:8140/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "workflow/test",
    "messages": [
      {
        "role": "user",
        "content": "Hello, this is an async workflow test."
      }
    ],
    "stream": true
  }')

echo "$RESPONSE" | jq .

# Extract job ID if it exists
JOB_ID=$(echo "$RESPONSE" | jq -r '.id // empty')

if [ ! -z "$JOB_ID" ] && [ "$JOB_ID" != "null" ]; then
  echo ""
  echo "Job ID: $JOB_ID"
  echo ""
  echo "4. Checking job status..."
  sleep 2
  curl -s http://10.10.199.96:8140/api/v1/jobs/$JOB_ID | jq .
fi

echo ""
echo "Test completed."