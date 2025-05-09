# internal/config Package
## Overview
Manages application configuration and default settings. Provides type-safe access to configuration parameters through the `Config` struct.

## Key Components
- **Config Struct**: Central storage for all configuration values
- **LoadConfig()**: Initializes and validates the configuration
- **GetSetting<T>()**: Type-safe retrieval of configuration values

## Usage Example
```go
// Get a GitHub token from config
config.GetSetting[string]("github_token")
```
