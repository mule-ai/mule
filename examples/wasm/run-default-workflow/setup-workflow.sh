#!/bin/bash
# Script to create a complete workflow with steps and test it with WASM

# This script assumes you have a running Mule server on localhost:8080
# and that you have curl and jq installed

echo "Creating a complete workflow with steps..."

# Create a provider (if not already exists)
echo "Creating provider..."
PROVIDER_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/providers \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Local Provider",
    "api_base_url": "",
    "api_key": "test-key"
  }')

PROVIDER_ID=$(echo $PROVIDER_RESPONSE | jq -r '.id')
echo "Provider ID: $PROVIDER_ID"

# Create an agent
echo "Creating agent..."
AGENT_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Test Agent",
    "description": "Test agent for workflow",
    "provider_id": "'$PROVIDER_ID'",
    "model_id": "gemini-1.5-flash",
    "system_prompt": "You are a helpful assistant."
  }')

AGENT_ID=$(echo $AGENT_RESPONSE | jq -r '.id')
echo "Agent ID: $AGENT_ID"

# Create a workflow
echo "Creating workflow..."
WORKFLOW_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Complete Test Workflow",
    "description": "A complete workflow with one agent step",
    "is_async": false
  }')

WORKFLOW_ID=$(echo $WORKFLOW_RESPONSE | jq -r '.id')
echo "Workflow ID: $WORKFLOW_ID"

# Add a step to the workflow
echo "Adding step to workflow..."
STEP_RESPONSE=$(curl -s -X POST http://localhost:8080/api/v1/workflows/$WORKFLOW_ID/steps \
  -H "Content-Type: application/json" \
  -d '{
    "step_order": 1,
    "step_type": "agent",
    "agent_id": "'$AGENT_ID'",
    "description": "Call test agent"
  }')

echo "Step created successfully"

echo "Workflow '$WORKFLOW_ID' is ready to use!"

echo "To test with the WASM module:"
echo "1. Compile the WASM module:"
echo "   cd examples/wasm/run-default-workflow"
echo "   GOOS=wasip1 GOARCH=wasm go build -o run-default-workflow.wasm ."
echo ""
echo "2. Upload the WASM module to Mule"
echo "3. Create a workflow that uses this WASM module"
echo "4. Execute the workflow with input like:"
echo '   {"prompt": "Hello, process this text"}'