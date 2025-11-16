# Mule v2 - Data Model Diagram

```mermaid
erDiagram
    PROVIDERS {
        string id PK
        string name
        string api_base_url
        string api_key_encrypted
        timestamp created_at
        timestamp updated_at
    }
    
    TOOLS {
        string id PK
        string name
        string description
        string type
        json config
        timestamp created_at
        timestamp updated_at
    }
    
    AGENTS {
        string id PK
        string name
        string description
        string provider_id FK
        string model_id
        string system_prompt
        timestamp created_at
        timestamp updated_at
    }
    
    AGENT_TOOLS {
        string agent_id FK
        string tool_id FK
    }
    
    WORKFLOWS {
        string id PK
        string name
        string description
        boolean is_async
        timestamp created_at
        timestamp updated_at
    }
    
    WORKFLOW_STEPS {
        string id PK
        string workflow_id FK
        int step_order
        string type "AGENT|WASM"
        string agent_id FK
        string wasm_module_id FK
        json config
        timestamp created_at
    }
    
    WASM_MODULES {
        string id PK
        string name
        string description
        bytea module_data
        timestamp created_at
        timestamp updated_at
    }
    
    JOBS {
        string id PK
        string workflow_id FK
        string status "QUEUED|RUNNING|COMPLETED|FAILED"
        json input_data
        json output_data
        timestamp created_at
        timestamp started_at
        timestamp completed_at
    }
    
    JOB_STEPS {
        string id PK
        string job_id FK
        string workflow_step_id FK
        string status "PENDING|RUNNING|COMPLETED|FAILED"
        json input_data
        json output_data
        timestamp started_at
        timestamp completed_at
    }
    
    ARTIFACTS {
        string id PK
        string job_id FK
        string name
        string mime_type
        bytea data
        timestamp created_at
    }
    
    PROVIDERS ||--o{ AGENTS : "has"
    AGENTS ||--o{ AGENT_TOOLS : "uses"
    TOOLS ||--o{ AGENT_TOOLS : "provided_to"
    WORKFLOWS ||--o{ WORKFLOW_STEPS : "contains"
    WORKFLOW_STEPS ||--o{ AGENTS : "invokes"
    WORKFLOW_STEPS ||--o{ WASM_MODULES : "executes"
    WORKFLOWS ||--o{ JOBS : "executes"
    JOBS ||--o{ JOB_STEPS : "has"
    JOBS ||--o{ ARTIFACTS : "produces"
```

## Entity Descriptions

### PROVIDERS
Stores configuration for AI providers (OpenAI-compatible APIs):
- `api_base_url`: Base URL for the API (e.g., https://api.openai.com/v1)
- `api_key_encrypted`: Encrypted API key for secure storage

### TOOLS
Represents available tools that can be used by agents:
- `type`: Type of tool (e.g., "http", "database", "memory")
- `config`: JSON configuration specific to the tool type

### AGENTS
AI agents combining a model, system prompt, and tools:
- `provider_id`: Reference to the AI provider
- `model_id`: Identifier for the specific model to use
- `system_prompt`: Instructions that define the agent's behavior

### AGENT_TOOLS
Many-to-many relationship between agents and tools.

### WORKFLOWS
Definition of ordered workflow executions:
- `is_async`: Whether executions should be asynchronous by default

### WORKFLOW_STEPS
Individual steps within workflows:
- `type`: Either "AGENT" for agent invocation or "WASM" for WASM execution
- `agent_id`: Reference to agent for agent steps
- `wasm_module_id`: Reference to WASM module for WASM steps
- `step_order`: Position in the workflow sequence

### WASM_MODULES
WebAssembly modules that can be executed as workflow steps:
- `module_data`: Binary data of the compiled WASM module

### JOBS
Instances of workflow executions:
- `status`: Current execution status
- `input_data`: Data provided when starting the job
- `output_data`: Results from the completed job

### JOB_STEPS
Execution records for individual workflow steps within jobs:
- `status`: Status of this specific step execution
- `input_data`: Data passed to this step
- `output_data`: Results from this step

### ARTIFACTS
Persistent data produced during job executions:
- `mime_type`: MIME type of the artifact data
- `data`: Binary content of the artifact