# Go Dependency Fix Agent Quick Reference

## Essential Commands

### Basic Dependency Management
```bash
go mod tidy          # Resolve missing entries, clean up go.mod/go.sum
go mod download      # Download modules to local cache
go mod verify        # Verify dependencies haven't been modified
go list -m all       # List all dependencies
```

### Adding/Updating Dependencies
```bash
go get module/path                    # Get latest version
go get module/path@version           # Get specific version
go get module/path@commit            # Get specific commit
go get -u module/path                # Update to latest version
```

### Diagnosing Issues
```bash
go mod why module/path               # Why is this dependency needed?
go mod graph                         # Show full dependency graph
go env                               # Show Go environment
go env GOMOD                         # Show current module file
```

### Multi-Module Repositories
```bash
# Work with specific module
cd /path/to/module
go mod tidy

# List all modules in repo
find . -name "go.mod" -exec dirname {} \;
```

## Common Error Patterns and Fixes

### 1. Missing go.sum entries
**Error**: "missing go.sum entry for module"
**Fix**: `go mod tidy`

### 2. Cannot find module
**Error**: "cannot find module providing package"
**Fix**: 
1. Check import path correctness
2. `go mod tidy`

### 3. Version conflicts
**Error**: "conflicting dependencies" or "ambiguous import"
**Fix**:
```bash
go get conflicting/module@specific_version
go mod tidy
```

### 4. Invalid local paths
**Error**: "invalid version: unknown revision"
**Fix**: Add replacement in go.mod
```
replace github.com/module => ../local/path
```

## Directory Context Verification

Always verify before running commands:
```bash
pwd                    # Current directory
go env GOMOD          # Current module file path
basename $(go env GOMOD)  # Just the go.mod filename
```

## Verification Commands

After making changes:
```bash
go build ./...        # Build all packages
go test ./...         # Test all packages
go mod verify         # Verify module integrity
```

## Repository-Specific Information

### Module Structure
- Main module: `github.com/mule-ai/mule` (repository root)
- Example modules: `github.com/mule-ai/mule/examples/wasm/*`

### Working Directories
- Main module: `/data/jbutler/git/mule-ai/mule`
- Example modules: `/data/jbutler/git/mule-ai/mule/examples/wasm/*`

### Useful Makefile Targets
```bash
make tidy             # go mod tidy
make lint             # Run linter (includes dependency checks)
make test             # Run tests
```

## Safety Reminders

1. **Always check directory context** before running Go commands
2. **Run go mod tidy** after any dependency changes
3. **Verify fixes work** with build/test commands
4. **Don't assume** - always confirm error is resolved
5. **Be methodical** - one change at a time, verify each step