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

#### 7. Forge Conversation Step (Herd H3.2)

- `forge_conversation` workflow step type: a job talks to a long-lived
  forge agent conversation (addressed by agent id or pinned
  conversation id), optionally waits for the turn, and returns the
  turn's assistant text as step output. Full contract in
  ["Forge Conversation Step Type" below](#forge-conversation-step-type-herd-h32).

#### 8. Per-Job WebSocket Subscriptions (Herd H3.4)

- WS clients can subscribe per job (`{"op":"subscribe","job_id":…}` /
  `unsubscribe` / `subscribe_all`); never-subscribed clients keep the
  full firehose. Full contract in
  ["Per-Job WebSocket Subscriptions" below](#per-job-websocket-subscriptions-herd-h34).

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

- **pi**: @mariozechner/pi-coding-agent (npm package)
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

## Tenancy & Authentication (Herd H3.1)

Mule supports multi-user deployments. The full contract lives in
**[`docs/TENANCY.md`](docs/TENANCY.md)**; summary:

- `users` + `api_keys` (migration `0012_tenancy.sql`): SHA-256 key
  hashes, `key_prefix`, `restricted` flag, optional `expires_at`; argon2id
  password hashes. `owner_id` on agents/workflows/jobs/skills/providers.
- `Authorization: Bearer sk_mule_<hex>` is required on every
  `/api/v1/*` and `/v1/*` route; unknown/expired → 401, restricted
  writes → 403. `/health` and `/ws` stay unauthenticated.
- First-boot bootstrap creates the operator's admin from
  `MULE_ADMIN_*` (id defaults to the documented
  `00000000-0000-4000-8000-000000000001`; generated credentials are
  logged once). Idempotent on later boots.
- Non-admins see only their own primitives; foreign objects read as
  404. `MULE_NO_AUTH=1` restores the legacy single-operator behavior
  with a synthetic admin context (local/ranch dev).

## Forge Conversation Step Type (Herd H3.2)

A `forge_conversation` workflow step (migration `0013_forge_step.sql`
widens the `workflow_steps.step_type` CHECK to admit it) lets a mule
job drive a **long-lived forge agent conversation**: it resolves a
target conversation by address, posts a templated message tagged with
its mule job, optionally waits for the turn to end, and returns the
turn's assistant text as the step output. Implementation:
`internal/engine/forge.go` (`processForgeConversationStep`), validated
by `validation.ValidateForgeConversationStep`.

### Config shape

All step config lives in the `workflow_steps.config` JSONB column
(`primitive.WorkflowStep.Config`):

```json
{
  "forge_url":       "http://10.10.199.29:8110",  // required, http/https
  "api_key":         "sk_forge_…",                // required (v1: inlined, see below)
  "agent_id":        "<forge agent uuid>",        // one of agent_id | conversation_id
  "conversation_id": "<forge session uuid>",      // optional pin; wins over agent_id
  "mode":            "message",                   // message | steer | follow_up | fork (default message)
  "content":         "Summarize: {{.prompt}}",    // required; Go text/template over the job input map
  "wait":            true,                        // optional, default false
  "timeout_secs":    300                          // optional, default 300, positive int
}
```

`content` is rendered with Go `text/template` (`missingkey=error`) over
the job input map — the same "render over the input" contract the
agent step's prompt uses (agent steps pass `inputData["prompt"]` through
verbatim; this step generalizes it to the whole input map).

### Addressing resolution order

1. Pinned `conversation_id` wins: use it as-is.
2. `mode: fork`: `agent_id` is required; mule calls
   `POST /agents/{agent_id}/conversations` with
   `fork_from` = the pinned conversation (or the agent's active one,
   from `GET /agents/{agent_id}/active`) and posts to the NEW session
   id returned in `{"session":{"id":…}}`.
3. `agent_id`: `GET /agents/{agent_id}/active` →
   `{busy, current_conversation}`; when `current_conversation` is null
   the step creates one via `POST /agents/{agent_id}/conversations`.

v1 mode semantics: `message` / `steer` / `fork` are the real
behaviors; `follow_up` is accepted as an alias of `message` (forge
has a single `POST /messages` entry point — steer is a forge-side
capability mule will pin down post-H2).

### Messages posted

POST `/messages {"session_id", "content"}` (202). Forge's message API
accepts plain content with no provenance field, so mule-origin tagging
is a **content prefix** in v1:
`[mule job <job_id> <workflow_id>] <rendered content>`.
(Upgrade path: a forge `source`/`origin` field on messages, or a
mule-claims table — not in H3 scope.)

All calls carry `Authorization: Bearer <api_key>`.

### wait / cancel / timeout semantics

- `wait: false` (default): the step returns as soon as the message is
  accepted (202). Step output: `{forge_conversation: <session id>,
  output: <posted content incl. prefix>}`.
- `wait: true`: mule snapshots the conversation's highest message
  sequence, posts the message, then waits for the turn to end with
  **SSE as the primary signal**: `GET /sessions/{id}/events?since=N&
  oneshot=true` — forge emits `message` rows and a final
  `turn_ended` event and closes the stream. A messages-table fallback
  covers the attach race (turn ends before the SSE connection
  attaches): a post-turn assistant row is treated as turn-end.
- **Timeout** (`timeout_secs`, default 300): the step FAILS with
  `timed out waiting for forge turn …` and the workflow stops. The
  forge turn **keeps running** — mule gives up *waiting*; it does not
  interrupt the agent (use forge's `POST /sessions/{id}/interrupt`
  separately when you want that).
- **Cancellation** (`DELETE /api/v1/jobs/:id` → engine context
  cancel): the wait aborts promptly — the SSE request and the
  fallback poll loop both select on the job context, and no poll
  goroutine is left behind (the wait runs in the step's own
  goroutine). The job is marked cancelled; the forge turn keeps
  running, same as on timeout.
- On success the step output is `{forge_conversation: <session id>,
  output: <turn's final assistant text>}` (fetched from
  `GET /messages?session_id=…` filtered to the post-turn sequence
  range), which flows into the next step as its input map.

### v1: api_key in config; upgrade path

The forge key (`sk_forge_…`) is **inlined in the step config** in v1.
Known trade-off: the key is visible to anyone who can read the
workflow row (mule tenancy scopes that to the owner + admin, and
restricted keys cannot write workflows). The documented upgrade path
is a `secrets` table + `api_key_ref` in the config (mule resolves the
reference at step time; config stores only the ref), mirroring how
providers already keep `api_key_encrypted` under
`MULE_ENCRYPTION_SECRET`. The engine's config parser is the single
place to change; step behavior does not move.

## Per-Job WebSocket Subscriptions (Herd H3.4)

The `/ws` hub (`internal/api/websocket.go`) is a broadcast firehose by
default. Clients may now narrow it per job with inbound control
messages (JSON text frames, acked synchronously):

```json
{"op":"subscribe","job_id":"…"}      → {"op":"subscribed","job_id":"…"}
{"op":"unsubscribe","job_id":"…"}    → {"op":"unsubscribed","job_id":"…"}
{"op":"subscribe_all"}               → {"op":"subscribed_all"}
```

Filtering rules (per client, thread-safe on the client's own lock):

- **Never subscribed** (no subscribe op ever sent): receives
  everything — the backward-compatible default the ranch mule worker
  panes rely on.
- **Subscribed to ≥1 job**: receives only frames whose job id is in
  the subscription set (`job_update`, `job_step_update`). Frames
  without a job id (agent events) are withheld for filtered clients;
  `subscribe_all` restores the full firehose.
- `unsubscribe` removes a job; a client that has ever subscribed and
  then unsubscribes everything receives nothing until it subscribes
  again or sends `subscribe_all`.

`JobStreamer` (the polling tail that emits job status changes) goes
through the same `BroadcastJobUpdate`/`BroadcastJobStepUpdate` path,
so it inherits the filter automatically. Acceptance is covered by
`internal/api/websocket_test.go` (H3.4 acceptance: A-subscribed vs
B-subscribed vs subscribe-all vs default clients; step-update
filtering).

## Dependencies & Constraints

### External Dependencies

- @mariozechner/pi-coding-agent package from npm
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
