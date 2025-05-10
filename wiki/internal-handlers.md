# internal/handlers Package
## Overview
Handles web API request routing and business logic implementation. Contains handlers for:
- GitHub integrations
- Local repository operations
- Logs management
- Settings configuration
- Repository state tracking

## Key Components
- **GitHubHandler**: Manages GitHub webhook and API interactions
- **LocalProviderHandler**: Handles local repository operations
- **LogHandler**: Implements log retrieval and filtering
- **SettingsHandler**: Manages settings persistence and updates

## Dependency Diagram
```mermaid
graph TD
    A[internal/handlers] --> B[pkg/remote/github]
    A --> C[pkg/repository/local]
    A --> D[internal/state]
    A --> E[pkg/agent]
```
