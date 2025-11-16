# Mule

Mule is an AI workflow platform that enables users to create, configure, and execute complex AI-powered workflows. It combines the power of AI agents, custom tools, and WebAssembly modules to create flexible and extensible automation pipelines.

## Documentation

- [Product Requirements Document](MULE-V2.md) - Complete specification of Mule v2
- [Data Model Diagram](DATA_MODEL.md) - Entity relationship diagram showing database schema
- [Sequence Diagram](SEQUENCE_DIAGRAM.md) - Workflow execution flow and component interactions
- [Software Architecture](SOFTWARE_ARCHITECTURE.md) - High-level system architecture
- [Primitives Relationship](PRIMITIVES_RELATIONSHIP.md) - How core primitives relate to each other
- [Component Interaction](COMPONENT_INTERACTION.md) - Detailed component interaction diagram

## Overview

Mule consists of a few core primitives:
* **AI providers** - connections to models, supporting OpenAI compliant APIs
* **Tools** - extensible tools that can be provided to agents
* **WASM modules** - imperative code execution using the wazero library
* **Agents** - combination of a model, system prompt, and tools using Google ADK
* **Workflow Steps** - either a call to an Agent or execution of a WASM module
* **Workflows** - ordered execution of workflow steps

## Technology Stack

* **Backend**: Go programming language with Google ADK and wazero
* **Frontend**: React UI compiled into the Go binary with light/dark mode support
* **Database**: PostgreSQL for configuration storage and job queuing
* **API**: OpenAI-compatible API as the main interface to workflows

## Key Features

* Fully static React frontend compiled into Go binary
* Workflow builder with drag-and-drop interface
* Per-step and full workflow execution with real-time output streaming
* Background job processing with configurable worker pools
* Synchronous and asynchronous execution modes
* Light and dark UI modes

For detailed technical specifications, see the [Product Requirements Document](MULE-V2.md).