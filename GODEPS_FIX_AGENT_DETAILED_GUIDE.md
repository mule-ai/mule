# Go Dependency Fix Agent Detailed Guide

This document provides detailed procedures for handling the specific Go dependency issues you're designed to fix.

## 1. Missing go.sum Entries

### Identification
- Error message contains: "missing go.sum entry"
- Often occurs after manually editing go.mod or switching branches

### Diagnosis Process
1. Identify the affected module by checking the error message
2. Confirm current directory with `pwd` and `go env GOMOD`
3. Check if go.sum exists: `ls -la go.sum`
4. Verify which dependencies are missing: `go list -m all`

### Fix Procedure
```
# Navigate to the correct module directory
cd /path/to/module

# Run tidy to populate missing entries
go mod tidy

# Verify fix
go list -m all | grep missing_module
```

### Verification
- Error should no longer occur
- go.sum should contain checksums for all dependencies
- Project should build successfully

## 2. Incorrect Module Paths

### Identification
- Error message contains: "cannot find module" or "malformed module path"
- Import statements don't match module declarations
- Replacement directives pointing to wrong locations

### Diagnosis Process
1. Check module declaration in go.mod
2. Compare with import statements in .go files
3. Look for replace directives that might be incorrect
4. Verify module path matches repository structure

### Fix Procedure
```
# For incorrect module declaration
# Edit go.mod to correct the module line
# module github.com/correct/path

# For incorrect import paths in code
# Update import statements to match actual module path

# For local replacements
# Ensure replace directive points to correct relative path
replace github.com/module/path => ../relative/path

# Always tidy after changes
go mod tidy
```

### Common Mistakes to Avoid
- Using file system paths in import statements
- Mismatched module names between go.mod and expected imports
- Incorrect relative paths in replace directives

## 3. Multi-Module Repository Navigation

### Structure Recognition
- Multiple go.mod files in different directories
- Each go.mod represents an independent module
- Modules can depend on each other through versioning or replacements

### Working with Nested Modules
```
# To work on main module
cd /repo/root
go mod tidy

# To work on nested module
cd /repo/root/sub/module
go mod tidy

# To see all modules
find . -name "go.mod" -exec dirname {} \;
```

### Cross-Module Dependencies
When modules in the same repository depend on each other:
```
# In go.mod of dependent module
require github.com/my/repo/submodule v0.0.0

replace github.com/my/repo/submodule => ../submodule
```

## 4. Proper Command Sequencing

### Standard Sequence for Dependency Changes
1. `go mod download` - Populate module cache (if needed)
2. Make changes to go.mod (manually or via go get)
3. `go mod tidy` - Clean up and resolve dependencies
4. `go mod verify` - Check integrity
5. `go build ./...` - Verify builds work
6. `go test ./...` - Verify tests pass

### When Adding New Dependencies
```
# Preferred method - let go mod tidy handle it
# Add import to .go file, then:
go mod tidy

# Alternative - explicit get
go get github.com/some/module@version
go mod tidy
```

### When Removing Dependencies
```
# Remove import from .go file, then:
go mod tidy
```

### Resolving Version Conflicts
```
# See why dependency is needed
go mod why github.com/conflicting/module

# Force specific version
go get github.com/conflicting/module@v1.2.3

# Tidy to resolve everything
go mod tidy
```

## Error-Specific Solutions

### "go: updates to go.mod needed"
```
go mod tidy
```

### "missing go.sum entry for module"
```
go mod tidy
```

### "cannot find module providing package"
1. Verify import path is correct
2. Check if dependency is in go.mod
3. Run `go mod tidy`

### "invalid version: unknown revision"
1. Check if it's a local path that needs replacement
2. Verify the version exists
3. For local development: `go mod edit -replace=module/path=../local/path`

### "ambiguous import"
1. Run `go mod why` on both packages
2. Resolve version conflicts with `go get`
3. Use `go mod tidy`

## Verification Checklist

Before reporting success:
- [ ] Error message no longer appears
- [ ] Project builds with `go build ./...`
- [ ] Tests pass with `go test ./...`
- [ ] No unexpected changes to go.mod or go.sum
- [ ] Commands executed in correct directory context
- [ ] All modules in repository work correctly

## Repository-Specific Procedures

For this repository (github.com/mule-ai/mule):
1. Main module is in repository root
2. Example modules are in examples/wasm/* 
3. Focus on main module unless specifically asked about examples
4. Use Makefile commands when appropriate:
   - `make tidy` for dependency resolution
   - `make lint` to verify overall health
   - `make test` to verify fixes don't break anything

## Troubleshooting Complex Issues

If simple solutions don't work:
1. Clear module cache: `go clean -modcache`
2. Re-download dependencies: `go mod download`
3. Check for private modules requiring authentication
4. Verify GOPROXY settings: `go env GOPROXY`
5. Check git status for uncommitted changes affecting modules

Always explain what you're doing and why at each step.