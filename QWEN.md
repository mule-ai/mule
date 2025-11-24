# Mule AI Workflow Platform - Development Context

## Project Overview

Mule is an AI workflow platform that enables users to create, configure, and execute complex AI-powered workflows. It combines AI agents, custom tools, and WebAssembly modules to create flexible automation pipelines.

### Core Primitives

1. **AI Providers** - Connections to OpenAI-compliant APIs
2. **Tools** - Extensible tools that can be provided to agents
3. **WASM Modules** - Imperative code execution using the wazero library
4. **Agents** - Combination of a model, system prompt, and tools using Google ADK
5. **Workflow Steps** - Either a call to an agent or execution of a WASM module
6. **Workflows** - Ordered execution of workflow steps

### Technology Stack

- **Backend**: Go programming language with Google ADK and wazero
- **Frontend**: React UI compiled into the Go binary with light/dark mode support
- **Database**: PostgreSQL for configuration storage and job queuing
- **API**: OpenAI-compatible API as the main interface to workflows
- **Containerization**: Multi-stage Docker builds with scratch final stage

## Project Structure

```
mule/
├── cmd/api/              # Main application entry point
├── frontend/             # React frontend application
├── internal/             # Core internal packages
│   ├── agent/            # Agent runtime implementation
│   ├── api/              # API middleware and utilities
│   ├── database/         # Database connection and migrations
│   ├── engine/           # Workflow execution engine
│   ├── frontend/         # Embedded frontend serving
│   ├── manager/          # WASM module manager
│   ├── primitive/        # Core data structures and interfaces
│   └── validation/       # Input validation
├── pkg/                  # Shared packages
│   ├── database/         # Database models
│   ├── job/              # Job processing and storage
│   └── model/            # Domain models
├── scripts/              # Development and deployment scripts
└── ...
```

## Building and Running

### Prerequisites

- Go 1.24 or later
- PostgreSQL 12 or later
- Node.js 18+ (for frontend development)

### Development Setup

1. **Database Setup**:
   ```sql
   CREATE DATABASE mulev2;
   CREATE USER mule WITH PASSWORD 'mule';
   GRANT ALL PRIVILEGES ON DATABASE mulev2 TO mule;
   ```

2. **Build and Run**:
   ```bash
   # Install dependencies
   go mod tidy
   
   # Build the application
   make build
   
   # Run the server
   ./cmd/api/bin/mule -db "postgres://mule:mule@localhost:5432/mulev2?sslmode=disable"
   
   # Or use the Makefile
   make run
   ```

### Docker Development

```bash
# Build the Docker image
docker build -t mule:latest .

# Start with Docker Compose
docker-compose up -d

# View logs
docker-compose logs -f mule
```

## API Endpoints

### Core API
- `GET /health` - Health check endpoint
- `GET /v1/models` - List available AI models
- `POST /v1/chat/completions` - OpenAI-compatible chat completions

### Management API
- `GET/POST/PUT/DELETE /api/v1/providers` - AI provider management
- `GET/POST/PUT/DELETE /api/v1/tools` - Tool management
- `GET/POST/PUT/DELETE /api/v1/agents` - Agent management
- `GET/POST/PUT/DELETE /api/v1/workflows` - Workflow management
- `GET /api/v1/workflows/{id}/steps` - Workflow step management
- `GET /api/v1/jobs` - Job listing
- `GET /api/v1/jobs/{id}` - Job details
- `GET /api/v1/jobs/{id}/steps` - Job step details
- `GET/POST/PUT/DELETE /api/v1/wasm-modules` - WASM module management

### Real-time
- `WS /ws` - WebSocket endpoint for real-time job updates

## Development Conventions

### Code Organization

- **cmd/** - Contains main applications
- **internal/** - Private packages that shouldn't be imported by external projects
- **pkg/** - Public packages that can be imported by external projects
- **API design** - Follow REST conventions with proper error handling

### Testing

```bash
# Run all tests
make test

# Run linting
make lint

# Run with hot reload
make air
```

### Database Migrations

Migrations are embedded directly in the binary using Go's embed package. The initial schema is defined in `internal/database/migrations/0001_initial_schema.sql`.

## Key Components

### Agent Runtime (`internal/agent/runtime.go`)

Handles agent execution using Google ADK. Supports OpenAI-compatible API requests and responses.

### Workflow Engine (`internal/engine/engine.go`)

Processes workflows through a worker pool system. Jobs are queued in PostgreSQL and processed asynchronously.

### WASM Executor (`internal/engine/wasm.go`)

Executes WebAssembly modules using the wazero library. Modules are stored in the database and loaded on demand.

### Primitive Store (`internal/primitive/store_pg.go`)

Manages all core primitives (providers, tools, agents, workflows) in PostgreSQL.

### Job Store (`pkg/job/store_pg.go`)

Handles job persistence and processing state management.

## Frontend Development

The React frontend is compiled into the Go binary. During development:

```bash
cd frontend
npm install
npm start
```

For production builds:
```bash
npm run build
```

## Configuration

The application can be configured via command-line flags:

```bash
./mule -db "postgres://user:pass@host:5432/dbname?sslmode=disable" -listen ":8080"
```

- `-db`: PostgreSQL connection string
- `-listen`: HTTP listen address

## Deployment

### Single Binary Distribution

All components are compiled into a single Go binary with embedded frontend assets, requiring no external runtime dependencies.

### Docker Deployment

Multi-stage Docker builds create minimal container images. The Docker Compose setup includes both the Mule application and PostgreSQL database.