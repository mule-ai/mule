# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Mule is an AI workflow platform that enables users to create, configure, and execute complex AI-powered workflows. It combines AI agents powered by **pi RPC**, custom skills, and WebAssembly modules to create flexible automation pipelines, exposed through an OpenAI-compatible API.

The project has been migrated from Google ADK to pi RPC for agent execution, providing a more flexible and extensible architecture with skills support.

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

# Run staging environment
docker-compose -f docker-compose.staging.yml up -d
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

# Run tests for specific package
go test -v ./internal/agent/pirc/...
```

### Development Setup
```bash
# Set up test data (providers, agents, skills, WASM modules)
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
   - Supports: Anthropic, OpenAI, Google providers
   - Enables dynamic model discovery

2. **Skills** - Pi agent skills that can be assigned to agents
   - Table: `skills`
   - Stores skill name, description, path (directory), and enabled status
   - Skills are bound to agents via `agent_skills` junction table
   - Pi skills provide extensibility (file operations, grep, find, bash, read, write, edit, etc.)

3. **Agents** - AI agents powered by pi RPC runtime
   - Table: `agents`
   - References: provider_id, model_id, system_prompt, pi_config (JSONB)
   - Uses pi RPC for execution with configurable skills, thinking level, tools, and extensions

4. **WASM Modules** - WebAssembly modules for imperative code
   - Table: `wasm_modules`
   - Executed using wazero library in sandboxed environment
   - Binary module_data stored in database

5. **Workflows** - Ordered sequences of workflow steps
   - Table: `workflows`
   - Can be synchronous or asynchronous (`is_async` flag)
   - Can be invoked via `/v1/chat/completions` with model prefix `workflow/` or `async/workflow/`

6. **Workflow Steps** - Individual execution steps
   - Table: `workflow_steps`
   - Types: "AGENT" (invokes agent via pi RPC) or "WASM" (executes WASM)
   - Ordered by `step_order` within a workflow

### Execution Flow

1. **Configuration Phase**: Primitives configured via UI/API → Stored in PostgreSQL via Primitive Manager
2. **Execution Phase**: User calls `/v1/chat/completions` → Request queued as Job → Worker executes Workflow Steps → Each step invokes Agent (pi RPC) or WASM (wazero) → Results streamed via WebSocket

### Key Components

- **cmd/api/**: Main application entry point, HTTP handlers, API server
  - `server.go`: HTTP server setup and routing
  - `handlers.go`: OpenAI-compatible API endpoints (`/v1`) and management APIs
  - `memory_handlers.go`: Memory and semantic search endpoints
  - `wasm_handlers.go`: WASM module management endpoints
  - `integration_test.go`, `comprehensive_test.go`, `skills_test.go`: API tests

- **internal/**: Core application logic
  - `agent/`: Agent runtime with pi RPC integration
  - `agent/pirc/`: **pi RPC bridge package** - handles subprocess management and event streaming
    - `pibridge.go`: PI subprocess management, RPC command execution, event parsing
    - `event_mapper.go`: Converts pi events to Mule WebSocket format
    - `websocket_integration.go`: Streams pi events to WebSocket clients
    - `pibridge_test.go`, `event_mapper_test.go`, `e2e_streaming_test.go`, `websocket_integration_test.go`, `performance_test.go`: Tests and benchmarks
  - `api/`: HTTP middleware and WebSocket handling
  - `config/`: Application configuration
  - `database/`: PostgreSQL connection, migrations, and data access
  - `engine/`: Workflow engine orchestrating job execution
  - `initialization/`: Application initialization logic
  - `manager/`: Primitive management (providers, skills, agents, workflows)
  - `primitive/`: Core primitive types and validation
  - `tools/`: Tool registry and implementations (bash, filesystem, http, database)
  - `validation/`: Input validation logic
  - `wasmcompiler/`: WASM module compilation utilities

- **pkg/**: Reusable packages
  - `database/`: Shared database models (Provider, Agent, Workflow, Job, etc.)
  - `job/`: Job queue management, job store, and job execution logic

### PI RPC Integration

The `pirc` package provides the core infrastructure for pi integration:

- **Subprocess Management**: Spawns pi as a subprocess with `--mode rpc --no-session` flags
- **RPC Protocol**: JSON lines communicated over stdin/stdout
- **Commands**: prompt, steer, follow_up, abort, new_session, set_model, set_thinking_level, bash
- **Events**: text_delta, thinking_delta, tool_execution_*, agent_start/end, message_update, extension_ui_request/response
- **Configuration**: Provider, model, API key, system prompt, thinking level, skills, tools, extensions, working directory
- **Event Channel**: Buffered channel (100 events) with non-blocking sends to prevent deadlocks

### API Structure

- **OpenAI-compatible endpoints** (`/v1`):
  - `GET /v1/models` - Lists agents (prefixed with "agent/") and workflows (prefixed with "workflow/")
  - `POST /v1/chat/completions` - Executes agents/workflows with sync/async modes

- **Skills API** (`/api/v1/skills`) - CRUD operations for skills
  - `GET /api/v1/skills` - List all skills
  - `POST /api/v1/skills` - Create a skill
  - `GET /api/v1/skills/{id}` - Get a skill
  - `PUT /api/v1/skills/{id}` - Update a skill
  - `DELETE /api/v1/skills/{id}` - Delete a skill

- **Tools API** (`/api/v1/tools`) - Tool registry management
  - `GET /api/v1/tools` - List all tools in registry
  - `POST /api/v1/tools` - Register a new tool
  - `GET /api/v1/tools/{id}` - Get a tool
  - `PUT /api/v1/tools/{id}` - Update a tool
  - `DELETE /api/v1/tools/{id}` - Delete a tool

- **Agent Tools API** (`/api/v1/agents/{id}/tools`) - Manage tools on agents
  - `GET /api/v1/agents/{id}/tools` - List tools assigned to an agent
  - `POST /api/v1/agents/{id}/tools` - Assign a tool to an agent
  - `DELETE /api/v1/agents/{id}/tools/{toolId}` - Remove a tool from an agent

- **Agent Skills API** (`/api/v1/agents/{id}/skills`) - Manage skills on agents
  - `GET /api/v1/agents/{id}/skills` - List skills assigned to an agent
  - `PUT /api/v1/agents/{id}/skills` - Assign skills to an agent
  - `DELETE /api/v1/agents/{id}/skills/{skillId}` - Remove skill from agent

- **Settings API** (`/api/v1/settings`) - Application settings management
  - `GET /api/v1/settings` - List all settings
  - `GET /api/v1/settings/{key}` - Get a specific setting
  - `PUT /api/v1/settings/{key}` - Update a setting

- **Memory Config API** (`/api/v1/memory-config`) - Memory vector search configuration
  - `GET /api/v1/memory-config` - Get memory configuration
  - `PUT /api/v1/memory-config` - Update memory configuration

- **Management API** (`/api/v1`):
  - `/providers`, `/providers/{id}`, `/providers/{id}/models` - Provider management
  - `/tools`, `/tools/{id}` - Tool registry management
  - `/agents`, `/agents/{id}`, `/agents/{id}/skills`, `/agents/{id}/tools` - Agent management
  - `/skills`, `/skills/{id}` - Skill management
  - `/workflows`, `/workflows/{id}`, `/workflows/{id}/steps` - Workflow management
  - `/jobs`, `/jobs/{id}`, `/jobs/{id}/steps` - Job monitoring and cancellation
  - `/wasm-modules`, `/wasm-modules/{id}`, `/wasm-modules/{id}/source` - WASM module management
  - `/wasm-modules/compile`, `/wasm-modules/test`, `/wasm-modules/example` - WASM compilation and testing
  - `/memory-config` - Memory vector search configuration
  - `/settings`, `/settings/{key}` - Application settings

- **Real-time**: `WS /ws` - WebSocket for job and agent execution updates

### Database

- **Migrations**: `internal/database/migrations/`
  - `0001_initial_schema.sql` - Core schema (providers, agents, workflows, jobs)
  - `0002_add_error_message_to_jobs.sql` - Error tracking
  - `0002_add_memory_config.sql` - Memory vector search configuration
  - `0002_add_wasm_source_code.sql` - WASM module source code storage
  - `0003_add_settings_table.sql` - Application settings
  - `0004_add_max_tool_calls_setting.sql` - Agent tool call limits
  - `0005_add_job_timeout_setting.sql` - Job timeout configuration
  - `0006_add_wasm_module_config.sql` - WASM module configuration
  - `0007_add_working_directory_to_jobs.sql` - Job working directory
  - `0008_add_skills_table.sql` - Skills and agent_skills tables, pi_config on agents
  - `0009_optimize_job_queries.sql` - Job query performance optimization
  - `0010_add_query_optimization_indexes.sql` - Additional query optimization indexes
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

### Database Resource Management
The project uses helper functions in `internal/database/rows.go` to safely clean up database resources:

```go
// Safe resource cleanup with logging
rows, err := db.QueryContext(ctx, query)
if err != nil {
    return err
}
defer database.CloseRows(rows)
```

**Available Helpers:**
- `CloseRows(rows *sql.Rows)` - Safely closes sql.Rows, logs any errors
- `CloseDB(db *sql.DB)` - Safely closes sql.DB connection
- `CloseStmt(stmt *sql.Stmt)` - Safely closes sql.Stmt

**Usage Pattern:**
```go
// All database operations should use these helpers
rows, err := store.db.QueryContext(ctx, query)
if err != nil {
    return err
}
defer database.CloseRows(rows)

for rows.Next() {
    // Process rows...
}
if err := rows.Err(); err != nil {
    return err
}
```

These helpers handle nil pointers safely and log errors rather than returning them, since Close() errors typically indicate non-critical issues (connection already closed, etc.).

### Goroutine and Request Lifecycle
The project uses proper goroutine cleanup to prevent resource leaks:

**Timeout Middleware:**
```go
// TimeoutMiddleware waits for handler goroutines to complete after timeout
// This prevents orphaned goroutines consuming resources
go func() {
    select {
    case <-done:
        return
    case <-timeout:
        waitForHandler() // Wait up to 5s for handler completion
        return
    }
}()
```

**Best Practices:**
- Always use `defer` to clean up resources (database connections, goroutines)
- Use `sync.WaitGroup` when waiting for multiple goroutines
- Check for goroutine leaks with `go test -race`

### Testing
- Unit tests alongside implementation files (`*_test.go`)
- Integration tests in `cmd/api/`
- Use `testify/assert` for assertions
- Tests skip gracefully when API keys are not available
- Performance benchmarks in `*_test.go` files with Benchmark functions

### Agent Configuration (pi_config)
- Agents store pi-specific configuration in `pi_config` JSONB field
- Configurable options: thinking level (off, minimal, low, medium, high, xhigh), skills, tools, extensions
- Example:
  ```json
  {
    "thinking_level": "medium",
    "skills": ["skill-id-1", "skill-id-2"],
    "tools": ["read", "write", "edit", "bash", "grep", "find"],
    "extensions": ["extension-name"],
    "working_dir": "/path/to/directory"
  }
  ```

### WASM Execution
- WASM modules compiled from Go: `GOOS=js GOARCH=wasm go build -o module.wasm`
- Example: `hello.wasm` compiled from `hello_wasm.go`
- Host functions enable Go-WASM communication

### Job Processing
- Background workers poll `jobs` table for QUEUED status
- Steps executed sequentially based on `step_order`
- Results stored incrementally in `job_steps` table
- WebSocket broadcasts real-time updates

### Event Streaming
- PIEventStreamer handles real-time event broadcasting to WebSocket clients
- Events filtered by type (text_delta, thinking_delta, tool events, lifecycle events)
- Non-blocking event sending prevents deadlocks under high load

### Extension UI Request/Response Pattern
Agents can request user input during execution via the extension_ui_request event. This enables interactive workflows where agents prompt users for selections, confirmations, or text input.

**UI Request Types:**
- `select` - User selects from a list of options
- `confirm` - User confirms or cancels an action (yes/no)
- `input` - User enters text input

**Event Flow:**
1. Agent sends `extension_ui_request` event with request ID, method, title, and options/input
2. Mule streams this event to connected WebSocket clients
3. Client application displays the UI prompt to the user
4. User provides input/selection
5. Client sends response via `SendExtensionUIResponse(id, value, confirmed)` on the bridge

**WebSocket Event Format:**
```json
{
  "event": "extension_ui_request",
  "data": {
    "id": "ui-req-uuid",
    "method": "select",
    "title": "Select an option",
    "options": ["Option A", "Option B", "Option C"],
    "timeout": 30000
  }
}
```

**Response Format (via Bridge):**
```go
// On the Bridge instance
bridge.SendExtensionUIResponse("ui-req-uuid", "selected_value", true)
```

**Client Integration:**
```go
// In WebSocket handler or client code
for event := range events {
    if event.Type == "MuleEventExtensionUIRequest" {
        // Display prompt to user
        response := showPrompt(event.ID, event.Title, event.Options)
        // Send response back to agent
        bridge.SendExtensionUIResponse(event.ID, response.Value, response.Confirmed)
    }
}
```

### Testing Patterns
The project uses comprehensive testing following these patterns:

**Unit Tests:**
- Files alongside implementation: `*_test.go`
- Use `testify/assert` for assertions
- Mock external dependencies (stores, bridges)
- Example: `internal/agent/pirc/pibridge_test.go`

**Integration Tests:**
- Test full request/response cycles
- Located in `cmd/api/` or `internal/agent/`
- Use httptest for HTTP testing
- Example: `cmd/api/integration_test.go`, `cmd/api/skills_test.go`

**End-to-End Tests:**
- Test complete workflows from API to execution
- May require external services (database, pi)
- Skip gracefully when dependencies unavailable
- Example: `internal/agent/pirc/e2e_streaming_test.go`

**Mock Stores for Testing:**
```go
type MockPrimitiveStore struct {
    // Embed common mock methods
    // Add specific mock methods as needed
}
```

**Test Skipping Pattern:**
```go
if os.Getenv("ANTHROPIC_API_KEY") == "" {
    t.Skip("Skipping test: ANTHROPIC_API_KEY not set")
}
```

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

### Adding a Skill
1. Create skill directory with skill files
2. Add skill via API: `POST /api/v1/skills` with name, description, path
3. Assign to agent: `PUT /api/v1/agents/{id}/skills` with skill_ids

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

### Debugging Agent Execution
1. Check pi is installed: `which pi` or `pi --version`
2. Verify provider API keys are configured
3. Check agent has valid pi_config (skills, thinking level)
4. Review WebSocket events for error details
5. Enable debug logging: `LOG_LEVEL=debug`
