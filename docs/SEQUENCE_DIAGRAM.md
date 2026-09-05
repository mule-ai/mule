# Mule v2 - Workflow Execution Sequence Diagram

```mermaid
sequenceDiagram
    participant U as User
    participant A as API Server
    participant Q as PostgreSQL (Job Queue)
    participant W as Workflow Workers
    participant ADK as Google ADK Runtime
    participant WR as WASM Runtime
    participant AP as AI Provider
    
    %% Synchronous Workflow Execution
    U->>A: POST /v1/chat/completions {model: "workflow/xyz", messages: [...]}
    A->>Q: Insert job record with status=QUEUED
    A->>W: Notify of new job (or workers poll)
    W->>Q: Claim job (update status=RUNNING)
    loop For each workflow step
        W->>Q: Get step configuration
        alt If step type is AGENT
            W->>ADK: Initialize agent with tools
            ADK->>AP: Call model via provider
            AP-->>ADK: Return response
            ADK->>Q: Store step result
        else If step type is WASM
            W->>WR: Load and execute WASM module
            WR-->>W: Return execution result
            W->>Q: Store step result
        end
    end
    W->>Q: Update job status=COMPLETED
    A->>Q: Poll for job completion
    A-->>U: Return final result
    
    %% Asynchronous Workflow Execution
    note over U,A: For async execution, model name starts with "async/"
    U->>A: POST /v1/chat/completions {model: "async/workflow/xyz", messages: [...]}
    A->>Q: Insert job record with status=QUEUED
    A-->>U: Return {"id": "job-123", "status": "queued", "message": "The workflow has been started"}
    
    %% Agent Direct Execution
    U->>A: POST /v1/chat/completions {model: "agent/abc", messages: [...]}
    A->>ADK: Initialize agent with tools
    ADK->>AP: Call model via provider
    AP-->>ADK: Return response
    ADK-->>A: Return agent response
    A-->>U: Return response directly
    
    %% WASM Direct Execution (Future Enhancement)
    note over W,WR: Direct WASM execution could be supported in future
    U->>A: POST /v1/wasm/execute {module: "module-id", input: {...}}
    A->>WR: Load and execute WASM module
    WR-->>A: Return execution result
    A-->>U: Return result
```

## Flow Description

### Synchronous Workflow Execution
1. User sends a request to execute a workflow synchronously
2. API server creates a job record in PostgreSQL with QUEUED status
3. Workflow workers pick up the job and begin execution
4. For each step in the workflow:
   - If it's an agent step, the worker initializes the agent with its tools and executes it via the AI provider
   - If it's a WASM step, the worker loads and executes the WASM module
5. Results from each step are stored in the database
6. When all steps complete, the job status is updated to COMPLETED
7. API server returns the final result to the user

### Asynchronous Workflow Execution
1. User sends a request to execute a workflow asynchronously (using "async/" prefix)
2. API server creates a job record and immediately returns a job ID to the user
3. Workflow execution proceeds in the background as described above
4. User can check the job status using the job ID

### Direct Agent Execution
1. User sends a request to execute an agent directly
2. API server initializes the agent and executes it immediately
3. Response is returned directly to the user without job queuing

## Key Components Interaction

- **API Server**: Handles incoming requests, manages job creation and result retrieval
- **PostgreSQL**: Acts as both persistent storage and job queue
- **Workflow Workers**: Execute workflow steps, managing the orchestration
- **Google ADK Runtime**: Provides the framework for agent execution
- **WASM Runtime**: Executes WebAssembly modules securely
- **AI Providers**: External services that provide the actual AI model capabilities