# Mule v2 - High-Level Software Architecture

```mermaid
graph TD
    subgraph "Frontend Layer"
        A[React UI] --> B[Embedded Go Server]
    end
    
    subgraph "API Layer"
        B --> C[HTTP Router]
        C --> D[Models Endpoint]
        C --> E[Chat Completions Endpoint]
        C --> F[Primitive Management API]
        C --> G[Job Management API]
    end
    
    subgraph "Core Services"
        H[Primitive Manager] --> I[Database Interface]
        J[Workflow Engine] --> I
        K[Agent Runtime] --> L[Google ADK]
        M[WASM Executor] --> N[wazero]
    end
    
    subgraph "External Systems"
        O[PostgreSQL Database]
        P[AI Providers]
        Q[WASM Modules]
    end
    
    subgraph "Background Processing"
        R[Worker Pool] --> J
        R --> S[Job Queue Manager]
        S --> O
    end
    
    %% Connections
    I --> O
    J --> H
    J --> K
    J --> M
    K --> L
    K --> P
    M --> N
    M --> Q
    D --> H
    E --> J
    F --> H
    G --> J
    
    %% Styling
    classDef frontend fill:#FFE4B5,stroke:#333;
    classDef api fill:#87CEEB,stroke:#333;
    classDef core fill:#98FB98,stroke:#333;
    classDef external fill:#FFB6C1,stroke:#333;
    classDef background fill:#DDA0DD,stroke:#333;
    
    class A,B frontend
    class C,D,E,F,G api
    class H,I,J,K,L,M,N core
    class O,P,Q external
    class R,S background
```

## Architecture Components

### Frontend Layer
- **React UI**: Static React application compiled into the Go binary
- **Embedded Go Server**: Serves the static frontend assets with no external filesystem dependencies

### API Layer
- **HTTP Router**: Routes incoming requests to appropriate handlers
- **Models Endpoint**: Implements `GET /v1/models` to list available agents and workflows
- **Chat Completions Endpoint**: Implements `POST /v1/chat/completions` for executing agents and workflows
- **Primitive Management API**: CRUD operations for providers, tools, agents, and workflows
- **Job Management API**: Interface for monitoring and managing job executions

### Core Services

#### Primitive Manager
- Manages all core entities (providers, tools, agents, workflows)
- Provides database interface for persistent storage
- Handles validation and integrity checking

#### Workflow Engine
- Orchestrates workflow execution
- Manages job queue processing
- Coordinates between different step types

#### Agent Runtime
- Integrates with Google ADK for agent execution
- Manages tool binding and lifecycle
- Handles communication with AI providers

#### WASM Executor
- Uses wazero library for secure WASM execution
- Manages module loading and instantiation
- Provides host functions for Go integration

### External Systems
- **PostgreSQL Database**: Primary data store for all configuration and job execution data
- **AI Providers**: External OpenAI-compatible APIs
- **WASM Modules**: User-provided WebAssembly modules

### Background Processing
- **Worker Pool**: Configurable pool of workers for job execution
- **Job Queue Manager**: Manages job queuing and execution using PostgreSQL

## Data Flow

1. **Configuration Phase**:
   - User configures primitives through UI/API
   - Data stored in PostgreSQL via Primitive Manager

2. **Execution Phase**:
   - User initiates workflow/agent execution via API
   - Request queued as job in PostgreSQL
   - Worker picks up job and executes steps
   - Each step either invokes an agent (via Google ADK) or executes a WASM module (via wazero)
   - Results stored in database and streamed back to user

## Key Design Principles

1. **Single Binary Deployment**: All components compiled into a single Go binary with embedded frontend assets
2. **Database-Centric**: PostgreSQL used for both configuration storage and job queuing
3. **Secure Execution**: WASM modules executed in secure sandboxed environment
4. **Extensible Architecture**: Modular design allows for future enhancements
5. **Idiomatic Go**: Following Go best practices and minimal abstraction philosophy