# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Mule is an AI-powered development team tool written in Go that monitors git repositories and automatically completes issues assigned to it. The system uses AI agents to generate code changes and create pull requests.

## Common Development Commands

### Building and Running
```bash
make build          # Build the mule binary for Linux
make run           # Build and run the application
make air           # Run with hot-reload for development
make all           # Clean, format, test, and build everything
```

### Testing
```bash
make test          # Run linting and all tests
go test ./...      # Run all tests without linting
go test -v ./pkg/agent/  # Run tests for specific package
go test -v ./... -run TestName  # Run specific test by name
```

### Code Quality
```bash
make fmt           # Format all Go code
make lint          # Run golangci-lint
make tidy          # Update dependencies
```

### Running the Application
```bash
./cmd/mule/bin/mule --server  # Web interface mode (default port :8083)
./cmd/mule/bin/mule --cli     # CLI workflow mode
```

## High-Level Architecture

### Directory Structure
- `cmd/mule/` - Main application entry point and web UI assets
- `internal/` - Internal packages (config, handlers, scheduler, settings, state)
- `pkg/` - Public packages containing core domain logic:
  - `agent/` - AI agent system and workflow execution
  - `remote/` - Git provider abstractions (GitHub, local)
  - `repository/` - Repository management and issue processing
  - `integration/` - Extensible integrations (Discord, Matrix, gRPC, API)
  - `rag/` - Retrieval Augmented Generation for context awareness
- `api/proto/` - gRPC protocol buffer definitions

### Core Components

1. **Agent System** (`pkg/agent/`)
   - Agents: AI-powered code generators with configurable models
   - Workflows: Multi-step processes chaining agents
   - Tools: File operations and other capabilities
   - Validation: Quality control functions

2. **State Management** (`internal/state/`)
   - Thread-safe centralized application state
   - Manages repositories, agents, workflows, and integrations
   - Coordinates scheduler for periodic tasks

3. **Repository Management** (`pkg/repository/`)
   - Tracks local git repositories
   - Syncs with remote providers (GitHub, local)
   - Generates code changes based on issues
   - Creates and manages pull requests

4. **Configuration** (`internal/config/`)
   - YAML-based configuration in `~/.config/mule/config.yaml`
   - Manages repositories, agents, workflows, and settings
   - Supports hot-reloading

### Workflow Execution
1. Repository detects issue with "mule" label
2. Workflow triggered with issue context
3. Agents process using configured steps
4. Changes validated and committed
5. Pull request created
6. Further refinement via PR comments

### Key Patterns
- Provider pattern for AI and git providers
- Event-driven integration system
- Template-based prompt generation
- Thread-safe state management with mutexes

### Development Notes
- Requires Go 1.24.0 and CGO (for SQLite)
- Uses Air for hot-reload development
- gRPC server runs on port 9090
- Web UI available on port 8083
- Supports multiple AI providers (OpenAI, Anthropic, local)
- RAG system enhances code understanding context