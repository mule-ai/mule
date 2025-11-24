# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Mule is an AI workflow platform that enables users to create, configure, and execute complex AI-powered workflows. It combines AI agents, custom tools, and WebAssembly modules to create flexible automation pipelines, exposed through an OpenAI-compatible API.

## Development Commands

### Building and Running
```bash
# Build the application
make build
# or manually: cd cmd/api && CGO_ENABLED=1 GOOS=linux go build -o bin/mule

# Run the server (requires PostgreSQL)
./cmd/api/bin/mule -db "postgres://mule:mule@localhost:5432/mulev2?sslmode=disable"

# Run with hot reload during development
make air

# Run with Docker
docker-compose up -d
```

### Testing and Code Quality
```bash
# Run all tests
make test

# Run linting
make lint

# Format code
make fmt

# Run a single test
go test -v ./internal/engine -run TestEngineExecuteWorkflow
```

### Development Setup
```bash
# Set up test data (providers, agents, WASM modules)
./setup-dev.sh

# Test workflow execution
./test_workflow.sh
```

## Architecture

### Core Primitives

The system is built around six core primitives stored in PostgreSQL:

1. **Providers** - AI provider configurations (OpenAI-compatible APIs)
   - Table: `providers`
   - Configuration: API base URL, encrypted API key
   - Enables dynamic model discovery

2. **Tools** - Extensible tools for agents (Google ADK patterns)
   - Table: `tools`
   - Includes memory operations and custom tool creation
   - Tools are bound to agents via `agent_tools` junction table

3. **Agents** - AI agents combining models, prompts, and tools
   - Table: `agents`
   - References: provider_id, model_id, system_prompt
   - Uses Google ADK for execution

4. **WASM Modules** - WebAssembly modules for imperative code
   - Table: `wasm_modules`
   - Executed using wazero library in sandboxed environment
   - Binary module_data stored in database

5. **Workflows** - Ordered sequences of workflow steps
   - Table: `workflows`
   - Can be synchronous or asynchronous (`is_async` flag)

6. **Workflow Steps** - Individual execution steps
   - Table: `workflow_steps`
   - Types: "AGENT" (invokes agent) or "WASM" (executes WASM)
   - Ordered by `step_order` within a workflow

### Execution Flow

1. **Configuration Phase**: Primitives configured via UI/API → Stored in PostgreSQL via Primitive Manager
2. **Execution Phase**: User calls `/v1/chat/completions` → Request queued as Job → Worker executes Workflow Steps → Each step invokes Agent (ADK) or WASM (wazero) → Results streamed via WebSocket

### Key Components

- **cmd/api/**: Main application entry point, HTTP handlers, API server
  - `server.go`: HTTP server setup and routing
  - `handlers.go`: OpenAI-compatible API endpoints (`/v1`)
  - `integration_test.go`, `comprehensive_test.go`: API tests

- **internal/**: Core application logic
  - `agent/`: Agent runtime with Google ADK integration
  - `api/`: HTTP middleware and WebSocket handling
  - `database/`: PostgreSQL connection, migrations, and data access
  - `engine/`: Workflow engine orchestrating job execution
  - `manager/`: Primitive management (providers, tools, agents, workflows)
  - `primitive/`: Core primitive types and validation
  - `provider/`: AI provider implementations
  - `validation/`: Input validation logic

- **pkg/**: Reusable packages
  - `database/`: Shared database models
  - `job/`: Job queue management and execution
  - `provider/`, `tool/`, `workflow/`: Domain-specific types

### API Structure

- **OpenAI-compatible endpoints** (`/v1`):
  - `GET /v1/models` - Lists agents (prefixed with "agent/") and workflows (prefixed with "workflow/")
  - `POST /v1/chat/completions` - Executes agents/workflows with sync/async modes

- **Management API** (`/api/v1`):
  - `/providers`, `/tools`, `/agents`, `/workflows` - CRUD operations
  - `/jobs`, `/jobs/{id}`, `/jobs/{id}/steps` - Job monitoring
  - `/workflows/{id}/steps` - Workflow step management

- **Real-time**: `WS /ws` - WebSocket for job execution updates

### Database

- **Migrations**: `internal/database/migrations/`
  - `0001_initial_schema.sql` - Core schema
  - `0002_add_error_message_to_jobs.sql` - Error tracking
- **Connection**: Use `DB_CONN_STRING` environment variable or `-db` flag
- **Job Queue**: PostgreSQL-based with worker pool processing

### Frontend

- **Technology**: React with light/dark mode support
- **Deployment**: Static assets compiled into Go binary (`internal/frontend/embed.go`)
- **Development**: `cd frontend && npm start` (runs on separate port)
- **Building**: `cd frontend && npm run build` (embeds into binary)

## Important Patterns

### Error Handling
- Database operations return structured errors
- Workflow steps track individual failures in `job_steps` table
- Jobs have `status` field: QUEUED, RUNNING, COMPLETED, FAILED

### Testing
- Unit tests alongside implementation files (`*_test.go`)
- Integration tests in `cmd/api/`
- Use `testify/assert` for assertions
- Database tests use test fixtures and cleanup

### WASM Execution
- WASM modules compiled from Go: `GOOS=js GOARCH=wasm go build -o module.wasm`
- Example: `hello.wasm` compiled from `hello_wasm.go`
- Host functions enable Go-WASM communication

### Job Processing
- Background workers poll `jobs` table for QUEUED status
- Steps executed sequentially based on `step_order`
- Results stored incrementally in `job_steps` table
- WebSocket broadcasts real-time updates

## Common Tasks

### Adding a New API Endpoint
1. Add handler in `cmd/api/handlers.go`
2. Register route in `cmd/api/server.go`
3. Add validation in `internal/validation/validator.go` if needed
4. Write tests in `cmd/api/`

### Creating a Database Migration
1. Create new `.sql` file in `internal/database/migrations/`
2. Follow naming: `000N_description.sql`
3. Use `internal/database/migrator.go` patterns
4. Test with `make test`

### Adding a WASM Module
1. Write Go code with `//go:build js && wasm`
2. Compile: `GOOS=js GOARCH=wasm go build -o module.wasm`
3. Upload via `/api/v1/wasm-modules` endpoint
4. Reference in workflow steps

### Debugging Workflow Execution
1. Check `jobs` table for job status
2. Inspect `job_steps` for step-level details
3. Use WebSocket connection for real-time logs
4. Check PostgreSQL logs for query issues
5. Review application logs for execution errors
