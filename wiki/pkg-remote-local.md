# pkg/remote/local Package
## Overview
Provides local repository operations for working with Git repositories without external dependencies. Key functionality includes:
- Local branch management
- File system operations
- Commit history traversal

## Key Components
```go
type LocalProvider interface {
    CloneRepository(url string, dir string) error
    GetBranches() ([]string, error)
    CheckoutBranch(branch string) error
}

func GetLocalClient(repoPath string) (*LocalProvider, error)
```
