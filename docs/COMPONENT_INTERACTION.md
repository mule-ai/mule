# Mule v2 - Component Interaction Diagram

```mermaid
graph TD
    subgraph "User Interfaces"
        UI[Web UI<br/>React Application]
        API[OpenAI Compatible API]
    end
    
    subgraph "Go Application"
        direction TB
        
        subgraph "API Layer"
            HTTP[HTTP Server<br/>Gin/Echo]
            ROUTER[Router<br/>Endpoint Mapping]
        end
        
        subgraph "Business Logic"
            WM[Workflow Manager]
            AM[Agent Manager]
            TM[Tool Manager]
            PM[Provider Manager]
            WMExec[Workflow Executor]
        end
        
        subgraph "Integration Layers"
            ADK[Google ADK<br/>Agent Runtime]
            WASM[wazero<br/>WASM Runtime]
            DB[Database Layer<br/>GORM/SQL]
        end
        
        subgraph "Background Processing"
            WP[Worker Pool]
            JQ[Job Queue<br/>PostgreSQL Based]
        end
    end
    
    subgraph "External Services"
        DBS[(PostgreSQL<br/>Database)]
        AIS[AI Services<br/>OpenAI Compatible]
        WASMS[WASM Modules<br/>User Provided]
    end
    
    %% API Layer connections
    HTTP --> ROUTER
    ROUTER -->|/v1/models| AM
    ROUTER -->|/v1/chat/completions| WM
    ROUTER -->|/primitives/*| PM
    ROUTER -->|/primitives/*| TM
    ROUTER -->|/primitives/*| AM
    ROUTER -->|/primitives/*| WM
    ROUTER -->|/jobs/*| WM
    
    %% Business Logic connections
    WM --> WMExec
    WM --> DB
    AM --> DB
    TM --> DB
    PM --> DB
    
    %% Integration connections
    WMExec --> ADK
    WMExec --> WASM
    ADK --> DB
    WASM --> DB
    
    %% Background Processing
    WP --> JQ
    WMExec --> WP
    JQ --> DB
    
    %% External connections
    DB --> DBS
    ADK --> AIS
    WASM --> WASMS
    
    %% User interfaces
    UI --> HTTP
    API --> HTTP
    
    %% Styling
    classDef ui fill:#FFE4B5,stroke:#333;
    classDef apiLayer fill:#87CEEB,stroke:#333;
    classDef businessLogic fill:#98FB98,stroke:#333;
    classDef integration fill:#F0E68C,stroke:#333;
    classDef background fill:#DDA0DD,stroke:#333;
    classDef external fill:#FFB6C1,stroke:#333;
    
    class UI,API ui
    class HTTP,ROUTER apiLayer
    class WM,AM,TM,PM,WMExec businessLogic
    class ADK,WASM,DB integration
    class WP,JQ background
    class DBS,AIS,WASMS external
```

## Component Interactions

### User Interface Layer
- **Web UI**: Static React application served by the embedded Go server
- **OpenAI Compatible API**: RESTful interface that mimics OpenAI's API for broad compatibility

### API Layer
- **HTTP Server**: Handles incoming HTTP requests using Gin or Echo framework
- **Router**: Maps endpoints to appropriate handlers

### Business Logic Layer
- **Workflow Manager**: Handles workflow CRUD operations and execution initiation
- **Agent Manager**: Manages agent configurations and direct agent execution
- **Tool Manager**: Handles tool definitions and configurations
- **Provider Manager**: Manages AI provider configurations
- **Workflow Executor**: Orchestrates the execution of workflows and their steps

### Integration Layers
- **Google ADK**: Provides the runtime for executing agents with their tools
- **wazero**: Executes WebAssembly modules in a secure sandboxed environment
- **Database Layer**: Abstracts database operations using GORM or direct SQL

### Background Processing
- **Worker Pool**: Configurable pool of goroutines for concurrent job execution
- **Job Queue**: PostgreSQL-based queue system for managing workflow jobs

### External Services
- **PostgreSQL Database**: Persistent storage for all configurations and job data
- **AI Services**: External OpenAI-compatible APIs
- **WASM Modules**: User-provided WebAssembly binaries

## Key Interaction Flows

### 1. Workflow Definition
```
UI/API → HTTP Server → Router → Workflow Manager → Database Layer → PostgreSQL
```

### 2. Workflow Execution Initiation
```
UI/API → HTTP Server → Router → Workflow Manager → Workflow Executor → Job Queue → PostgreSQL
```

### 3. Workflow Step Execution
```
Worker Pool → Job Queue → Workflow Executor → 
  ├── Agent Manager → Google ADK → AI Services
  └── WASM Executor → wazero → WASM Modules
```

### 4. Data Persistence
```
All Managers → Database Layer → PostgreSQL
```

## Data Flow Patterns

### Configuration Data
1. Created/updated via UI or API
2. Stored in PostgreSQL through Database Layer
3. Retrieved by managers when needed

### Execution Data
1. Jobs created in PostgreSQL when execution initiated
2. Workers claim jobs and update status
3. Step results stored as job progresses
4. Final results returned to user

### Real-time Communication
1. WebSocket connections for live execution updates
2. Direct streaming from agent executions
3. Status polling for job monitoring

## Security Boundaries

- **API Layer**: Authentication and rate limiting
- **Business Logic**: Input validation and sanitization
- **Integration Layers**: Secure credential handling
- **WASM Execution**: Sandboxed execution environment
- **Database**: Encrypted storage for sensitive data