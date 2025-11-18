#!/bin/bash

# Mule AI Platform - Development Setup Script
# ===========================================
#
# This script sets up a complete development environment for the Mule AI Platform,
# including:
# - A provider configuration for external AI services
# - A test agent for AI interactions
# - A WASM module for WebAssembly testing
#
# Prerequisites:
# - jq (for JSON parsing)
# - curl (for API calls)
# - base64 (for WASM file encoding)
# - Go compiler (if hello.wasm needs to be compiled)
#
# Usage:
#   ./setup-dev.sh                    # Run with default settings
#   ./setup-dev.sh --dry-run          # Test without making API calls
#   ./setup-dev.sh --api-base URL     # Use custom API URL
#   ./setup-dev.sh --help             # Show help
#
# The script will:
# 1. Create a provider with proxy configuration
# 2. Create a test agent linked to the provider
# 3. Upload hello.wasm as a WASM module (compiling from hello.go if needed)
#
# After running, you can test the setup using the curl commands provided
# in the script output.

set -e

# Default API base URL
API_BASE="http://10.10.199.96:8140"
DRY_RUN=false

# Parse command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --dry-run)
            DRY_RUN=true
            shift
            ;;
        --api-base)
            API_BASE="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [--dry-run] [--api-base API_URL]"
            echo ""
            echo "Options:"
            echo "  --dry-run     Test the script without making API calls"
            echo "  --api-base    Set the API base URL (default: $API_BASE)"
            echo "  -h, --help    Show this help message"
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            echo "Use --help for usage information"
            exit 1
            ;;
    esac
done

echo "🚀 Setting up Mule AI Platform..."
echo "API Base: $API_BASE"
if [ "$DRY_RUN" = true ]; then
    echo "🧪 DRY RUN MODE - No API calls will be made"
fi
echo ""

# Create provider
echo "📡 Creating provider..."
if [ "$DRY_RUN" = true ]; then
    echo "🧪 DRY RUN: Would create provider with proxy configuration"
    PROVIDER_ID="dry-run-provider-id"
else
    # Check if provider with name "proxy" already exists
    EXISTING_PROVIDER=$(curl -s -X GET "${API_BASE}/api/v1/providers" | jq -r '.[] | select(.name=="proxy") | .id')

    if [ -n "$EXISTING_PROVIDER" ]; then
        echo "⚠️  Provider 'proxy' already exists with ID: $EXISTING_PROVIDER"
        PROVIDER_ID=$EXISTING_PROVIDER
    else
        PROVIDER_RESPONSE=$(curl -s -X POST "${API_BASE}/api/v1/providers" \
          -H "Content-Type: application/json" \
          -d '{
            "name": "proxy",
            "api_base_url": "https://bifrost.butler.ooo/v1",
            "api_key": "sk-bf-b235fddc-75ed-4679-ab6c-580487f873fa"
          }')

        PROVIDER_ID=$(echo "$PROVIDER_RESPONSE" | jq -r '.id')
        if [ "$PROVIDER_ID" = "null" ]; then
            echo "❌ Failed to create provider:"
            echo "$PROVIDER_RESPONSE"
            exit 1
        fi
        echo "✅ Provider created with ID: $PROVIDER_ID"
    fi
fi
echo "✅ Using provider with ID: $PROVIDER_ID"

# Wait a moment for provider to be fully created
sleep 1

# Create agent
echo "🤖 Creating agent..."
if [ "$DRY_RUN" = true ]; then
    echo "🧪 DRY RUN: Would create agent with provider ID: $PROVIDER_ID"
    AGENT_ID="dry-run-agent-id"
else
    AGENT_RESPONSE=$(curl -s -X POST "${API_BASE}/api/v1/agents" \
      -H "Content-Type: application/json" \
      -d "{
        \"name\": \"test\",
        \"description\": \"test agent\",
        \"provider_id\": \"$PROVIDER_ID\",
        \"model_id\": \"llamacpp/qwen3-30b-a3b\",
        \"system_prompt\": \"you're a helpful assistant\"
      }")

    AGENT_ID=$(echo "$AGENT_RESPONSE" | jq -r '.id')
    if [ "$AGENT_ID" = "null" ]; then
        echo "❌ Failed to create agent:"
        echo "$AGENT_RESPONSE"
        exit 1
    fi
fi
echo "✅ Agent created with ID: $AGENT_ID"

# Wait a moment for agent to be fully created
sleep 1


echo ""
echo "🎉 Setup complete!"
echo "Provider: $PROVIDER_ID"
echo "Agent: $AGENT_ID"
echo ""
echo "You can now test the agent with:"
echo "curl -X POST \"${API_BASE}/v1/chat/completions\" \\"
echo "  -H \"Content-Type: application/json\" \\"
echo "  -d '{"
echo "    \"model\": \"agent/test\","
echo "    \"messages\": [{\"role\": \"user\", \"content\": \"Hello!\"}]"
echo "  }'"
echo ""
