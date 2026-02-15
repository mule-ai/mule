#!/bin/bash

# Update the code-editor agent with the new system prompt for Go dependency fixes

AGENT_ID="bfa17fec-74ec-44e8-8a76-8722a03b7aaf"
API_BASE="https://mule.butler.ooo/api/v1"

# Update the agent
echo "Updating agent with new system prompt..."
curl -X PUT \
  -H "Content-Type: application/json" \
  -d @agent_update_payload.json \
  "$API_BASE/agents/$AGENT_ID"

echo "Agent update completed."