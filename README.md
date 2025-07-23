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

## Docs

Documentation is available on [muleai.io](https://muleai.io/docs)

## Key Features

* Multi-agent workflows for complex tasks
* RAG (Retrieval Augmented Generation) for better code understanding
* MCP (Model Context Protocol) support for extensible tool integration
* Multiple AI provider support (OpenAI, Anthropic, local models)
* Web UI and CLI interfaces
* Discord and Matrix chat integrations
* gRPC and REST API support

## Contributing

* Find an issue marked `good first issue`
* Open a Pull Request

## To Do

* Add the ability to create a new repository
* Implement manager mode to allow spawning multiple agents that track their own repository 
