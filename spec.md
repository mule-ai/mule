# Mule PI Integration Specification

## Header Section

- **Project Name**: Mule PI Integration
- **Version**: 1.0.0
- **Date Created**: February 15, 2026
- **Status**: Draft

## Problem Statement

Mule currently uses a custom Google ADK-based agent runtime that requires significant maintenance and doesn't leverage the full power of the pi coding agent ecosystem. The current implementation:

- Uses Google ADK for agent execution, which is complex and has heavy dependencies
- Has a custom tool registry system with limited extensibility
- Lacks support for the Agent Skills standard
- Doesn't support the rich pi ecosystem (extensions, prompt templates, themes)
- Has manual tool execution loops that are error-prone

We need to replace this with pi in RPC mode, which provides:
- A battle-tested agent runtime with proven tools (read, write, edit, bash, grep, find, ls)
- Built-in support for the Agent Skills standard
- Extensible architecture via extensions, skills, prompt templates, and themes
- Clean RPC protocol for headless operation
- Full system access for code generation tasks

## Goals & Success Criteria

### Primary Goals

1. **Replace Agent Runtime**: Remove Google ADK dependency and integrate pi in RPC mode as the primary agent execution engine
2. **Implement Skills System**: Replace current "tools" concept with skills - users select available skills when creating/editing agents
3. **Preserve Current Capabilities**: Ensure all existing functionality (streaming, tool execution, job management) continues to work
4. **Leverage PI Features**: Support skills, extensions, and other pi features through agent configuration
5. **Tight Integration**: Make pi feel like a native part of Mule, not an external wrapper

### Success Metrics

- Agent execution via pi RPC produces identical responses to current implementation
- Skills can be created, managed, and assigned to agents via API
- Streaming responses work correctly through WebSocket
- All existing workflows continue to function
- No regression in API response times

### Non-Goals

- Keep Google ADK as a fallback option (removed entirely)
- Implement interactive pi TUI mode within Mule
- Migrate existing custom tools to pi extensions (users create new skills)
- Support all pi CLI modes (focus on RPC only)

## Functional Requirements

### User Stories

1. **As a user**, I want to create an agent that uses pi with specific skills, so that I can leverage specialized capabilities
2. **As a user**, I want to manage skills separately from agents, so that I can reuse skills across multiple agents
3. **As a user**, I want to execute an agent and receive streaming responses, so that I can see real-time progress
4. **As a user**, I want to configure pi options (thinking level, provider, model) per agent, so that I have fine-grained control

### Core Features

#### 1. PI Agent Runtime (replaces Google ADK)

- Spawn pi process in RPC mode (`pi --mode rpc`)
- JSON protocol over stdin/stdout for command/response
- Parse events from stdout for streaming
- Support all RPC commands: prompt, steer, follow_up, abort, new_session, etc.
- Handle extension UI requests (select, confirm, input, editor)
- Graceful process management (start, stop, restart on failure)

#### 2. Skills System

- **Skill CRUD**: Create, read, update, delete skills stored in database
- **Skill Storage**: Store skill as directory path or inline content in database
- **Skill Assignment**: Many-to-many relationship between agents and skills
- **Skill Execution**: Pass skill paths to pi via `--skill` flag at startup
- **Skill Discovery**: Support global (~/.pi/agent/skills/), project (.pi/skills/), and custom paths

#### 3. Agent Configuration

- **Provider Configuration**: Support all pi providers (anthropic, openai, google, etc.)
- **Model Selection**: Allow model ID specification per agent
- **Thinking Level**: Configure thinking level (off, minimal, low, medium, high, xhigh)
- **System Prompt**: Current system prompt becomes pi system prompt
- **Working Directory**: Support per-request working directory
- **Session Management**: Per-agent session storage or ephemeral mode

#### 4. Tool Integration

- **Built-in Tools**: PI provides read, bash, edit, write, grep, find, ls by default
- **Skill Tools**: Skills can add additional tools
- **Tool Options**: Allow disabling specific tools via `--tools` or `--no-tools`

#### 5. Streaming & Events

- Parse pi event stream (agent_start, message_update, tool_execution_start, etc.)
- Convert pi events to Mule WebSocket format
- Support all event types: text deltas, thinking, tool calls, tool results
- Handle extension UI requests via WebSocket

#### 6. Workflow Integration

- Agents used in workflows receive same pi configuration
- Workflow step results flow to next step correctly
- Async workflow support preserved

### User Interactions

1. **Create Agent with Skills**: POST /api/v1/agents with skill_ids array
2. **Add Skills to Agent**: PUT /api/v1/agents/:id/skills with skill_ids
3. **Execute Agent**: POST /v1/chat/completions with model="agent/xyz"
4. **Stream Response**: Connect to WS /ws for real-time events
5. **Manage Skills**: CRUD operations at /api/v1/skills

### Data Flow

```
User Request → HTTP Handler → Agent Runtime → PI RPC Process
                                                    ↓
                                            stdin: commands
                                            stdout: events
                                                    ↓
                                        Event Parser → WebSocket → Client
```

## Technical Requirements

### Languages & Versions

- **Go**: 1.25.4 (current)
- **Node.js**: 20.x (for pi binary execution)
- **PostgreSQL**: Current version (no change)

### Dependencies

- **pi**: @jbutlerdev/pi-coding-agent (npm package)
- **Database**: PostgreSQL (existing)
- **Messaging**: WebSocket (existing)

### Architecture

- **Runtime Pattern**: Spawn pi as subprocess, communicate via JSON-RPC
- **Process Management**: One pi process per agent execution (or session pooling)
- **Event Handling**: Goroutines for stdout parsing, channels for event dispatch
- **Configuration**: Agent settings passed as CLI flags to pi

### Development Style

- Incremental migration (keep working, add pi, then remove ADK)
- Feature flags for gradual rollout
- Comprehensive test coverage

## Non-Functional Requirements

### Performance

- Concurrent agents: Support multiple simultaneous executions

### Reliability

- Automatic pi process restart on crash
- Timeout handling for long-running operations
- Graceful shutdown on server stop

## Data Requirements

### New/Modified Tables

1. **skills** (NEW)
   - id: VARCHAR(255) PRIMARY KEY
   - name: TEXT NOT NULL UNIQUE
   - description: TEXT
   - path: TEXT NOT NULL (skill directory path)
   - enabled: BOOLEAN DEFAULT TRUE
   - created_at: TIMESTAMPTZ
   - updated_at: TIMESTAMPTZ

2. **agent_skills** (NEW - junction table)
   - agent_id: VARCHAR(255) REFERENCES agents(id)
   - skill_id: VARCHAR(255) REFERENCES skills(id)
   - PRIMARY KEY (agent_id, skill_id)

3. **agents** (MODIFIED - remove tool references)
   - Remove tools JSONB column (replaced by skills)
   - Add pi_config JSONB for thinking level, session options, etc.

### Data Migration

- Export existing tool configurations to skill format (if applicable)
- Migrate agent tool assignments to agent skill assignments

## API & Integration Requirements

### Internal APIs

- Agent runtime communicates with pi via stdin/stdout
- Event parsing and conversion happens in-process
- No external API calls to pi services

### External APIs

- Provider APIs remain the same (pi handles auth via providers)
- No new external dependencies

## Testing Requirements

### Testing pi

- pi will run successfully with the following configuration
- `pi -p --provider local-llm --model llamacpp/qwen3-30b-a3b "hi"`

### Unit Tests

- RPC command/response parsing
- Event stream parsing
- Skill CRUD operations
- Agent-skill assignment

### Integration Tests

- Full agent execution flow
- Streaming response verification
- Workflow execution with agents
- WebSocket event delivery

### End-to-End Tests

- Create agent with skills
- Execute agent via API
- Verify streaming response
- Check workflow integration

## Deployment Requirements

### Environments

- Dev: Local pi binary, no changes
- Staging: Same as production
- Production: Standard deployment

### Infrastructure

- No new infrastructure needed
- Node.js 20.x available on runtime hosts

### CI/CD

- Add pi installation to build process (npm install -g)
- Test pi subprocess spawning in CI

## Dependencies & Constraints

### External Dependencies

- @jbutlerdev/pi-coding-agent package from npm
- pi-mono repository for development reference

### Team Constraints

- Single developer for initial implementation
- 2-3 weeks estimated effort

### Time Constraints

- No hard deadline
- Phased rollout acceptable

## Risks & Mitigation

### Technical Risks

1. **PI RPC Protocol Complexity**: Handle all event types correctly
   - Mitigation: Comprehensive event parsing, fallback handling

2. **Process Management**: Graceful start/stop/restart
   - Mitigation: Use成熟 process management library or patterns

3. **Performance Overhead**: Spawning pi per request
   - Mitigation: Consider process pooling if needed

### Resource Risks

1. **Node.js Dependency**: Adding Node.js to Go service
   - Mitigation: Node.js likely already needed for frontend

### Timeline Risks

1. **Scope Creep**: Feature additions during implementation
   - Mitigation: Strict feature freeze after spec approval
