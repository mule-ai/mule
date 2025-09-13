# Mule

### Your AI development team

mule is an AI Agent that monitors your git repositories and completes issues assigned to it.

Issues are assigned by giving them the `mule` label.

After the work is completed, the agent will create a pull request. Additional refinement can be requested by commenting on the pull request.

When the pull request is closed or merged, no more work will be completed unless the issue is reopened.

It is intended that the agent will be able to work on multiple issues at once through the creation of multiple pull requests.

## Demo

Below is a quick demo of the agent interaction workflow using the local provider. This same workflow can be done using a GitHub provider and performing these steps in the GitHub UI.

https://github.com/user-attachments/assets/f891017b-3794-4b8f-b779-63f0d9e97087

## Installation

### Prerequisites
- Go 1.24+ (as specified in go.mod)
- Git with SSH keys configured for GitHub access (if using GitHub repositories)
- AI provider access (Ollama recommended for local setup)

### Quick Start
```bash
git clone https://github.com/mule-ai/mule.git
cd mule
make run
```

The web interface will be available at http://localhost:8083

### Configuration
Default configuration includes:
- Ollama provider with qwen2.5-coder:32b and qwq:32b-q8_0 models
- Code generation workflow with architect and code agents
- Validation functions: goFmt, goModTidy, golangciLint, goTest, getDeps

## Docs

Documentation is available on [muleai.io](https://muleai.io/docs)

## Contributing

* Find an issue marked `good first issue`
* Open a Pull Request

## Features

* ✅ **RAG (Retrieval-Augmented Generation)**: ChromeM-based vector database with repository indexing and semantic search
* ✅ **Multi-agent workflows**: Sequential workflow orchestration with agent specialization and validation
* ✅ **Multiple AI Providers**: OpenAI, Ollama, and Google AI support via external genai library
* ✅ **Production Integrations**: Discord bot, Matrix client, RSS feeds, gRPC server, and memory management
* ✅ **Validation Framework**: Go toolchain integration (goFmt, goTest, golangciLint, etc.)
* ✅ **Repository Management**: GitHub integration and local Git operations
* 🔄 **Manager mode**: Work in progress for multiple agent spawning 
