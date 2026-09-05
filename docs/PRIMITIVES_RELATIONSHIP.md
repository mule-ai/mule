# Mule v2 - Core Primitives Relationship Diagram

```mermaid
graph LR
    subgraph "Configuration Primitives"
        direction TB
        Provider["AI Provider<br/>• API Base URL<br/>• API Key<br/>• Model Discovery"]
        Tool["Tool<br/>• Name/Description<br/>• Type<br/>• Configuration"]
        Agent["Agent<br/>• Provider + Model<br/>• System Prompt<br/>• Tools Collection"]
        WASMModule["WASM Module<br/>• Binary Code<br/>• Execution Context"]
        Workflow["Workflow<br/>• Ordered Steps<br/>• Configuration"]
    end
    
    subgraph "Execution Primitives"
        direction TB
        WorkflowStep["Workflow Step<br/>• Agent Invocation OR<br/>• WASM Execution"]
        Job["Job<br/>• Workflow Instance<br/>• Execution Status<br/>• Input/Output Data"]
    end
    
    Provider -- "Provides models for" --> Agent
    Tool -- "Used by" --> Agent
    Agent -- "Composed of" --> WorkflowStep
    WASMModule -- "Executed as" --> WorkflowStep
    Workflow -- "Contains ordered" --> WorkflowStep
    Workflow -- "Instantiated as" --> Job
    
    style Provider fill:#FFE4B5,stroke:#333
    style Tool fill:#87CEEB,stroke:#333
    style Agent fill:#98FB98,stroke:#333
    style WASMModule fill:#FFB6C1,stroke:#333
    style Workflow fill:#DDA0DD,stroke:#333
    style WorkflowStep fill:#F0E68C,stroke:#333
    style Job fill:#87CEFA,stroke:#333
```

## Primitive Relationships Explained

### Configuration Primitives (Static Definitions)

1. **AI Providers**
   - Define connection details to OpenAI-compatible APIs
   - Used to discover available models
   - Contain encrypted API credentials

2. **Tools**
   - Reusable functionality that can be provided to agents
   - Defined with specific configurations
   - Examples: HTTP clients, database connectors, memory operations

3. **Agents**
   - Combine a model from a provider with a system prompt and tools
   - Represent an AI persona with specific capabilities
   - Built using Google ADK patterns

4. **WASM Modules**
   - Compiled WebAssembly code for imperative execution
   - Provide custom logic that can't be expressed with agents
   - Executed securely using the wazero library

5. **Workflows**
   - Ordered collections of workflow steps
   - Define the sequence of operations to execute
   - Can mix agent invocations and WASM executions

### Execution Primitives (Runtime Instances)

1. **Workflow Steps**
   - Concrete instances of either:
     - Agent invocations (with specific agent reference)
     - WASM executions (with specific module reference)
   - Contain step-specific configuration
   - Part of a workflow definition

2. **Jobs**
   - Runtime instances of workflows
   - Track execution status and results
   - Contain input data and output data
   - Managed by the background job system

## Composition Hierarchy

```
Provider
└── Agent
    └── Workflow Step (Agent type)
        └── Workflow
            └── Job

Tool
└── Agent
    └── Workflow Step (Agent type)
        └── Workflow
            └── Job

WASM Module
└── Workflow Step (WASM type)
    └── Workflow
        └── Job
```

## Execution Flow

1. **Configuration**: Primitives are defined and stored in the database
2. **Composition**: Workflows are created by arranging workflow steps
3. **Instantiation**: A job is created when a workflow is executed
4. **Execution**: Each workflow step in the job is executed in order:
   - Agent steps invoke the agent runtime with the specified agent
   - WASM steps execute the specified WASM module
5. **Completion**: Results are collected and returned to the user