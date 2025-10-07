# Mule - Multi-Agent AI Development Team

## Project Overview

Mule is a sophisticated multi-agent workflow engine that automates software development tasks using AI. It combines deterministic workflow orchestration with intelligent AI agents to handle complex development workflows, from issue resolution to code generation and deployment.

### Core Capabilities
- Multi-Agent Workflows: Orchestrate specialized AI agents for different tasks (architect, coder, reviewer, tester)
- Intelligent Issue Resolution: Automatically processes GitHub issues labeled with `mule`
- RAG-Powered Context: ChromeM vector database provides semantic search across codebases
- Multiple AI Providers: Support for OpenAI, Anthropic, Google AI, and local models via Ollama
- Production Integrations: Discord, Matrix, RSS feeds, and comprehensive APIs
- Advanced Memory System: Persistent memory for context and learning across workflows
- Validation Framework: Built-in code quality and testing validation

### Technology Stack
- **Language**: Go 1.24
- **AI Integration**: Custom genai library for LLM providers
- **Version Control**: go-git for Git operations
- **Web Framework**: Standard library HTTP with embedded templates
- **gRPC**: Protocol Buffers v3 with gRPC-Go for API services
- **Database**: SQLite for chat history and memory
- **Configuration**: Viper for YAML configuration management
- **Logging**: Structured logging with logr and zap

## Project Structure

```
mule/
├── cmd/               # Command-line tools
│   ├── mule/         # Main application with web UI
│   └── memory-cli/   # Memory management CLI
├── internal/         # Private packages
│   ├── config/      # Configuration management
│   ├── handlers/    # HTTP handlers
│   ├── scheduler/   # Workflow scheduling
│   ├── settings/    # Application settings
│   └── state/       # Global application state
├── pkg/             # Public packages
│   ├── agent/       # AI agent implementation
│   ├── auth/        # SSH authentication utilities
│   ├── integration/ # External integrations
│   ├── log/         # Logging utilities
│   ├── rag/         # Retrieval-augmented generation
│   ├── remote/      # Remote providers (GitHub, local)
│   ├── repository/  # Repository management
│   └── validation/  # Validation framework
├── api/             # gRPC/Protocol Buffer definitions
├── wiki/            # Technical documentation
├── examples/        # Usage examples
├── .github/         # GitHub workflows
├── Makefile         # Build and development commands
├── go.mod           # Go module dependencies
└── README.md        # Project documentation
```

## Building and Running

### Prerequisites
- Go 1.24+ (as specified in go.mod)
- Git with SSH keys configured for GitHub access
- AI provider access (Ollama recommended for local setup)

### Development Commands
```bash
# Install dependencies
go mod download

# Run tests
make test

# Format code
make fmt

# Run linters
make lint

# Build the application
make build

# Run with hot-reload for development
make air

# Build and run the application
make run

# Clean, format, test, and build everything
make all
```

### Running Modes
1. **Server Mode**: Web interface for managing repositories and settings
   ```bash
   ./cmd/mule/bin/mule --server
   ```
2. **CLI Mode**: Direct workflow execution with prompts
   ```bash
   ./cmd/mule/bin/mule --workflow <workflow_name> --prompt "<prompt>"
   ```

## API Access

### gRPC API (Port 9090)
High-performance, type-safe API for:
- Workflow execution and monitoring
- Agent management and control
- Provider configuration
- System health and metrics

### REST API (Port 8083)
HTTP/JSON API for:
- Web application integration
- Simple automations
- Health checks and status
- Configuration management

See [API.md](./API.md) for complete API documentation and examples.

## Development Workflow

### 1. Create a Branch
```bash
git checkout -b feature/your-feature-name
# or
git checkout -b fix/issue-description
```

### 2. Make Changes
- Follow Go coding standards
- Write tests for new functionality
- Update documentation as needed
- Keep commits focused and atomic

### 3. Testing
```bash
# Run all tests
make test

# Run specific package tests
go test ./pkg/agent

# Run with race detection
go test -race ./...

# Integration tests
make test-integration
```

### 4. Code Quality
```bash
# Format code
make fmt

# Run linters
make lint

# Security checks
make security-check
```

## Core Components

### Agent System (`pkg/agent/`)
- Agents: AI-powered code generators with configurable models
- Workflows: Multi-step processes chaining agents
- Tools: File operations and other capabilities
- Validation: Quality control functions

### State Management (`internal/state/`)
- Thread-safe centralized application state
- Manages repositories, agents, workflows, and integrations
- Coordinates scheduler for periodic tasks

### Repository Management (`pkg/repository/`)
- Tracks local git repositories
- Syncs with remote providers (GitHub, local)
- Generates code changes based on issues
- Creates and manages pull requests

### Configuration (`internal/config/`)
- YAML-based configuration in `~/.config/mule/config.yaml`
- Manages repositories, agents, workflows, and settings
- Supports hot-reloading

## Workflow Execution

1. Repository detects issue with "mule" label
2. Workflow triggered with issue context
3. Agents process using configured steps
4. Changes validated and committed
5. Pull request created
6. Further refinement via PR comments

## Key Patterns

- Provider pattern for AI and git providers
- Event-driven integration system
- Template-based prompt generation
- Thread-safe state management with mutexes

## Development Notes

- Requires Go 1.24.0 and CGO (for SQLite)
- Uses Air for hot-reload development
- gRPC server runs on port 9090
- Web UI available on port 8083
- Supports multiple AI providers (OpenAI, Anthropic, local)
- RAG system enhances code understanding context

## Contributing

See [CONTRIBUTING.md](./CONTRIBUTING.md) for detailed contribution guidelines.

### Quick Start for Contributors
1. Fork and clone the repository
2. Install dependencies with `go mod download`
3. Run tests with `make test`
4. Start development server with `make dev`

### Code Standards
- Follow Go coding standards and Effective Go guidelines
- Use golangci-lint for static analysis
- Write comprehensive tests with >80% coverage
- Document all exported functions and packages