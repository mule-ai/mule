# CRUSH.md

## Build/Lint/Test Commands
```bash
make build          # Build the mule binary for Linux
make run            # Build and run the application
make air            # Run with hot-reload for development
make all            # Clean, format, test, and build everything
make test           # Run linting and all tests
go test ./...       # Run all tests without linting
go test -v ./pkg/agent/  # Run tests for specific package
go test -v ./... -run TestName  # Run specific test by name
make fmt            # Format all Go code
make lint           # Run golangci-lint
make tidy           # Update dependencies
```

## Code Style Guidelines

### Imports
- Group imports by standard library, external modules, and internal packages
- Use `golangci-lint` to enforce import ordering
- Avoid unused imports

### Formatting
- Use `go fmt` for formatting
- Follow Go naming conventions (PascalCase for exported names, camelCase for unexported)

### Types
- Use explicit types over type inference when clarity is improved
- Prefer `time.Time` for time values
- Use `context.Context` for cancellation and timeouts

### Naming Conventions
- Use descriptive names that indicate purpose (e.g., `issueID` instead of `id`)
- Follow Go conventions: PascalCase for exported identifiers, camelCase for unexported
- Use short, clear names for local variables

### Error Handling
- Always handle errors explicitly
- Log errors with context when possible
- Use `errors.Is()` and `errors.As()` for error type checking

### Testing
- Write tests for all packages
- Use table-driven tests where appropriate
- Test both success and error cases