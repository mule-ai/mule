# Mule API Documentation

Mule provides comprehensive API access through both high-performance gRPC and standard HTTP REST interfaces. These APIs enable external services, applications, and tools to integrate with Mule's multi-agent workflows, manage resources, and access advanced AI capabilities.

## 🚀 Overview

Mule offers two complementary API interfaces:

| API Type | Port | Protocol | Use Case |
|----------|------|----------|----------|
| **gRPC** | 9090 | HTTP/2 + Protocol Buffers | High-performance, type-safe, streaming |
| **HTTP REST** | 8083 | HTTP/1.1 + JSON | Web applications, simple integrations |

Both APIs provide access to:
- **Workflow Management**: Execute and monitor multi-agent workflows
- **Agent Operations**: Manage and interact with AI agents  
- **Provider Management**: Access AI model providers
- **Integration Control**: Trigger and manage integrations
- **System Monitoring**: Health checks and status monitoring

## 🔌 gRPC API

### Service Definition

The Mule gRPC service provides a comprehensive interface for workflow and agent management:

```protobuf
service MuleService {
  // System Health
  rpc GetHeartbeat(HeartbeatRequest) returns (HeartbeatResponse);
  
  // Workflow Management
  rpc ListWorkflows(ListWorkflowsRequest) returns (ListWorkflowsResponse);
  rpc GetWorkflow(GetWorkflowRequest) returns (GetWorkflowResponse);
  rpc ExecuteWorkflow(ExecuteWorkflowRequest) returns (ExecuteWorkflowResponse);
  rpc ListRunningWorkflows(ListRunningWorkflowsRequest) returns (ListRunningWorkflowsResponse);
  
  // Agent Management
  rpc ListAgents(ListAgentsRequest) returns (ListAgentsResponse);
  rpc GetAgent(GetAgentRequest) returns (GetAgentResponse);
  
  // Provider Management
  rpc ListProviders(ListProvidersRequest) returns (ListProvidersResponse);
}
```

### Connection Configuration

**Default Configuration:**
```json
{
  "grpc": {
    "enabled": true,
    "host": "0.0.0.0",
    "port": 9090
  }
}
```

**Environment Variables:**
```bash
export MULE_GRPC_HOST="0.0.0.0"
export MULE_GRPC_PORT="9090"
export MULE_GRPC_ENABLED="true"
```

### Client Examples

#### Go Client

```go
package main

import (
    "context"
    "log"
    "time"
    
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    pb "github.com/mule-ai/mule/api/proto"
)

func main() {
    // Connect to Mule gRPC server
    conn, err := grpc.NewClient("localhost:9090", grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil {
        log.Fatalf("Failed to connect: %v", err)
    }
    defer conn.Close()
    
    client := pb.NewMuleServiceClient(conn)
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    // Health check
    heartbeat, err := client.GetHeartbeat(ctx, &pb.HeartbeatRequest{})
    if err != nil {
        log.Fatalf("Heartbeat failed: %v", err)
    }
    log.Printf("Mule status: %s, Version: %s", heartbeat.Status, heartbeat.Version)
    
    // List workflows
    workflows, err := client.ListWorkflows(ctx, &pb.ListWorkflowsRequest{})
    if err != nil {
        log.Fatalf("Failed to list workflows: %v", err)
    }
    
    for _, workflow := range workflows.Workflows {
        log.Printf("Workflow: %s (%s)", workflow.Name, workflow.Id)
    }
    
    // Execute workflow
    execution, err := client.ExecuteWorkflow(ctx, &pb.ExecuteWorkflowRequest{
        WorkflowName: "code_generation",
        Prompt:       "Create a REST API endpoint for user management",
        Path:         "/path/to/project",
    })
    if err != nil {
        log.Fatalf("Failed to execute workflow: %v", err)
    }
    
    log.Printf("Workflow execution started: %s", execution.ExecutionId)
}
```

#### Python Client

```python
import grpc
import mule_pb2
import mule_pb2_grpc
from google.protobuf.json_format import MessageToDict

def main():
    # Connect to Mule gRPC server
    channel = grpc.insecure_channel('localhost:9090')
    client = mule_pb2_grpc.MuleServiceStub(channel)
    
    # Health check
    heartbeat = client.GetHeartbeat(mule_pb2.HeartbeatRequest())
    print(f"Mule status: {heartbeat.status}, Version: {heartbeat.version}")
    
    # List agents
    agents_response = client.ListAgents(mule_pb2.ListAgentsRequest())
    
    for agent in agents_response.agents:
        print(f"Agent: {agent.name} (ID: {agent.id})")
        print(f"  Provider: {agent.provider_name}")
        print(f"  Model: {agent.model}")
        print(f"  Tools: {', '.join(agent.tools)}")
    
    # Execute workflow
    execution = client.ExecuteWorkflow(mule_pb2.ExecuteWorkflowRequest(
        workflow_name="code_generation",
        prompt="Implement a new feature for user authentication",
        path="/path/to/project"
    ))
    
    print(f"Workflow execution started: {execution.execution_id}")
    
    # Monitor execution
    running_workflows = client.ListRunningWorkflows(mule_pb2.ListRunningWorkflowsRequest())
    
    for workflow in running_workflows.running_workflows:
        if workflow.execution_id == execution.execution_id:
            print(f"Status: {workflow.status}")
            print(f"Current step: {workflow.current_step}")

if __name__ == "__main__":
    main()
```

#### Node.js Client

```javascript
const grpc = require('@grpc/grpc-js');
const protoLoader = require('@grpc/proto-loader');

const packageDefinition = protoLoader.loadSync('mule.proto', {
    keepCase: true,
    longs: String,
    enums: String,
    defaults: true,
    oneofs: true
});

const muleProto = grpc.loadPackageDefinition(packageDefinition).mule.v1;

async function main() {
    const client = new muleProto.MuleService('localhost:9090', grpc.credentials.createInsecure());
    
    // Health check
    client.getHeartbeat({}, (error, response) => {
        if (error) {
            console.error('Heartbeat failed:', error);
            return;
        }
        console.log(`Mule status: ${response.status}, Version: ${response.version}`);
    });
    
    // List workflows
    client.listWorkflows({}, (error, response) => {
        if (error) {
            console.error('Failed to list workflows:', error);
            return;
        }
        
        response.workflows.forEach(workflow => {
            console.log(`Workflow: ${workflow.name} (${workflow.id})`);
            console.log(`  Description: ${workflow.description}`);
            console.log(`  Steps: ${workflow.steps.length}`);
        });
    });
    
    // Execute workflow
    const executeRequest = {
        workflow_name: 'code_generation',
        prompt: 'Create a microservice for data processing',
        path: '/path/to/project'
    };
    
    client.executeWorkflow(executeRequest, (error, response) => {
        if (error) {
            console.error('Failed to execute workflow:', error);
            return;
        }
        
        console.log(`Workflow execution started: ${response.execution_id}`);
    });
}

main().catch(console.error);
```

### Message Types

#### Workflow Messages

```protobuf
message Workflow {
  string id = 1;
  string name = 2;
  string description = 3;
  bool is_default = 4;
  repeated WorkflowStep steps = 5;
  repeated string validation_functions = 6;
  repeated TriggerSettings triggers = 7;
  repeated TriggerSettings outputs = 8;
}

message WorkflowStep {
  string id = 1;
  int32 agent_id = 2;
  string agent_name = 3;
  string output_field = 4;
  TriggerSettings integration = 5;
}

message ExecuteWorkflowRequest {
  string workflow_name = 1;
  string prompt = 2;
  string path = 3;
}

message ExecuteWorkflowResponse {
  string execution_id = 1;
  string status = 2;
  string message = 3;
}
```

#### Agent Messages

```protobuf
message Agent {
  int32 id = 1;
  string name = 2;
  string provider_name = 3;
  string model = 4;
  string prompt_template = 5;
  string system_prompt = 6;
  repeated string tools = 7;
  UDiffSettings udiff_settings = 8;
}

message UDiffSettings {
  bool enabled = 1;
}
```

#### Running Workflow Monitoring

```protobuf
message RunningWorkflow {
  string execution_id = 1;
  string workflow_name = 2;
  string status = 3;
  google.protobuf.Timestamp started_at = 4;
  repeated WorkflowStepResult step_results = 5;
  string current_step = 6;
}

message WorkflowStepResult {
  string step_id = 1;
  string status = 2;
  string content = 3;
  string error_message = 4;
  google.protobuf.Timestamp completed_at = 5;
}
```

## 🌐 HTTP REST API

### Base Configuration

**Default Configuration:**
```json
{
  "api": {
    "enabled": true,
    "port": 8083,
    "host": "0.0.0.0",
    "cors": true
  }
}
```

**Base URL:** `http://localhost:8083`

### Core Endpoints

#### System Health

**GET /api/health**
```bash
curl http://localhost:8083/api/health
```

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:00Z",
  "version": "v1.2.0",
  "components": {
    "grpc": "healthy",
    "integrations": "healthy",
    "workflows": "healthy"
  }
}
```

#### Repository Management

**GET /api/repositories**
```bash
curl http://localhost:8083/api/repositories
```

**Response:**
```json
{
  "repositories": [
    {
      "id": "repo1",
      "name": "mule-ai/mule",
      "path": "/path/to/repo",
      "provider": "github",
      "status": "active"
    }
  ]
}
```

**POST /api/repositories/clone**
```bash
curl -X POST http://localhost:8083/api/repositories/clone \
  -H "Content-Type: application/json" \
  -d '{
    "url": "https://github.com/user/repo.git",
    "path": "/local/path",
    "branch": "main"
  }'
```

**POST /api/repositories/sync**
```bash
curl -X POST http://localhost:8083/api/repositories/sync \
  -H "Content-Type: application/json" \
  -d '{
    "repository_id": "repo1",
    "force": false
  }'
```

#### Agent Management

**GET /api/agents**
```bash
curl http://localhost:8083/api/agents
```

**Response:**
```json
{
  "agents": [
    {
      "id": 10,
      "name": "code",
      "provider_name": "ollama",
      "model": "qwen2.5-coder:32b",
      "tools": ["revertFile", "tree", "readFile"],
      "udiff_settings": {
        "enabled": true
      }
    },
    {
      "id": 11,
      "name": "architect",
      "provider_name": "ollama", 
      "model": "qwq:32b-q8_0",
      "tools": ["tree", "readFile"]
    }
  ]
}
```

**GET /api/agents/{id}**
```bash
curl http://localhost:8083/api/agents/10
```

**POST /api/agents**
```bash
curl -X POST http://localhost:8083/api/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "reviewer",
    "provider_name": "openai",
    "model": "gpt-4",
    "system_prompt": "You are a code reviewer...",
    "tools": ["readFile", "tree"]
  }'
```

#### Workflow Management

**GET /api/workflows**
```bash
curl http://localhost:8083/api/workflows
```

**Response:**
```json
{
  "workflows": [
    {
      "id": "workflow_code_generation",
      "name": "Code Generation",
      "description": "This is a simple code generation workflow",
      "is_default": true,
      "steps": [
        {
          "id": "step_architect",
          "agent_id": 11,
          "agent_name": "architect",
          "output_field": "generatedText"
        },
        {
          "id": "step_code_generation",
          "agent_id": 10,
          "agent_name": "code", 
          "output_field": "generatedText"
        }
      ],
      "validation_functions": [
        "goFmt",
        "goModTidy",
        "golangciLint",
        "goTest"
      ]
    }
  ]
}
```

**POST /api/workflows/execute**
```bash
curl -X POST http://localhost:8083/api/workflows/execute \
  -H "Content-Type: application/json" \
  -d '{
    "workflow_id": "workflow_code_generation",
    "input": "Create a REST API for user management",
    "path": "/path/to/project"
  }'
```

**Response:**
```json
{
  "execution_id": "exec_12345",
  "status": "started",
  "workflow_id": "workflow_code_generation",
  "started_at": "2024-01-15T10:30:00Z"
}
```

**GET /api/workflows/executions**
```bash
curl http://localhost:8083/api/workflows/executions
```

**GET /api/workflows/executions/{id}**
```bash
curl http://localhost:8083/api/workflows/executions/exec_12345
```

**Response:**
```json
{
  "execution_id": "exec_12345",
  "workflow_id": "workflow_code_generation",
  "status": "running",
  "started_at": "2024-01-15T10:30:00Z",
  "current_step": "step_code_generation",
  "steps": [
    {
      "step_id": "step_architect",
      "status": "completed",
      "output": "Analysis complete...",
      "completed_at": "2024-01-15T10:31:00Z"
    },
    {
      "step_id": "step_code_generation", 
      "status": "running",
      "started_at": "2024-01-15T10:31:00Z"
    }
  ]
}
```

#### Provider Management

**GET /api/providers**
```bash
curl http://localhost:8083/api/providers
```

**Response:**
```json
{
  "providers": [
    {
      "name": "ollama",
      "type": "ollama",
      "status": "connected",
      "models": [
        "qwen2.5-coder:32b",
        "qwq:32b-q8_0",
        "gemma3:27b"
      ]
    },
    {
      "name": "openai",
      "type": "openai", 
      "status": "connected",
      "models": [
        "gpt-4",
        "gpt-3.5-turbo"
      ]
    }
  ]
}
```

**GET /api/models**
```bash
curl http://localhost:8083/api/models
```

**Response:**
```json
{
  "models": [
    {
      "name": "qwen2.5-coder:32b",
      "provider": "ollama",
      "type": "chat",
      "context_length": 32768
    },
    {
      "name": "gpt-4",
      "provider": "openai",
      "type": "chat", 
      "context_length": 8192
    }
  ]
}
```

#### Integration Management

**GET /api/integrations**
```bash
curl http://localhost:8083/api/integrations
```

**Response:**
```json
{
  "integrations": [
    {
      "name": "discord",
      "type": "chat",
      "status": "connected",
      "config": {
        "enabled": true,
        "channels": ["123456789012345678"]
      }
    },
    {
      "name": "matrix-default",
      "type": "chat",
      "status": "connected",
      "config": {
        "enabled": true,
        "rooms": ["!room:matrix.org"]
      }
    }
  ]
}
```

**POST /api/integrations/trigger**
```bash
curl -X POST http://localhost:8083/api/integrations/trigger \
  -H "Content-Type: application/json" \
  -d '{
    "integration": "discord",
    "event": "sendMessage",
    "data": {
      "content": "Hello from Mule API!",
      "channelId": "123456789012345678"
    }
  }'
```

#### Tool Management

**GET /api/tools**
```bash
curl http://localhost:8083/api/tools
```

**Response:**
```json
{
  "tools": [
    {
      "name": "readFile",
      "description": "Read contents of a file",
      "parameters": {
        "path": "string"
      }
    },
    {
      "name": "tree",
      "description": "Show directory tree structure",
      "parameters": {
        "path": "string",
        "depth": "integer"
      }
    }
  ]
}
```

### RSS Integration Endpoints

**GET /rss**
```bash
curl http://localhost:8083/rss
```
Returns RSS XML feed.

**GET /rss-index**
```bash
curl http://localhost:8083/rss-index
```
Returns HTML interface for browsing RSS feeds.

**POST /rss/add**
```bash
curl -X POST http://localhost:8083/rss/add \
  -H "Content-Type: application/json" \
  -d '{
    "title": "New Item",
    "description": "Item description",
    "link": "https://example.com",
    "author": "Author Name"
  }'
```

## 🔐 Authentication & Security

### API Authentication

For production deployments, enable authentication:

```json
{
  "api": {
    "auth": {
      "enabled": true,
      "type": "bearer",
      "secret": "${API_SECRET}"
    }
  }
}
```

**Usage with Authentication:**
```bash
curl -H "Authorization: Bearer ${API_TOKEN}" \
  http://localhost:8083/api/workflows
```

### gRPC Authentication

For gRPC with authentication:

```go
// Add authentication metadata
md := metadata.New(map[string]string{
    "authorization": "bearer " + token,
})
ctx = metadata.NewOutgoingContext(ctx, md)

// Make authenticated call
response, err := client.ListWorkflows(ctx, &pb.ListWorkflowsRequest{})
```

### Rate Limiting

Configure rate limiting:

```json
{
  "api": {
    "rateLimit": {
      "enabled": true,
      "requestsPerMinute": 100,
      "burstSize": 20
    }
  }
}
```

### CORS Configuration

Configure CORS for web applications:

```json
{
  "api": {
    "cors": {
      "enabled": true,
      "allowedOrigins": ["http://localhost:3000"],
      "allowedMethods": ["GET", "POST", "PUT", "DELETE"],
      "allowedHeaders": ["Content-Type", "Authorization"]
    }
  }
}
```

## 📊 Monitoring & Metrics

### Health Check Endpoint

**GET /health**
```bash
curl http://localhost:8083/health
```

**Response:**
```json
{
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:00Z",
  "checks": {
    "database": "healthy",
    "integrations": "healthy",
    "workflows": "healthy"
  }
}
```

### Metrics Endpoint

**GET /metrics**
```bash
curl http://localhost:8083/metrics
```

**Response (Prometheus format):**
```
# HELP mule_workflows_total Total number of workflow executions
# TYPE mule_workflows_total counter
mule_workflows_total{status="success"} 150
mule_workflows_total{status="failed"} 5

# HELP mule_workflow_duration_seconds Workflow execution duration
# TYPE mule_workflow_duration_seconds histogram
mule_workflow_duration_seconds_bucket{le="1"} 50
mule_workflow_duration_seconds_bucket{le="5"} 120
mule_workflow_duration_seconds_bucket{le="10"} 145
```

### Logging Endpoint

**GET /api/logs**
```bash
curl http://localhost:8083/api/logs?level=info&since=1h
```

**Response:**
```json
{
  "logs": [
    {
      "timestamp": "2024-01-15T10:30:00Z",
      "level": "info",
      "message": "Workflow execution started",
      "workflow_id": "workflow_code_generation",
      "execution_id": "exec_12345"
    }
  ]
}
```

## 🔄 Streaming & Real-time Updates

### WebSocket Support

Connect to real-time workflow updates:

```javascript
const ws = new WebSocket('ws://localhost:8083/api/ws/workflows');

ws.onmessage = function(event) {
    const update = JSON.parse(event.data);
    console.log('Workflow update:', update);
};

// Subscribe to specific workflow
ws.send(JSON.stringify({
    type: 'subscribe',
    execution_id: 'exec_12345'
}));
```

### Server-Sent Events

Monitor workflow progress via SSE:

```bash
curl -N http://localhost:8083/api/stream/workflows/exec_12345
```

**Response:**
```
data: {"type": "step_started", "step_id": "step_architect", "timestamp": "2024-01-15T10:30:00Z"}

data: {"type": "step_completed", "step_id": "step_architect", "output": "Analysis complete", "timestamp": "2024-01-15T10:31:00Z"}

data: {"type": "workflow_completed", "execution_id": "exec_12345", "status": "success", "timestamp": "2024-01-15T10:35:00Z"}
```

## 🛠 SDK & Client Libraries

### Official SDKs

#### Go SDK

```go
import "github.com/mule-ai/mule-go-sdk"

client := mule.NewClient("localhost:8083")

// Execute workflow
execution, err := client.ExecuteWorkflow(ctx, &mule.ExecuteWorkflowRequest{
    WorkflowID: "code_generation",
    Input:      "Create a new feature",
    Path:       "/project/path",
})
```

#### Python SDK

```python
from mule_sdk import MuleClient

client = MuleClient("http://localhost:8083")

# Execute workflow
execution = client.execute_workflow(
    workflow_id="code_generation",
    input="Create a new feature",
    path="/project/path"
)

# Monitor progress
for update in client.stream_workflow(execution.execution_id):
    print(f"Step {update.step_id}: {update.status}")
```

#### JavaScript SDK

```javascript
import { MuleClient } from '@mule-ai/sdk';

const client = new MuleClient('http://localhost:8083');

// Execute workflow
const execution = await client.executeWorkflow({
    workflowId: 'code_generation',
    input: 'Create a new feature',
    path: '/project/path'
});

// Monitor progress
client.streamWorkflow(execution.executionId)
    .on('update', (update) => {
        console.log(`Step ${update.stepId}: ${update.status}`);
    });
```

## 🔧 Error Handling

### HTTP Error Codes

| Code | Description | Response |
|------|-------------|----------|
| 200 | Success | Request completed successfully |
| 400 | Bad Request | Invalid request parameters |
| 401 | Unauthorized | Authentication required |
| 403 | Forbidden | Insufficient permissions |
| 404 | Not Found | Resource not found |
| 429 | Rate Limited | Too many requests |
| 500 | Internal Error | Server error |

### Error Response Format

```json
{
  "error": {
    "code": "WORKFLOW_NOT_FOUND",
    "message": "Workflow 'invalid_workflow' not found",
    "details": {
      "available_workflows": ["code_generation", "validation_pipeline"]
    },
    "timestamp": "2024-01-15T10:30:00Z",
    "request_id": "req_12345"
  }
}
```

### gRPC Error Handling

```go
import "google.golang.org/grpc/status"

resp, err := client.ExecuteWorkflow(ctx, req)
if err != nil {
    if st, ok := status.FromError(err); ok {
        switch st.Code() {
        case codes.NotFound:
            log.Printf("Workflow not found: %s", st.Message())
        case codes.InvalidArgument:
            log.Printf("Invalid request: %s", st.Message())
        default:
            log.Printf("gRPC error: %s", st.Message())
        }
    }
}
```

## 📚 Integration Examples

### Webhook Integration

Set up webhooks to trigger workflows:

```bash
# Configure webhook endpoint
curl -X POST http://localhost:8083/api/webhooks \
  -H "Content-Type: application/json" \
  -d '{
    "url": "/webhook/github",
    "workflow_id": "issue_processor",
    "events": ["issues.opened", "pull_request.opened"]
  }'
```

### CI/CD Integration

Use Mule in CI/CD pipelines:

```yaml
# .github/workflows/mule-analysis.yml
name: Code Analysis with Mule
on: [pull_request]

jobs:
  analyze:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v2
      
      - name: Execute Mule Workflow
        run: |
          curl -X POST http://mule-server:8083/api/workflows/execute \
            -H "Content-Type: application/json" \
            -H "Authorization: Bearer ${{ secrets.MULE_API_TOKEN }}" \
            -d '{
              "workflow_id": "code_review",
              "input": "${{ github.event.pull_request.title }}",
              "path": "."
            }'
```

### Monitoring Integration

Integrate with monitoring systems:

```bash
# Prometheus scraping configuration
- job_name: 'mule'
  static_configs:
    - targets: ['localhost:8083']
  metrics_path: '/metrics'
  scrape_interval: 30s
```

## 🚀 Performance & Scalability

### Connection Pooling

Configure connection pooling for high-throughput:

```go
// gRPC connection pool
pool := grpcpool.New(func() (*grpc.ClientConn, error) {
    return grpc.Dial("localhost:9090", grpc.WithInsecure())
}, 10, 100, time.Hour)

// HTTP client with connection pooling
client := &http.Client{
    Transport: &http.Transport{
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 10,
        IdleConnTimeout:     90 * time.Second,
    },
}
```

### Async Processing

Use async patterns for better performance:

```javascript
// Async workflow execution
const promises = workflowIds.map(id => 
    client.executeWorkflow({
        workflowId: id,
        input: inputs[id]
    })
);

const results = await Promise.all(promises);
```

### Caching

Implement caching for frequently accessed data:

```bash
# Cache workflow definitions
curl -H "Cache-Control: max-age=3600" \
  http://localhost:8083/api/workflows
```

---

*This API documentation provides comprehensive guidance for integrating with Mule's powerful multi-agent platform. Both gRPC and REST APIs are production-ready with robust error handling, monitoring, and scalability features.*