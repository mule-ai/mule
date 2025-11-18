#!/bin/bash

# Test script for Mule chat completions endpoint
# This tests the exact model that was failing in the logs

echo "Testing Mule chat completions endpoint..."
echo "Endpoint: http://10.10.199.96:8140/v1/chat/completions"
echo "Model: agent/test-agent"
echo ""

curl -X POST http://10.10.199.96:8140/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "agent/test",
    "messages": [
      {
        "role": "user",
        "content": "Hello, this is a test message."
      }
    ],
    "stream": false
  }'

echo ""
echo "Test completed."
