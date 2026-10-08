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

## Wakes (Herd H3.3)

`wakes` is mule's **fleet-level wake-policy engine**: a *wake* is a
condition plus a target plus a prompt template. The scheduler
(`internal/wake`, started from `cmd/api/server.go` alongside the
workflow engine) fires due wakes — it renders the template and either
submits a mule job or posts a message to a forge agent conversation.
Ranch's machine-local trigger scheduler stays the edge (cron/event/
webhook firing mule jobs or local panes); wakes is the fleet superset
that can address any forge agent from any condition, including
conditions only mule sees (job completed, memory trigger, agent
signal). Tables: `wakes` + `wake_log` (migration `0014_wakes.sql`).

### Kinds

| kind           | spec                              | lane            | status |
|----------------|-----------------------------------|-----------------|--------|
| `cron`         | `{expr}`                          | 30 s due-scan   | live   |
| `webhook`      | `{source, tag?}` (tagless matches any tag for that source) | `POST /api/v1/wakes/fire` | live |
| `event`        | `{type}` (e.g. `job.completed`, `job.failed`, any external type) | engine job-event sink + fire endpoint | live |
| `job`          | `{workflow_id, status}` (`completed`\|`failed`) | engine job-event sink | live |
| `memory`       | `{agent_id, predicate}`           | —               | **inert in H3**: validated + stored, never fires; H4.5 wires it to the memory store |
| `agent_signal` | `{agent_id, kinds[]}`             | —               | **inert in H3**: validated + stored, never fires; H4.6 wires it to the agent signal bus |

### Target

- `target.workflow_id` — the fire submits a mule job on that workflow
  with input `{"prompt": <rendered template>, "payload": <fire payload>}`;
  the job id is recorded in `wake_log.job_id`.
- `target.forge_conversation` — an H3.2 step-config map
  (`{forge_url, api_key, agent_id | conversation_id}`); the fire posts
  the rendered prompt as a plain message (prefixed `[mule wake <id>]`),
  fire-and-forget — no turn wait, wakes are cheap. A bare `agent_id`
  without forge credentials is not addressable in v1 (400 at create);
  a shared fleet forge config is the documented upgrade path.

### Fire path

1. **Rate-limit guardrail** (runaway proactivity, research §10.3, the
   H5.3 kill-switch ancestor): if the wake's `wake_log` already has
   ≥ `rate_limit_per_hour` rows in the trailing hour, the fire is
   **dropped** and logged (`ok=false, error='rate_limited'`) — no
   submission. Enforced in the control plane, not the agent.
2. Render `prompt_template` (Go `text/template`, `missingkey=error`)
   over `{wake_id, wake_name, kind, source, payload, fired_at}`.
3. Submit per the target (above).
4. Record `wake_log {payload, job_id?, ok, error}` and — for cron
   wakes — advance `last_fired_at`/`next_due_at` in the **same
   transaction** (`Store.RecordFire`), so a fire cannot be double-
   counted by the scan lane.

Cron wakes advance `next_due_at` even on submission failure: the
schedule stays on cadence and the rate limit bounds a broken target to
`rate_limit_per_hour` error rows per hour. Every attempt (ok, failed,
rate-limited) lands in `wake_log`, so throttling is visible in the
audit trail (and, in H5, the Activity View).

### Cron subset

Faithful Go port of the ranch daemon's 5-field cron semantics
(`ranch-2/crates/ranch/src/triggers.rs`, "minimal cron"): fields are
minute hour day-of-month month day-of-week; each field supports `*`,
`*/n`, single values, inclusive ranges `a-b`, and lists `a,b,c`.
Ranges: 0-59 / 0-23 / 1-31 / 1-12 / 0-6 (0 = Sunday; `7` is not
accepted). UTC. No `?`, `L`, `W`, names, or range-steps. Next-fire is
computed by minute probing (bounded 366 days) — the same brute force
as the Rust reference. Invalid expr at create/update → 400.
`next_due_at` is computed at create/update and after each fire.

### API & auth

- `GET/POST /api/v1/wakes`, `GET/PATCH/DELETE /api/v1/wakes/{id}` —
  owner-scoped (H3.1 `owner_id` pattern; admin sees all).
- `GET /api/v1/wakes/{id}/log` — newest first, limit 100.
- `POST /api/v1/wakes/fire {kind: "webhook"|"event", source, event_tag?,
  payload?}` — the external ingress (webhooks + external event pushes).
  `kind=webhook` matches `spec {source, tag?}`; `kind=event` treats
  `source` as the event TYPE (matches `spec {type}`).

Auth choice (deliberate): wakes are the **proactivity primitive** —
the H3.1 write-restriction that applies to agent/workflow/job/policy
routes does NOT apply to wake CRUD; any valid owner key has full CRUD
on its own wakes. The fire endpoint accepts any valid key: it is an
ingress, not a mutation (it triggers other owners' already-published
wakes fleet-wide, which is the point of a fleet ingress).

### Engine event hook

`Engine.SetJobEventSink` (wired in `Scheduler.Start()`) receives a
`JobEvent` (`job.completed` / `job.failed`, with job + workflow ids)
from `processJob`'s terminal paths. The scheduler publishes them to
its bounded `EventBus` (drop-oldest + warn when full), which the
event lane matches against `event`/`job` wakes.

### Ranch integration

ranchd's `WebhookEvent` handling gains an optional `daemon.toml
[wakes] mule_forward = true` (+ `wakes_url`/`wakes_api_key`): when a
webhook event arrives locally, ranchd fire-and-forget POSTs
`{kind:"webhook", source, event_tag, payload}` to the mule fire
endpoint — a single webhook can drive both local panes and fleet
agents. A mule failure is logged and dropped; it never fails local
webhook handling. The existing trigger → mule job path is unchanged.

Acceptance (`internal/wake/wake_test.go`, `cmd/api/wake_tenancy_test.go`):
cron wake fires once per window against a fake forge (no double fire
on rescan); webhook fire endpoint wakes the agent with a rendered
prompt; `job.completed` on a real engine job fires the event-kind wake
with the job id in the payload; `rate_limit_per_hour=2` drops the
third fire with `error='rate_limited'` in `wake_log`; workflow targets
submit a job (`wake_log.job_id` set); user B cannot see user A's
wakes; inert kinds are accepted and never fire.

## Policy Engine (Herd H3.5)

mule's control plane for autonomous agent action: **versioned,
owner-scoped bundles of deterministic rules** that answer
`(agent_id, tool, input)` with a synchronous verdict
`{allow | deny | ask, rule_id, reason}`. Mule owns the store +
evaluator (`internal/policy`, migration `0015_policies.sql`:
`policies` + `policy_audit` + `policy_memo`); forge's `before_tool`
hook is the enforcement point; ranch is the approval UI.

### Rule shape

```json
{
  "id": "rm-guard",
  "description": "rm -rf is dangerous",
  "action": {"tool": "bash", "match": "rm -rf"},
  "sensitivity": "sensitive",
  "disposition": "deny",
  "reason": "rm -rf is blocked by operator policy",
  "reviewer": false,
  "review_mode": "agent"
}
```

- `action.tool` ∈ `bash | write | edit | web | email | *`.
- `disposition` ∈ `allow | deny | ask`; `sensitivity` ∈
  `low | normal | sensitive` (the dots tiers: sensitive data →
  named-recipient rules; destructive/irreversible or money → always
  `ask`; credentials/money transfer → `deny` with a
  hand-back-to-human reason).
- Rules validate at write time: non-empty array, unique non-empty
  ids, and `action.match` must PARSE as a glob (`*`/`?` dialect, ≤ 512
  chars; empty match = "every call of this tool") — bad shapes are
  400 at creation, never at evaluate time.
- `reviewer` + `review_mode` are v1 STUBS (see below).

### Matching semantics (v1, deterministic, documented)

A rule's `match` (when present) fires when EITHER of two legs hits —
these are the ONLY two legs:

1. **Rendered-line glob.** The input is rendered as `field=value`
   lines (top-level fields sorted lexicographically; string values
   unquoted, every other value compact JSON). The match glob is
   ANCHORED and matched against EACH rendered line; one hit is
   enough. Glob dialect: `*` = any sequence of characters INCLUDING
   `/`, `?` = exactly one character, everything else literal.
2. **Bash raw-command substring (bash only).** When the tool is
   `bash` and the input has a string `command` field, the match
   string must occur as a LITERAL SUBSTRING of the raw command (no
   glob interpretation on this leg: `*`/`?` are literal here).

This second leg is why a global deny rule
`{"action":{"tool":"bash","match":"rm -rf"}}` blocks
`sudo rm -rf /tmp/x`: the rendered `command=...` line does not START
with `rm -rf` (the anchored line-glob leg misses), but the raw
substring leg fires. One rule, two legs, no ambiguity; a rule with no
`match` matches every call of its tool.

### Decision order (most specific, then most restrictive)

1. **MEMO FIRST** (below).
2. **Active policies**: enabled policies that are same-owner OR
   global-scoped (admin callers see all owners); agent-scope
   policies apply only to their `agent_id`.
3. **MOST-SPECIFIC-SCOPES-WIN**: `agent > org > global`. The first
   scope with ANY matching rule decides; later scopes are not
   consulted. Within the winning scope, every matching rule is
   collected and the **MOST-RESTRICTIVE disposition wins
   (`deny > ask > allow`)**; a full tie resolves to the earliest
   rule id (as stored in the policy), deterministically.
4. **No matching policy**: non-blocking default `allow` with
   `rule_id ""` and reason `"no matching policy"` — and NOTHING is
   memoized (the default is not a decision; a policy created later
   must take effect immediately).

### Action hash + memo (and why resume is safe)

The memo table is keyed by `(rule_id, agent_id, action_hash)` where
`action_hash = sha256(tool + "\n" + canonical JSON of the input)`
(sorted-key compact JSON — key order and whitespace irrelevant; nil
input canonicalizes to `null`). Semantics:

- **Memo writes**: the winning `allow` and `deny` verdicts are
  upserted (idempotent, last deterministic evaluation wins). `ask`
  and the no-match default are NEVER written: an `ask` is a pending
  question, not a decided answer — an unapproved ask re-asks on
  retry, and the human's "yes" is what eventually lands in the memo
  (the H2.5 forge hook path records it).
- **Memo lookup** is rule-agnostic: ALL memo rows for
  `(agent_id, action_hash)` are read and the MOST-RESTRICTIVE
  verdict wins. A deny memoed under one rule still covers the action
  after rule churn — deleting the original rule cannot un-deny a
  previously-blocked action.
- **Resume safety**: the verdict is a pure function of
  `(agent_id, tool, input, policy set, memo)` — deterministic and
  stateless apart from the memo — so a resumed/interrupted turn
  re-evaluating the SAME tool call gets the SAME verdict (or the
  memoized one) instead of re-asking the human or drifting across
  scopes. The hash is over the semantic input, not any rendered
  string, so re-serialization cannot change the answer.

### v1: `reviewer: true` = deterministic floor

When the winning rule has `reviewer: true` and `review_mode:
"agent"`, the evaluator returns the deterministic verdict
IMMEDIATELY with `review: "pending"` — the deterministic verdict is
the FLOOR. The review-agent upgrade path (a cheap mule agent step
that can only LOOSEN the floor to a documented subset) is a
follow-up, not built in v1; forge never blocks on a review agent,
and evaluate stays a synchronous p99 < 50 ms decision query.

### API & auth

- `GET/POST /api/v1/policies`, `GET/PATCH/DELETE
  /api/v1/policies/{id}` — owner-scoped CRUD (H3.1 pattern; admin
  sees all). Every content change bumps `version` and writes a
  `policy_audit` row IN THE SAME TRANSACTION (actor = the caller's
  user id, or `system`); a no-content patch is a 200 with no bump.
  `policy_audit.policy_id` is `ON DELETE SET NULL`, so a policy's
  `'deleted'` row survives the deletion as the permanent record that
  it existed and who removed it.
- `POST /api/v1/policies/evaluate {agent_id, tool, input?}` — the
  forge `before_tool` hook's synchronous surface.
- `POST /api/v1/policies/memo {agent_id, tool, input?, verdict:
  allow|deny, rule_id?, reason?}` → `{recorded: true, verdict}` —
  record a DECIDED verdict for an action: the "human said yes/no"
  half of an `ask` round-trip (H3.5 part 2). The action hash is the
  evaluator's `HashAction`, so the endpoint can't drift from what
  `evaluate` keys on; the upsert goes through the same shared SQL
  path. Only `allow`/`deny` are accepted (an `ask` is a pending
  question; the no-match default is not a decision — 400 otherwise).
  Open to ANY valid key, restricted agent keys included: writing a
  memo row is recording a decision about the caller's own agent, not
  writing policy (same carve-out as `evaluate`).
- `POST /api/v1/policies/proposals {draft: {scope, agent_id?,
  rules}}` → `{proposal_id}` — an agent (restricted keys included)
  DRAFTS a new policy: a `policy_audit` row with `change='proposal'`
  and `policy_id NULL`, whose id IS the proposal id. It creates no
  policy.
- `POST /api/v1/policies/proposals/{id}/approve` — the HUMAN side:
  non-restricted key only. Materializes the draft as a new policy
  (owner = the APPROVER, version 1, enabled) and morphs the row to
  `'approved-proposal'` pointing at it, in one transaction.

Auth choice (deliberate, the H3.1 restricted-class carve-out): CRUD
and approve stay RESTRICTED CLASS — a restricted (agent) key may
read but not write. **`evaluate`, `memo`, and `proposals` stay open to any
valid key, including restricted agent keys**: evaluate is the
agent's own control-plane query ("may I do this?" — it only writes
its own agent's memo row), memo is recording a decided verdict about
the caller's own agent (the H3.5 part-2 ask round-trip; recording a
decision, not writing policy), and proposals are agent DRAFTS; none
materializes a policy. The loop is thus: agent proposes → human
(non-restricted key) approves → policy takes effect. (Implemented as
`restrictedWriteExemptPaths` in `internal/api/auth.go`; everything
else under `/api/v1/policies` stays restricted class.)

Acceptance (`internal/policy/policy_test.go`,
`cmd/api/policy_tenancy_test.go`): deterministic matrix — global
deny `bash`/`rm -rf` (raw-substring leg) vs agent-scope allow
`write`, no-policy default, agent > global specificity, within-scope
deny > ask > allow, full-tie rule-id ordering; memo — second
Evaluate returns `memoized: true`, most-restrictive memo wins across
rule ids, ask + default never memoized; action-hash stability
(key-order independent, nil → `"null"`); API — version bump + audit
rows (POST v1, PATCH v2, `rules changed v1→v2`, no-op no-bump,
`disabled v3`), owner isolation on evaluate, restricted key 403 on
CRUD + approve but 200 on evaluate/memo/proposals, ask → memo
allow → second evaluate `memoized: true` (a different action under
the same rule still asks), 400 on 'ask' verdict memo / missing
fields, proposal → approve
flow (`approved-proposal` audit row, approver-owned), 400 on bad
disposition / sensitivity / agent-scope-without-agent_id, and the
MULE_NO_AUTH=1 synthetic-admin path.

## Reflection Workflow (Herd H4.4)

The Herd's learning loop is a **real mule workflow**, shipped as
`examples/reflection/` and seeded against a mule instance by
`seed.sh` (idempotent; annotated manifest in
`examples/reflection/reflection.json`). It turns one forge agent's
recent **episodes** into **reviewed belief diffs**:

```
wake (cron 0 3 * * * / event) → job on workflow "agent-reflection":
  1. wasm  pull    → forge memory API: recall-query episode search
                     (N queries × k≤50) + active beliefs → {episodes,
                     beliefs, forge creds} (v1 creds flow: module config)
  2. agent step    → cheap mule agent (pi-based, no tools) proposes
                     belief diffs as STRICT JSON [{op:
                     add|strengthen|weaken|forget|supersede, belief,
                     confidence, rationale, source_episodes}], max 10
  3. wasm  check   → deterministic gate (internal/reflection/gate):
                     duplicate (normalized string, pending+active +
                     batch-local), contradiction (v1 string heuristic:
                     negation-marker flip + token overlap ≥ 0.4),
                     decay (confidence·exp(−λ·age), λ default 0.02/day),
                     op routing (add-only endpoint ⇒ everything else
                     needs a human)
  4. wasm  submit  → POST /agents/:id/memory/beliefs/proposals:
                     safe adds as-is; needs_review with a
                     "[needs human review] <reason>" rationale prefix
                     (they become pending beliefs a human acts on);
                     exact duplicates skipped; per-belt
                     accepted/rejected_reason + the decay report =
                     the job's output_data.
```

Governance after step 4 is forge's (H4.4 part 1, already shipped):
proposals land as `beliefs status='pending'` + `memory_review`
approval cards (ranch Keep/Forget/Edit); Keep → active → the next
turn's `memory:beliefs` prompt section.

v1 decisions & documented limits (full detail in
`examples/reflection/README.md`):

- **Step 2 is a plain mule agent step, not `forge_conversation`**: a
  one-shot cheap-model inference over bounded input; parking the loop
  in the reflected-on agent's own conversation is what the plan
  avoids (the loop is mule behavior; only governance is forge's).
- **Recall-based pull**: forge has no episode-list endpoint, so pull
  uses N recall queries × k=50 over `GET /agents/:id/memory/search`
  (503 when the embedding endpoint is down ⇒ failed step, not a
  silent no-op run). A cheap episode-list endpoint + the H4.5
  episode-count trigger close this.
- **Triggers**: nightly cron wake + a pre-created `episodes_ready`
  event wake fired by hand (`POST /api/v1/wakes/fire`); automatic
  N-new-episodes detection is the H4.5 memory trigger.
- **Credentials**: inline in the pull/check/submit module config
  (same stance as H3.2 `forge_conversation`); secrets table +
  `api_key_ref` is the upgrade path. The engine injects `mule_time`
  (host wall clock) into every wasm step's input — WASI preview1 has
  no clock (time.Now() is pinned to the WASI epoch), so without it
  the decay pass would be dead; config/input may override it (tests
  pin it).
- **Tests**: gate + parser are host-unit-tested
  (`internal/reflection/{gate,reflmem}`); `internal/engine/
  reflection_test.go` runs the real .wasm binaries through the real
  engine against a fake forge — including a full 4-step job with the
  agent step replaced by a fixed-output stand-in (agent steps spawn
  `pi --mode rpc` and cannot be faked in-process; the real-model leg
  is the manual acceptance).

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
