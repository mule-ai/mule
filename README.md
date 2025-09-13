# Mule

### Your Multi-Agent AI Development Team

Mule is a sophisticated multi-agent workflow engine that automates software development tasks using AI. It combines deterministic workflow orchestration with intelligent AI agents to handle complex development workflows, from issue resolution to code generation and deployment.

## 🎯 Core Capabilities

- **Multi-Agent Workflows**: Orchestrate specialized AI agents for different tasks (architect, coder, reviewer, tester)
- **Intelligent Issue Resolution**: Automatically processes GitHub issues labeled with `mule`
- **RAG-Powered Context**: ChromeM vector database provides semantic search across codebases
- **Multiple AI Providers**: Support for OpenAI, Anthropic, Google AI, and local models via Ollama
- **Production Integrations**: Discord, Matrix, RSS feeds, and comprehensive APIs
- **Advanced Memory System**: Persistent memory for context and learning across workflows
- **Validation Framework**: Built-in code quality and testing validation

## 🚀 Quick Start

### Prerequisites
- Go 1.24+ (as specified in go.mod)
- Git with SSH keys configured for GitHub access
- AI provider access (Ollama recommended for local setup)

### Installation

```bash
git clone https://github.com/mule-ai/mule.git
cd mule
make run
```

The web interface will be available at:
- **Web UI**: http://localhost:8083
- **gRPC API**: localhost:9090
- **REST API**: http://localhost:8083/api

### Basic Usage

1. **Label an issue** with `mule` in GitHub or create one in the local provider
2. **Mule automatically detects** the labeled issue and analyzes requirements
3. **AI agents collaborate** to plan, implement, and validate the solution
4. **Pull request created** with the implemented solution
5. **Request refinements** by commenting on the PR

## Demo

Below is a quick demo of the agent interaction workflow using the local provider. This same workflow can be done using a GitHub provider and performing these steps in the GitHub UI.

https://github.com/user-attachments/assets/f891017b-3794-4b8f-b779-63f0d9e97087

## 🏗️ Architecture

### Multi-Agent System
- **Architect Agent**: Analyzes requirements and designs solutions
- **Code Agent**: Implements features and fixes based on specifications  
- **Reviewer Agent**: Reviews code quality and suggests improvements
- **Tester Agent**: Creates and runs tests to validate functionality

### Workflow Engine
- **Sequential Orchestration**: Step-by-step agent coordination
- **Validation Pipeline**: Automated quality checks (formatting, linting, testing)
- **Error Recovery**: Intelligent retry and refinement mechanisms
- **Context Preservation**: Maintains conversation and code context across agents

## 🔧 Configuration

### Default Setup
```yaml
providers:
  - name: ollama
    models: [qwen2.5-coder:32b, qwq:32b-q8_0]
    
workflows:
  - name: code-generation
    agents: [architect, coder]
    validations: [goFmt, goModTidy, golangciLint, goTest]
    
integrations:
  - discord: enabled
  - matrix: enabled  
  - rss: enabled
  - grpc: port 9090
```

### Advanced Configuration
- **Custom Agents**: Define specialized agents for your domain
- **Validation Functions**: Custom validation rules and quality checks
- **Provider Selection**: Configure multiple AI providers with fallbacks
- **Memory Management**: Persistent context and learning configuration

## 📚 Features

### ✅ **Production Ready**
- **RAG (Retrieval-Augmented Generation)**: ChromeM-based vector database with repository indexing
- **Multi-Agent Workflows**: Sequential and parallel agent orchestration
- **Multiple AI Providers**: OpenAI, Anthropic, Google AI, Ollama support
- **Production Integrations**: Discord bot, Matrix client, RSS feeds, gRPC/HTTP APIs
- **Validation Framework**: Go toolchain integration (goFmt, goTest, golangciLint, etc.)
- **Repository Management**: GitHub integration and local Git operations
- **Memory System**: Persistent memory for context and learning
- **Authentication**: SSH and token-based GitHub authentication

### 🔄 **In Development**
- **Event-Based Actions**: Webhook triggers and real-time responses
- **Visual Workflow Designer**: Low-code workflow creation interface
- **Advanced Orchestration**: Hierarchical agents and consensus mechanisms
- **Enterprise Security**: RBAC, audit logging, and compliance features
- **Image Support**: Multimodal AI for visual context and analysis

## 🌐 Integrations

### Communication Platforms
- **Discord**: Bot for workflow triggers and notifications
- **Matrix**: Decentralized chat integration
- **RSS**: Automated feed monitoring and processing

### Development Tools
- **GitHub**: Issue tracking, PR management, repository operations
- **Git**: Local repository management and version control
- **Testing**: Automated test execution and validation

### AI Providers
- **OpenAI**: GPT-4, GPT-3.5-turbo support
- **Anthropic**: Claude-3 family integration  
- **Google AI**: Gemini Pro and vision models
- **Local Models**: Ollama for private, offline inference

## 📖 Documentation

- **API Reference**: [API.md](./API.md) - Comprehensive gRPC and REST API documentation
- **Architecture Guide**: [wiki/architecture.md](./wiki/architecture.md) - System design and components
- **Package Documentation**: [wiki/](./wiki/) - Detailed package and module guides
- **Examples**: [examples/](./examples/) - Sample workflows and integrations

## 🛠️ Development

### Contributing
1. Find an issue marked `good first issue`
2. Fork the repository
3. Create a feature branch
4. Submit a Pull Request

### Running Tests
```bash
make test
```

### Building from Source
```bash
make build
```

### Development Mode
```bash
make dev
```

## 🚀 API Access

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

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🤝 Community

- **Issues**: Report bugs and request features on [GitHub Issues](https://github.com/mule-ai/mule/issues)
- **Discussions**: Join conversations on [GitHub Discussions](https://github.com/mule-ai/mule/discussions)
- **Documentation**: Visit [muleai.io](https://muleai.io/docs) for guides and tutorials

---

**Mule** - Automating software development through intelligent multi-agent workflows. 
