# Mule

Mule is an AI workflow platform that enables users to create, configure, and execute complex AI-powered workflows. It combines the power of AI agents, custom tools, and WebAssembly modules to create flexible and extensible automation pipelines.

## Quick Start with Docker

The easiest way to get started is using Docker Compose:

```bash
# Clone the repository
git clone https://github.com/mule-ai/mule.git
cd mule

# Start the services
docker-compose up -d

# Check the services are running
docker-compose ps

# Access the application
# Web UI: http://localhost:8080
# API: http://localhost:8080/v1
# Health check: http://localhost:8080/health
```

The Docker Compose setup includes:
- **Mule API Server**: The main application on port 8080
- **PostgreSQL Database**: Configured with the Mule database schema on port 5432

### Docker Development

```bash
# Build the Docker image
docker build -t mule:latest .

# Run with custom database connection
docker run -p 8080:8080 \
  -e DB_CONN_STRING="postgres://user:pass@host:5432/dbname?sslmode=disable" \
  mule:latest

# View logs
docker-compose logs -f mule
docker-compose logs -f postgres
```

## Manual Installation

### Prerequisites

- Go 1.24 or later
- PostgreSQL 12 or later
- Node.js 18+ (for frontend development)

### Database Setup

1. Create a PostgreSQL database:
```sql
CREATE DATABASE mulev2;
CREATE USER mule WITH PASSWORD 'mule';
GRANT ALL PRIVILEGES ON DATABASE mulev2 TO mule;
```

2. Run the database migration:
```bash
psql -h localhost -U mule -d mulev2 -f internal/db/migrations/0001_initial_schema.sql
```

### Building and Running

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

## Documentation

- [Product Requirements Document](MULE-V2.md) - Complete specification of Mule v2
- [Data Model Diagram](DATA_MODEL.md) - Entity relationship diagram showing database schema
- [Sequence Diagram](SEQUENCE_DIAGRAM.md) - Workflow execution flow and component interactions
- [Software Architecture](SOFTWARE_ARCHITECTURE.md) - High-level system architecture
- [Primitives Relationship](PRIMITIVES_RELATIONSHIP.md) - How core primitives relate to each other
- [Component Interaction](COMPONENT_INTERACTION.md) - Detailed component interaction diagram

## Overview

Mule consists of a few core primitives:
* **AI providers** - connections to models, supporting OpenAI compliant APIs
* **Tools** - extensible tools that can be provided to agents
* **WASM modules** - imperative code execution using the wazero library
* **Agents** - combination of a model, system prompt, and tools using Google ADK
* **Workflow Steps** - either a call to an Agent or execution of a WASM module
* **Workflows** - ordered execution of workflow steps

## Technology Stack

* **Backend**: Go programming language with Google ADK and wazero
* **Frontend**: React UI compiled into the Go binary with light/dark mode support
* **Database**: PostgreSQL for configuration storage and job queuing
* **API**: OpenAI-compatible API as the main interface to workflows
* **Containerization**: Multi-stage Docker builds with scratch final stage

## Key Features

* Fully static React frontend compiled into Go binary
* Workflow builder with drag-and-drop interface
* Per-step and full workflow execution with real-time output streaming
* Background job processing with configurable worker pools
* Synchronous and asynchronous execution modes
* Light and dark UI modes
* Docker support with multi-stage builds for minimal container size
* Health checks and graceful shutdown
* Built-in tools including filesystem, HTTP, database, memory, and bash command execution
* WebSocket support for real-time updates

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

### Real-time
- `WS /ws` - WebSocket endpoint for real-time job updates

## Configuration

The application can be configured via command-line flags:

```bash
./mule -db "postgres://user:pass@host:5432/dbname?sslmode=disable" -listen ":8080"
```

- `-db`: PostgreSQL connection string (default: `postgres://user:pass@localhost:5432/mulev2?sslmode=disable`)
- `-listen`: HTTP listen address (default: `:8080`)

## Development

### Frontend Development

```bash
cd frontend
npm install
npm start
```

### Testing

```bash
# Run all tests
make test

# Run linting
make lint

# Run with hot reload
make air
```

### Building

```bash
# Build for production
make build

# Build Docker image
docker build -t mule:latest .
```

For detailed technical specifications, see the [Product Requirements Document](MULE-V2.md).