---
name: mule-api
description: Interact with Mule AI workflow platform - manage providers, agents, skills, workflows, WASM modules, and execute AI tasks via OpenAI-compatible API.
---

# Mule AI Agent Skill

This skill helps you interact with a running Mule AI workflow platform. Mule is an AI workflow platform that enables you to create, configure, and execute AI agents and workflows through an OpenAI-compatible API.

## Prerequisites

### Server Connection

Before interacting with Mule, you must determine the server URL:

1. **Check for MULE_SERVER environment variable**:
   ```bash
   echo $MULE_SERVER
   ```

2. **If MULE_SERVER is not set**, ask the user to either:
   - Set the `MULE_SERVER` environment variable (e.g., `export MULE_SERVER=https://mule.butler.ooo`)
   - Provide the Mule server URL directly

3. **All API requests** should be made to the base URL stored in `MULE_SERVER` (e.g., `curl ${MULE_SERVER}/v1/models`)

### Base URL Variable

Throughout this skill, replace `${MULE_SERVER}` with the actual server URL. Common examples:
- `http://localhost:8080` (local development)
- `https://mule.butler.ooo` (production)

---

## Overview of Mule Primitives

Mule uses six core primitives stored in PostgreSQL:

1. **Providers** - AI provider configurations (OpenAI-compatible APIs)
2. **Skills** - Pi agent skills that can be assigned to agents
3. **Agents** - AI agents powered by pi RPC runtime
4. **WASM Modules** - WebAssembly modules for imperative code
5. **Workflows** - Ordered sequences of workflow steps
6. **Workflow Steps** - Individual execution steps (AGENT or WASM type)

---

## Listing Available Models

To see what agents and workflows are available, list the models:

```bash
curl -s ${MULE_SERVER}/v1/models | jq .
```

**Response format:**
```json
{
  "data": [
    { "id": "agent/my-agent", "object": "model", "owned_by": "mule" },
    { "id": "workflow/my-workflow", "object": "model", "owned_by": "mule" }
  ]
}
```

Agents are prefixed with `agent/` and workflows with `workflow/`.

---

## Executing Agents and Workflows

### Execute an Agent

Run an agent using the `/v1/chat/completions` endpoint:

```bash
curl -s -X POST ${MULE_SERVER}/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "agent/my-agent",
    "messages": [
      { "role": "user", "content": "Your prompt here" }
    ]
  }' | jq .
```

### Execute a Workflow (Synchronous)

Run a workflow synchronously (waits for completion):

```bash
curl -s -X POST ${MULE_SERVER}/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "workflow/my-workflow",
    "messages": [
      { "role": "user", "content": "Your input here" }
    ]
  }' | jq .
```

### Execute a Workflow (Asynchronous)

Run a workflow asynchronously (returns immediately with job ID):

```bash
curl -s -X POST ${MULE_SERVER}/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "async/workflow/my-workflow",
    "messages": [
      { "role": "user", "content": "Your input here" }
    ]
  }' | jq .
```

**Response includes job ID:**
```json
{
  "id": "job-uuid-here",
  "object": "async.job",
  "status": "queued",
  "message": "The workflow has been started"
}
```

### Check Job Status

For async workflows, check job status:

```bash
curl -s ${MULE_SERVER}/api/v1/jobs/{job-id} | jq .
```

### List All Jobs

```bash
curl -s "${MULE_SERVER}/api/v1/jobs?page=1&page_size=20" | jq .
```

Query parameters:
- `page` - Page number (default: 1)
- `page_size` - Results per page (default: 20, max: 100)
- `status` - Filter by status (queued, running, completed, failed)
- `search` - Search by job ID
- `workflow_name` - Filter by workflow name

---

## Managing Providers

Providers are AI provider configurations (OpenAI-compatible APIs).

### List Providers

```bash
curl -s ${MULE_SERVER}/api/v1/providers | jq .
```

### Create a Provider

```bash
curl -s -X POST ${MULE_SERVER}/api/v1/providers \
  -H "Content-Type: application/json" \
  -d '{
    "name": "openai",
    "api_base_url": "https://api.openai.com/v1",
    "api_key_encrypted": "your-encrypted-api-key"
  }' | jq .
```

### Get Provider Models

List available models for a provider:

```bash
curl -s ${MULE_SERVER}/api/v1/providers/{provider-id}/models | jq .
```

### Update a Provider

```bash
curl -s -X PUT ${MULE_SERVER}/api/v1/providers/{provider-id} \
  -H "Content-Type: application/json" \
  -d '{
    "name": "openai",
    "api_base_url": "https://api.openai.com/v1",
    "api_key_encrypted": "updated-key"
  }' | jq .
```

### Delete a Provider

```bash
curl -s -X DELETE ${MULE_SERVER}/api/v1/providers/{provider-id}
```

---

## Managing Skills

Skills are pi agent skills that can be assigned to agents. They define capabilities the agent can use.

### List Skills

```bash
curl -s ${MULE_SERVER}/api/v1/skills | jq .
```

### Create a Skill

```bash
curl -s -X POST ${MULE_SERVER}/api/v1/skills \
  -H "Content-Type: application/json" \
  -d '{
    "name": "web-search",
    "description": "Enables the agent to search the web",
    "path": "/path/to/skill/directory",
    "enabled": true
  }' | jq .
```

### Get a Skill

```bash
curl -s ${MULE_SERVER}/api/v1/skills/{skill-id} | jq .
```

### Update a Skill

```bash
curl -s -X PUT ${MULE_SERVER}/api/v1/skills/{skill-id} \
  -H "Content-Type: application/json" \
  -d '{
    "name": "web-search",
    "description": "Updated description",
    "path": "/path/to/skill/directory",
    "enabled": true
  }' | jq .
```

### Delete a Skill

```bash
curl -s -X DELETE ${MULE_SERVER}/api/v1/skills/{skill-id}
```

---

## Managing Agents

Agents are AI entities powered by pi RPC that can execute tasks.

### List Agents

```bash
curl -s ${MULE_SERVER}/api/v1/agents | jq .
```

### Create an Agent

```bash
curl -s -X POST ${MULE_SERVER}/api/v1/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-agent",
    "description": "A helpful assistant agent",
    "provider_id": "provider-id-from-previous-step",
    "model_id": "gpt-4",
    "system_prompt": "You are a helpful AI assistant.",
    "pi_config": {
      "timeout": 300
    },
    "skill_ids": ["skill-id-1", "skill-id-2"]
  }' | jq .
```

**Agent fields:**
- `name` - Agent name (required)
- `description` - Agent description
- `provider_id` - ID of the provider to use (required)
- `model_id` - Model identifier from the provider (required)
- `system_prompt` - System prompt for the agent
- `pi_config` - Pi-specific configuration (map)
- `skill_ids` - Array of skill IDs to assign (optional)

### Get an Agent

```bash
curl -s ${MULE_SERVER}/api/v1/agents/{agent-id} | jq .
```

### Update an Agent

```bash
curl -s -X PUT ${MULE_SERVER}/api/v1/agents/{agent-id} \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-agent",
    "description": "Updated description",
    "provider_id": "provider-id",
    "model_id": "gpt-4",
    "system_prompt": "You are an updated assistant.",
    "pi_config": {}
  }' | jq .
```

### Delete an Agent

```bash
curl -s -X DELETE ${MULE_SERVER}/api/v1/agents/{agent-id}
```

---

## Managing Agent Skills

### Get Skills Assigned to an Agent

```bash
curl -s ${MULE_SERVER}/api/v1/agents/{agent-id}/skills | jq .
```

### Assign Skills to an Agent

```bash
curl -s -X PUT ${MULE_SERVER}/api/v1/agents/{agent-id}/skills \
  -H "Content-Type: application/json" \
  -d '{
    "skill_ids": ["skill-id-1", "skill-id-2"]
  }' | jq .
```

### Remove a Skill from an Agent

```bash
curl -s -X DELETE ${MULE_SERVER}/api/v1/agents/{agent-id}/skills/{skill-id}
```

---

## Managing Agent Tools

### Get Tools Assigned to an Agent

```bash
curl -s ${MULE_SERVER}/api/v1/agents/{agent-id}/tools | jq .
```

### Assign a Tool to an Agent

```bash
curl -s -X POST ${MULE_SERVER}/api/v1/agents/{agent-id}/tools \
  -H "Content-Type: application/json" \
  -d '{
    "tool_id": "tool-id"
  }'
```

### Remove a Tool from an Agent

```bash
curl -s -X DELETE ${MULE_SERVER}/api/v1/agents/{agent-id}/tools/{tool-id}
```

---

## Managing Tools

Tools represent external or internal capabilities that can be used by agents.

### List Tools

```bash
curl -s ${MULE_SERVER}/api/v1/tools | jq .
```

### Create a Tool

```bash
curl -s -X POST ${MULE_SERVER}/api/v1/tools \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-tool",
    "description": "Description of what the tool does",
    "metadata": {
      "type": "function",
      "function": {
        "name": "my_function",
        "description": "What the function does",
        "parameters": {
          "type": "object",
          "properties": {
            "arg1": { "type": "string" }
          },
          "required": ["arg1"]
        }
      }
    }
  }' | jq .
```

### Get a Tool

```bash
curl -s ${MULE_SERVER}/api/v1/tools/{tool-id} | jq .
```

### Update a Tool

```bash
curl -s -X PUT ${MULE_SERVER}/api/v1/tools/{tool-id} \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-tool",
    "description": "Updated description",
    "metadata": {}
  }' | jq .
```

### Delete a Tool

```bash
curl -s -X DELETE ${MULE_SERVER}/api/v1/tools/{tool-id}
```

---

## Managing Workflows

Workflows are ordered sequences of steps that can include agent invocations and WASM module executions.

### List Workflows

```bash
curl -s ${MULE_SERVER}/api/v1/workflows | jq .
```

### Create a Workflow

```bash
curl -s -X POST ${MULE_SERVER}/api/v1/workflows \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-workflow",
    "description": "A multi-step workflow",
    "is_async": false
  }' | jq .
```

**Workflow fields:**
- `name` - Workflow name (required)
- `description` - Workflow description
- `is_async` - If true, execution returns immediately with job ID

### Get a Workflow

```bash
curl -s ${MULE_SERVER}/api/v1/workflows/{workflow-id} | jq .
```

### Update a Workflow

```bash
curl -s -X PUT ${MULE_SERVER}/api/v1/workflows/{workflow-id} \
  -H "Content-Type: application/json" \
  -d '{
    "name": "my-workflow",
    "description": "Updated description",
    "is_async": true
  }' | jq .
```

### Delete a Workflow

```bash
curl -s -X DELETE ${MULE_SERVER}/api/v1/workflows/{workflow-id}
```

---

## Managing Workflow Steps

Workflow steps define the individual actions within a workflow.

### List Workflow Steps

```bash
curl -s ${MULE_SERVER}/api/v1/workflows/{workflow-id}/steps | jq .
```

### Create a Workflow Step

```bash
curl -s -X POST ${MULE_SERVER}/api/v1/workflows/{workflow-id}/steps \
  -H "Content-Type: application/json" \
  -d '{
    "step_order": 1,
    "type": "AGENT",
    "agent_id": "agent-id",
    "config": {
      "prompt_template": "Process: {input}"
    }
  }' | jq .
```

**Step types:**
- `AGENT` - Invokes an agent
- `WASM` - Executes a WASM module

**Step fields:**
- `step_order` - Order of execution (1-based, auto-incremented if not specified)
- `type` - "AGENT" or "WASM"
- `agent_id` - ID of agent to invoke (required for AGENT type)
- `wasm_module_id` - ID of WASM module (required for WASM type)
- `config` - Step-specific configuration

### Update a Workflow Step

```bash
curl -s -X PUT ${MULE_SERVER}/api/v1/workflows/{workflow-id}/steps/{step-id} \
  -H "Content-Type: application/json" \
  -d '{
    "step_order": 2,
    "type": "AGENT",
    "agent_id": "new-agent-id",
    "config": {}
  }' | jq .
```

### Delete a Workflow Step

```bash
curl -s -X DELETE ${MULE_SERVER}/api/v1/workflows/{workflow-id}/steps/{step-id}
```

### Reorder Workflow Steps

```bash
curl -s -X POST ${MULE_SERVER}/api/v1/workflows/{workflow-id}/steps/reorder \
  -H "Content-Type: application/json" \
  -d '{
    "step_ids": ["step-id-1", "step-id-2", "step-id-3"]
  }' | jq .
```

---

## Managing WASM Modules

WASM modules are WebAssembly modules that can be executed as part of workflows.

### List WASM Modules

```bash
curl -s ${MULE_SERVER}/api/v1/wasm-modules | jq .
```

### Create a WASM Module

```bash
curl -s -X POST ${MULE_SERVER}/api/v1/wasm-modules \
  -F "name=my-module" \
  -F "description=Description" \
  -F "config={}" \
  -F "module_data@module.wasm" | jq .
```

### Get a WASM Module

```bash
curl -s ${MULE_SERVER}/api/v1/wasm-modules/{module-id} | jq .
```

### Update a WASM Module

```bash
curl -s -X PUT ${MULE_SERVER}/api/v1/wasm-modules/{module-id} \
  -F "name=updated-name" \
  -F "description=Updated description" \
  -F "config={}" | jq .
```

### Delete a WASM Module

```bash
curl -s -X DELETE ${MULE_SERVER}/api/v1/wasm-modules/{module-id}
```

### Get WASM Module Source

```bash
curl -s ${MULE_SERVER}/api/v1/wasm-modules/{module-id}/source | jq .
```

### Update WASM Module Source

```bash
curl -s -X PUT ${MULE_SERVER}/api/v1/wasm-modules/{module-id}/source \
  -F "source=// go source code" | jq .
```

---

## Managing Jobs

Jobs represent workflow or WASM execution instances.

### List Jobs

```bash
curl -s "${MULE_SERVER}/api/v1/jobs?page=1&page_size=20" | jq .
```

### Create a Job

```bash
curl -s -X POST ${MULE_SERVER}/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "workflow_id": "workflow-id",
    "input_data": {
      "key": "value"
    },
    "working_directory": "/optional/path"
  }' | jq .
```

### Get a Job

```bash
curl -s ${MULE_SERVER}/api/v1/jobs/{job-id} | jq .
```

### Get Job Steps

```bash
curl -s ${MULE_SERVER}/api/v1/jobs/{job-id}/steps | jq .
```

### Cancel a Job

```bash
curl -s -X DELETE ${MULE_SERVER}/api/v1/jobs/{job-id}
```

---

## Common Workflow Examples

### Example 1: Create a Simple Agent Workflow

1. **Create a provider** (if not exists):
   ```bash
   curl -X POST ${MULE_SERVER}/api/v1/providers \
     -H "Content-Type: application/json" \
     -d '{"name": "local-llm", "api_base_url": "http://localhost:11434/v1", "api_key_encrypted": "test"}'
   ```

2. **Create an agent**:
   ```bash
   curl -X POST ${MULE_SERVER}/api/v1/agents \
     -H "Content-Type: application/json" \
     -d '{
       "name": "assistant",
       "provider_id": "provider-id",
       "model_id": "llama3",
       "system_prompt": "You are a helpful assistant."
     }'
   ```

3. **Execute the agent**:
   ```bash
   curl -X POST ${MULE_SERVER}/v1/chat/completions \
     -H "Content-Type: application/json" \
     -d '{
       "model": "agent/assistant",
       "messages": [{"role": "user", "content": "Hello!"}]
     }'
   ```

### Example 2: Create a Multi-Step Workflow

1. **Create a workflow**:
   ```bash
   curl -X POST ${MULE_SERVER}/api/v1/workflows \
     -H "Content-Type: application/json" \
     -d '{"name": "analyze-data", "description": "Data analysis workflow", "is_async": false}'
   ```

2. **Add step 1 - Data Collection Agent**:
   ```bash
   curl -X POST ${MULE_SERVER}/api/v1/workflows/{workflow-id}/steps \
     -H "Content-Type: application/json" \
     -d '{"step_order": 1, "type": "AGENT", "agent_id": "collector-agent-id", "config": {}}'
   ```

3. **Add step 2 - Data Processing Agent**:
   ```bash
   curl -X POST ${MULE_SERVER}/api/v1/workflows/{workflow-id}/steps \
     -H "Content-Type: application/json" \
     -d '{"step_order": 2, "type": "AGENT", "agent_id": "processor-agent-id", "config": {}}'
   ```

4. **Execute the workflow**:
   ```bash
   curl -X POST ${MULE_SERVER}/v1/chat/completions \
     -H "Content-Type: application/json" \
     -d '{
       "model": "workflow/analyze-data",
       "messages": [{"role": "user", "content": "Analyze this dataset..."}]
     }'
   ```

### Example 3: Async Workflow Execution

1. **Create an async workflow**:
   ```bash
   curl -X POST ${MULE_SERVER}/api/v1/workflows \
     -H "Content-Type: application/json" \
     -d '{"name": "long-task", "is_async": true}'
   ```

2. **Execute asynchronously**:
   ```bash
   curl -X POST ${MULE_SERVER}/v1/chat/completions \
     -H "Content-Type: application/json" \
     -d '{
       "model": "async/workflow/long-task",
       "messages": [{"role": "user", "content": "Start long task"}]
     }'
   ```

3. **Check job status**:
   ```bash
   # Get job ID from response, then:
   curl ${MULE_SERVER}/api/v1/jobs/{job-id}
   ```

---

## Error Handling

Common error responses:

- **400 Bad Request** - Invalid request body or parameters
- **404 Not Found** - Resource not found
- **409 Conflict** - Duplicate resource (e.g., provider name already exists)
- **500 Internal Server Error** - Server-side error

Error responses include a message describing the issue.

---

## WebSocket for Real-Time Updates

Connect to WebSocket for real-time job updates:

```bash
# Connect to WS endpoint
ws://${MULE_SERVER}/ws
```

The WebSocket sends job status updates as they occur.
