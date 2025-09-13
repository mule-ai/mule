# Contributing to Mule

Thank you for your interest in contributing to Mule! This document provides guidelines and information for contributors.

## 🚀 Getting Started

### Prerequisites

- Go 1.24+ (as specified in go.mod)
- Git with SSH keys configured
- Basic understanding of:
  - Go programming
  - AI/LLM concepts
  - Workflow automation
  - Multi-agent systems

### Development Setup

1. **Fork and Clone**
   ```bash
   git clone https://github.com/YOUR_USERNAME/mule.git
   cd mule
   ```

2. **Install Dependencies**
   ```bash
   go mod download
   ```

3. **Run Tests**
   ```bash
   make test
   ```

4. **Start Development Server**
   ```bash
   make dev
   ```

## 📋 How to Contribute

### Finding Issues to Work On

1. Look for issues labeled `good first issue` for beginners
2. Check `help wanted` issues for general contributions
3. Review our [project roadmap](https://github.com/mule-ai/mule/issues) for strategic initiatives

### Reporting Bugs

When reporting bugs, please include:

- **Clear description** of the issue
- **Steps to reproduce** the problem
- **Expected vs actual behavior**
- **Environment details** (OS, Go version, AI provider)
- **Relevant logs** or error messages

Use our [bug report template](https://github.com/mule-ai/mule/issues/new?template=bug_report.md).

### Suggesting Features

For feature requests, please:

- **Search existing issues** to avoid duplicates
- **Provide clear use cases** and benefits
- **Consider implementation complexity**
- **Align with project goals** and architecture

Use our [feature request template](https://github.com/mule-ai/mule/issues/new?template=feature_request.md).

## 🛠️ Development Workflow

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

### 5. Submit Pull Request

- **Fill out the PR template** completely
- **Link related issues** using keywords (fixes #123)
- **Request review** from relevant maintainers
- **Respond to feedback** promptly

## 📝 Coding Standards

### Go Style Guide

- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use [golangci-lint](https://golangci-lint.run/) for static analysis
- Keep functions focused and testable
- Use meaningful variable and function names

### Architecture Principles

- **Separation of concerns**: Keep packages focused
- **Dependency injection**: Use interfaces for testability
- **Error handling**: Always handle errors explicitly
- **Concurrency safety**: Use proper synchronization

### Documentation

- **Package documentation**: Every package needs a doc.go file
- **Public API documentation**: All exported functions must be documented
- **README updates**: Update relevant documentation for user-facing changes
- **Code comments**: Explain complex logic and business rules

### Testing

- **Unit tests**: Test individual functions and methods
- **Integration tests**: Test component interactions
- **Coverage**: Aim for >80% test coverage
- **Mocks**: Use interfaces and dependency injection for testing

## 🏗️ Project Structure

```
mule/
├── cmd/               # Command-line tools
│   ├── mule/         # Main application
│   └── memory-cli/   # Memory management CLI
├── internal/         # Private packages
│   ├── config/      # Configuration management
│   ├── handlers/    # HTTP handlers
│   └── scheduler/   # Workflow scheduling
├── pkg/             # Public packages
│   ├── agent/       # AI agent implementation
│   ├── integration/ # External integrations
│   ├── rag/        # Retrieval-augmented generation
│   ├── remote/     # Remote providers (GitHub, etc.)
│   └── validation/ # Validation framework
├── wiki/           # Technical documentation
├── examples/       # Usage examples
└── api/           # gRPC/Protocol Buffer definitions
```

## 🔍 Code Review Process

### Review Criteria

- **Functionality**: Does the code work as intended?
- **Design**: Is the solution well-architected?
- **Readability**: Is the code easy to understand?
- **Testing**: Are there adequate tests?
- **Performance**: Are there any performance concerns?
- **Security**: Are there any security implications?

### Review Timeline

- Initial review within 2-3 business days
- Follow-up reviews within 1 business day
- Urgent fixes may be fast-tracked

## 🎯 Areas for Contribution

### High-Priority Areas

1. **Multi-Agent Orchestration**
   - Advanced workflow patterns
   - Agent coordination algorithms
   - Performance optimization

2. **AI Provider Integration**
   - New model providers
   - Provider-specific optimizations
   - Fallback and retry logic

3. **Enterprise Features**
   - Security and compliance
   - Monitoring and observability
   - Authentication systems

4. **Developer Experience**
   - Visual workflow designer
   - Better error messages
   - Improved documentation

### Good First Issues

- Documentation improvements
- Unit test additions
- Bug fixes in isolated components
- Example workflow creation
- CLI enhancements

## 📚 Resources

### Documentation

- [Architecture Guide](./wiki/architecture.md)
- [API Documentation](./API.md)
- [Package Documentation](./wiki/)

### Learning Resources

- [Multi-Agent Systems](https://en.wikipedia.org/wiki/Multi-agent_system)
- [Go Best Practices](https://github.com/golang/go/wiki/CodeReviewComments)
- [AI/LLM Integration Patterns](https://docs.anthropic.com/claude/docs)

### Communication

- **GitHub Issues**: Bug reports and feature requests
- **GitHub Discussions**: General questions and ideas
- **Pull Requests**: Code contributions and reviews

## 🏆 Recognition

Contributors will be recognized through:

- **Contributors list** in repository
- **Release notes** for significant contributions
- **GitHub badges** and achievements
- **Maintainer nomination** for consistent contributors

## 📄 License

By contributing to Mule, you agree that your contributions will be licensed under the same [MIT License](LICENSE) as the project.

## ❓ Questions?

If you have questions about contributing, please:

1. Check existing [GitHub Discussions](https://github.com/mule-ai/mule/discussions)
2. Open a new discussion for general questions
3. Create an issue for specific bugs or features
4. Review our [documentation](./wiki/) for technical details

Thank you for helping make Mule better! 🚀