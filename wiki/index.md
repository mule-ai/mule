# Project Wiki
## Overview
This project follows a modular architecture with distinct packages handling specific responsibilities. The main components include:

- **cmd/mule**: Entry point with web handlers
- **internal/** packages: Core business logic and state management
- **pkg/** packages: Shared utilities and domain-specific functionality
- **repository/** packages: Data access and storage operations
- **handlers/**: Web API implementation
- **remote/**: External service integrations (e.g., GitHub)

## Navigation
1. [Architecture Diagram](architecture.md)
2. Package Documentation:
   - [internal/config](internal-config.md)
   - [internal/handlers](internal-handlers.md)
   - [internal/scheduler](internal-scheduler.md)
   - [pkg/agent](pkg-agent.md)
   - [pkg/repository](pkg-repository.md)
   - [pkg/remote](pkg-remote.md)
   - [pkg/validation](pkg-validation.md)
