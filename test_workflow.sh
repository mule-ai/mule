#!/bin/bash

# Test script for Mule workflow execution
# This tests workflow execution with both sync and async modes

echo "Testing Mule workflow execution..."
echo ""

# First, let's list available models to see what workflows exist
echo "1. Listing available models (agents and workflows)..."
curl -s http://10.10.199.96:8140/v1/models | jq .
echo ""
echo ""

# Test synchronous workflow execution via default endpoint (should wait for completion)
echo "2. Testing SYNCHRONOUS workflow execution via /v1/chat/completions..."
echo "This should wait for completion and return the final result..."
curl -X POST http://10.10.199.96:8140/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "workflow/test",
    "messages": [
      {
        "role": "user",
        "content": "Hello, this is a test workflow execution."
      }
    ]
  }' | jq .

echo ""
echo ""

# Test that stream parameter is ignored for workflows (should still be synchronous)
echo "3. Testing that 'stream' parameter is ignored for workflows..."
echo "This should still be synchronous despite stream: true..."
curl -X POST http://10.10.199.96:8140/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "workflow/test",
    "messages": [
      {
        "role": "user",
        "content": "Hello, this is a test with stream parameter."
      }
    ],
    "stream": true
  }' | jq .

echo ""
echo ""

# Test asynchronous workflow execution using async/ prefix on model name
echo "4. Testing ASYNCHRONOUS workflow execution using async/workflow/test model..."
echo "This should return immediately with job info..."
RESPONSE=$(curl -s -X POST http://10.10.199.96:8140/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "async/workflow/test",
    "messages": [
      {
        "role": "user",
        "content": "Hello, this is an async workflow test."
      }
    ]
  }')

echo "$RESPONSE" | jq .

# Extract job ID if it exists
JOB_ID=$(echo "$RESPONSE" | jq -r '.id // empty')

if [ ! -z "$JOB_ID" ] && [ "$JOB_ID" != "null" ]; then
  echo ""
  echo "Job ID: $JOB_ID"
  echo ""
  echo "5. Checking job status..."
  sleep 2
  curl -s http://10.10.199.96:8140/api/v1/jobs/$JOB_ID | jq .
  echo ""
  echo "Waiting a bit more for job to complete..."
  sleep 3
  echo "6. Checking job status again..."
  curl -s http://10.10.199.96:8140/api/v1/jobs/$JOB_ID | jq .
fi

echo ""
echo "Test completed."