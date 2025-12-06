# Mule v2 - AI Workflow Platform PRD

## Overview

Mule is an AI workflow platform that enables users to create, configure, and execute complex AI-powered workflows. It combines the power of AI agents, custom tools, and WebAssembly modules to create flexible and extensible automation pipelines.

## Core Primitives

### 1. AI Providers
- Support for OpenAI-compliant APIs
- Configuration options:
  - API Base URL
  - API Key
  - Model discovery via `v1/models` endpoint
- Dynamic model listing based on connected providers

### 2. Tools
- Extensible tool system based on Google ADK patterns
- Integration with jbutlerdev/genai tools including memory operations
- Built-in tools for filesystem operations, HTTP requests, database queries, and bash command execution
- Custom tool creation capabilities
- Tool categorization and metadata management

### 3. WASM Modules
- Execution using the wazero library
- Imperative code execution as workflow steps
- Secure sandboxed execution environment
- Data passing between Go and WASM contexts

### 4. Agents
- Combination of:
  - Model from a provider
  - System prompt/instructions
  - Available tools
- Implementation using Google ADK agent patterns
- Hierarchical agent composition
- Callback mechanisms for extensibility

### 5. Workflow Steps
- Atomic units of execution
- Two types:
  - Agent invocation
  - WASM module execution
- Input/output handling
- Error management

### 6. Workflows
- Ordered sequences of workflow steps
- Conditional execution logic
- Parallel execution capabilities
- Job queue integration with PostgreSQL

## Frontend Requirements

### UI/UX Design
- Modern, responsive interface with light/dark mode support
- Intuitive drag-and-drop workflow builder
- Real-time execution visualization
- Comprehensive configuration panels for all primitives

### Static Web Application
- Fully static React application
- Compiled into Go binary with no external filesystem dependencies
- Optimized asset bundling
- Client-side routing

### Core Features
1. **Configuration Management**
   - Provider setup and management
   - Tool configuration interface
   - WASM module upload and management
   - Agent creation and customization
   - Workflow design canvas

2. **Workflow Builder**
   - Visual workflow editor with drag-and-drop interface
   - Step configuration panels
   - Connection management between steps
   - Preview and validation capabilities

3. **Execution Interface**
   - Per-step execution with input text area
   - WebSocket-based real-time output streaming
   - Full workflow execution with same interface
   - Execution history and result viewing

## Backend Requirements

### Technology Stack
- Go programming language
- Google ADK for agent implementation
- wazero for WASM module execution
- PostgreSQL for data persistence and job queuing
- Idiomatic Go design patterns
- Minimal abstraction philosophy

### Core Components

#### 1. API Server
- OpenAI-compatible API implementation
- Endpoints:
  - `GET /v1/models` - Lists all agents and workflows
    - Agents prefixed with "agent/"
    - Workflows prefixed with "workflow/"
  - `POST /v1/chat/completions` - Executes agents or workflows
    - Model parameter specifies agent/workflow to execute
    - Messages concatenated as input prompt
    - Synchronous and asynchronous execution modes
    - Asynchronous responses include job ID

#### 2. Primitive Management
- Database-backed storage for all primitives
- CRUD operations for providers, tools, agents, and workflows
- Validation and integrity checking
- Migration system for schema evolution

#### 3. Workflow Engine
- Background job processing system
- Configurable worker pool
- Job queue implementation using PostgreSQL
- Step execution orchestration
- Error handling and retry mechanisms
- Progress tracking and status reporting

#### 4. WASM Execution Environment
- Secure module loading and instantiation
- Resource limiting and isolation
- Host function provision for Go integration
- Memory management and cleanup
- Performance optimization through module compilation

#### 5. Agent Runtime
- Google ADK integration for agent execution
- Tool binding and lifecycle management
- Memory service integration
- Callback handling and event processing
- Streaming response support

### Database Schema
- Providers table (API configuration)
- Tools table (tool definitions and metadata)
- Agents table (agent configurations)
- Workflows table (workflow definitions)
- Workflow steps table (step configurations)
- Jobs table (execution queue and history)
- Job steps table (individual step execution records)
- Artifacts table (persistent data storage)

## API Specification

### Models Endpoint
```
GET /v1/models
```
Returns a list of all available agents and workflows:
```json
{
  "data": [
    {
      "id": "agent/gpt-4-agent",
      "object": "model",
      "owned_by": "mule"
    },
    {
      "id": "workflow/data-processing-pipeline",
      "object": "model",
      "owned_by": "mule"
    }
  ]
}
```

### Chat Completions Endpoint
```
POST /v1/chat/completions
```
Request body:
```json
{
  "model": "agent/gpt-4-agent",
  "messages": [
    {"role": "user", "content": "Hello, world!"}
  ],
  "stream": false
}
```

Synchronous response:
```json
{
  "id": "chatcmpl-123",
  "object": "chat.completion",
  "created": 1677652288,
  "model": "agent/gpt-4-agent",
  "choices": [{
    "index": 0,
    "message": {
      "role": "assistant",
      "content": "Hello! How can I help you today?"
    },
    "finish_reason": "stop"
  }],
  "usage": {
    "prompt_tokens": 9,
    "completion_tokens": 12,
    "total_tokens": 21
  }
}
```

Asynchronous response:
```json
{
  "id": "job-456",
  "object": "async.job",
  "status": "queued",
  "message": "The workflow has been started"
}
```

## Technical Architecture

### System Components
1. **API Layer** - HTTP server implementing OpenAI-compatible endpoints
2. **Primitive Manager** - Database interface for all core entities
3. **Workflow Engine** - Background job processor with worker pool
4. **Agent Runtime** - Google ADK integration layer
5. **WASM Executor** - wazero-based module execution environment
6. **Frontend Server** - Embedded static file server for React UI

### Data Flow
1. User configures primitives through UI/API
2. Configuration stored in PostgreSQL
3. User initiates workflow execution
4. Request queued as job in PostgreSQL
5. Worker picks up job and executes steps sequentially
6. Each step either invokes an agent or executes a WASM module
7. Results stored and streamed back to user

### Security Considerations
- API key encryption at rest
- WASM module sandboxing
- Input validation and sanitization
- Rate limiting and resource quotas
- Authentication and authorization (future enhancement)

## Deployment and Operations

### Single Binary Distribution
- All components compiled into single Go binary
- Embedded React frontend assets
- No external runtime dependencies
- Cross-platform compatibility

### Configuration
- Environment variables for runtime configuration
- Database connection parameters
- Worker count configuration
- Logging and monitoring settings

### Monitoring
- Structured logging
- Performance metrics
- Health check endpoints
- Execution tracing (future enhancement)

## Future Enhancements

### Short Term
- Authentication and user management
- Advanced workflow branching and conditions
- Enhanced error handling and recovery
- Performance optimization

### Long Term
- Plugin system for custom integrations
- Distributed worker architecture
- Advanced scheduling capabilities
- Marketplace for shared workflows and tools

## Success Metrics

- Workflow execution reliability (>99.9%)
- Response time for simple agent calls (<2 seconds)
- Concurrent workflow capacity (>100 simultaneous)
- Developer onboarding time (<30 minutes)
- User satisfaction scores (>4.5/5)

## Release Milestones

### MVP (Minimum Viable Product)
- Basic agent execution via OpenAI API
- Simple sequential workflow engine
- Embedded React UI with light/dark modes
- PostgreSQL integration
- WASM module execution support

### Beta Release
- Full workflow builder UI
- Asynchronous execution support
- Comprehensive primitive management
- Job queue monitoring
- Basic error handling

### Production Release
- Performance optimizations
- Advanced workflow features
- Comprehensive documentation
- Security hardening
- Monitoring and alerting