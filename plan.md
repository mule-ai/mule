# Mule PI Integration Development Plan

## Phase 0: Discovery & Planning

- [x] **Objective**: Finalize requirements and prepare development environment
- [x] **Deliverables**: Approved spec document, development environment setup
- [x] **Estimated Duration**: 2 days

**Tasks**:
- [x] Review and finalize spec.md
- [x] Set up development environment with Node.js 20.x
- [x] Install pi globally and test basic RPC mode
- [x] Verify database migration can be created
- [x] Review current agent runtime implementation

## Phase 1: Foundation & Architecture

- [x] **Objective**: Create database schema changes and core data structures
- [x] **Deliverables**: New skills table, agent modifications, primitive store updates
- [x] **Estimated Duration**: 3 days

**Tasks**:
- [x] Create database migration 0008_add_skills_table.sql
- [x] Add Skill type to internal/primitive/primitive.go
- [x] Add AgentSkill junction table methods to PrimitiveStore
- [x] Update internal/database/store_pg.go with skill CRUD operations
- [x] Update internal/manager/agent.go to handle skills
- [x] Create internal/manager/skill.go for skill management

## Phase 2: PI Runtime Core

- [x] **Objective**: Build the pi RPC runtime integration
- [x] **Deliverables**: Working pi subprocess management, RPC command execution
- [x] **Estimated Duration**: 5 days

**Tasks**:
- [x] Create internal/agent/pirc/pibridge.go - pi process management
- [x] Implement stdin command writing (prompt, abort, steer, etc.)
- [x] Implement stdout event stream parsing
- [x] Handle process start/stop/error scenarios
- [x] Add process timeout and resource limits
- [x] Write unit tests for RPC communication

## Phase 3: Event & Streaming

- [x] **Objective**: Convert pi events to Mule streaming format
- [x] **Deliverables**: Working WebSocket streaming for agent execution
- [x] **Estimated Duration**: 3 days

**Tasks**:
- [x] Map pi event types to Mule event types
- [x] Implement message_update event handling (text, thinking, tool calls)
- [x] Handle tool_execution events for real-time output
- [x] Implement extension UI request handling (select, confirm, input)
- [x] Implement extension UI request handling (select, confirm, input)
- [x] Integrate with existing WebSocket infrastructure
- [x] Test end-to-end streaming

## Phase 4: Agent Integration

- [x] **Objective**: Replace Google ADK with pi in agent execution
- [x] **Deliverables**: Full agent execution via pi RPC
- [x] **Estimated Duration**: 4 days

**Tasks**:
- [x] Modify internal/agent/runtime.go to use pi instead of ADK
- [x] Implement agent configuration to pi flag conversion
- [x] Add skill path loading to agent startup
- [x] Handle session management per agent
- [x] Implement working directory handling
- [x] Preserve async workflow execution
- [x] Run comprehensive integration tests

## Phase 5: API & UI Updates

- [x] **Objective**: Expose skills management via API
- [x] **Deliverables**: REST API endpoints for skill CRUD
- [x] **Estimated Duration**: 3 days

**Tasks**:
- [x] Add /api/v1/skills endpoints (CRUD)
- [x] Add /api/v1/agents/:id/skills endpoints
- [x] Update agent creation/edit to support skill selection
- [x] Add skill_id field validation
- [x] Update API documentation
- [x] Test API integration

## Phase 6: Testing & Polish

- [x] **Objective**: Comprehensive testing and bug fixes
- [x] **Deliverables**: Stable, production-ready implementation
- [x] **Estimated Duration**: 3 days

**Tasks**:
- [x] Run existing test suite, fix regressions
- [x] Add new unit tests for pi integration
- [x] Add integration tests for agent execution
- [x] Test WebSocket streaming edge cases
- [x] Test skill assignment and execution
- [x] Performance testing and optimization
- [x] Code review and documentation

## Phase 7: Cleanup & Launch

- [x] **Objective**: Remove old code, final deployment prep
- [x] **Deliverables**: Clean codebase, production deployment
- [x] **Estimated Duration**: 2 days

**Tasks**:
- [x] Remove Google ADK dependencies from go.mod
- [x] Remove old ADK-related code
- [x] Update README with new architecture
- [x] Update CLAUDE.md with new agent runtime info
- [x] Deploy to staging environment
- [x] Run production smoke tests
- [x] Monitor and fix any issues

---

## Implementation Notes

### Key Technical Decisions

1. **Process Management**: Use Go's exec package with proper lifecycle management
2. **Event Parsing**: Line-by-line JSON parsing from stdout
3. **Skill Storage**: Store skill directory paths in database, not content
4. **Configuration**: Agent config stored as JSONB, passed as CLI flags to pi
5. **Streaming**: Direct WebSocket broadcast, no intermediate buffering
6. **forge_conversation step (H3.2)**: step config (forge_url, api_key,
   agent_id/conversation_id, mode, content template, wait, timeout_secs)
   lives entirely in the `workflow_steps.config` JSONB column; the step
   type is admitted by migration `0013_forge_step.sql`.
7. **Per-job WS subscriptions (H3.4)**: opt-in client-side filtering on
   the firehose; never-subscribed clients keep receiving everything.
8. **Wakes (H3.3)**: fleet wake-policy engine in `internal/wake`
   (migration `0014_wakes.sql`). Cron lane scans every 30 s against
   `next_due_at`; event lane consumes a bounded EventBus fed by
   `Engine.SetJobEventSink` (job.completed/job.failed) plus the
   `POST /api/v1/wakes/fire` ingress. Fire = render prompt_template
   (missingkey=error) → rate-limit check (drop + log
   `error='rate_limited'`) → workflow `SubmitJob` or
   `engine.ForgeSubmission.PostMessage` (H3.2 client, fire-and-forget
   → `wake_log` + cron schedule advance in one txn. `memory`/
   `agent_signal` kinds are validated-inert H4 stubs. Cron: faithful
   port of the ranch 5-field subset (`*`, `*/n`, lists, ranges, UTC,
   dow 0-6); invalid expr → 400. Auth: owner-scoped CRUD for any
   valid key; the fire endpoint is a fleet ingress (any valid key).
   Spec: spec.md "Wakes".
9. **Policy engine (H3.5)**: mule's control plane in `internal/policy`
   (migration `0015_policies.sql`: `policies` + `policy_audit` +
   `policy_memo`). A policy = versioned, owner-scoped bundle of
   deterministic rules; the evaluator answers `(agent_id, tool,
   input)` synchronously. Matching: a rule's `match` fires on EITHER
   (1) an anchored glob over each rendered `field=value` line of the
   input, or (2) a literal substring of the raw bash `command` — so
   global deny `bash`/`rm -rf` blocks `sudo rm -rf /tmp/x`. Decision:
   agent > org > global (first scope with a matching rule decides);
   within scope, most-restrictive wins (deny > ask > allow), full tie
   → earliest rule id; no match → non-blocking allow, un-memoized.
   Memo: `sha256(tool + "\n" + canonical input)` keyed; allow/deny
   memoized, ask + default never; lookup is rule-agnostic and the
   most-restrictive memo wins, so a memoed deny survives rule churn
   (resume/retry determinism). v1 `reviewer:true` = deterministic
   floor: verdict returned immediately with `review: "pending"`;
   review-agent upgrade is a documented follow-up. API: owner-scoped
   CRUD (every content change bumps `version` + writes a
   `policy_audit` row in the same txn; audit `policy_id` is
   ON DELETE SET NULL so 'deleted' rows survive),
   `POST /policies/evaluate` (open to restricted keys — the agent's
   own control-plane query), `POST /policies/memo` (record a decided
   verdict for an action — the "human said yes/no" half of an ask
   round-trip; HashAction + the shared upsertMemo path so it can't
   drift from the evaluator; allow/deny only; open to restricted keys
   — recording a decision, not writing policy), `POST /policies/proposals` (open to
   restricted keys — agent drafts) and
   `POST /policies/proposals/{id}/approve` (restricted-class write —
   human materializes the policy, owned by the approver, audit row
   morphs to 'approved-proposal'). One store bug fixed while
   testing: `auditInsert` passed a typed-nil `[]byte` detail,
   which lib/pq encodes as `''` — Postgres rejects that as JSON; now
   an untyped nil → SQL NULL. Tests: `internal/policy/policy_test.go`
   (deterministic matrix, memo semantics, hash stability, owner
   scoping) + `cmd/api/policy_tenancy_test.go` (version/audit, owner
   isolation on evaluate, ask→memo→memoized-allow round-trip,
   restricted-key carve-out incl. memo, proposal→approve
   flow, 400 validation, MULE_NO_AUTH path). Spec: spec.md
   "Policy Engine".
10. **Reflection workflow (H4.4)**: the learning loop ships as a real
   mule workflow in `examples/reflection/` (seeded idempotently by
   `seed.sh`; annotated manifest `reflection.json`):
   pull(wasm, forge memory recall search + active beliefs) → agent
   step (cheap model, STRICT-JSON belief diffs; a plain agent step,
   NOT forge_conversation — one-shot inference, not a conversation
   with the subject) → check(wasm, deterministic gate in
   `internal/reflection/gate`: normalized-string duplicate vs
   pending+active + batch-local, v1 string contradiction heuristic =
   negation-marker flip + Jaccard overlap ≥ 0.4, decay
   confidence·exp(−λ·age) λ default 0.02/day, add-only-endpoint op
   routing) → submit(wasm, POST /agents/:id/memory/beliefs/proposals:
   safe as-is, needs_review with "[needs human review]" rationale
   prefix, exact dups skipped; decay report + per-belt results = job
   output). Creds v1 = module config (secrets table is the upgrade,
   same stance as H3.2). Wakes: cron `0 3 * * *` + an `episodes_ready`
   event wake (manual fire; auto episode-count = H4.5). The engine
   injects `mule_time` (host clock — WASI has none) into every wasm
   step input. Also fixed while testing: `wasmEchoModule` (engine
   test helper) used single-byte LEB sizes and broke past 128 bytes.
   Tests: `internal/reflection/{gate,reflmem}` (host) +
   `internal/engine/reflection_test.go` (real .wasm through the real
   engine vs a fake forge, incl. a full 4-step job with a
   fixed-output stand-in for the agent step — the real-model leg is
   the manual acceptance). Spec: spec.md "Reflection Workflow".
11. **Memory trigger lane (H4.5)**: `internal/wake/memory_trigger.go`
    (started from `cmd/api/server.go` next to the scheduler) is the
    mule host's forwarder for forge's `memory_trigger_queue` (H4.5,
    forge migration 024): when a belief's `watch` matches an episode,
    forge queues a row; this lane polls each configured agent's
    `GET …/memory/triggers/pending` every 30 s and fires the wake
    named by `watch.wake_id` IN-PROCESS via `Scheduler.FireWake`
    (source `memory_trigger`), then ACKs with
    `POST …/triggers/:tid/consumed`. HTTP pull (not the plan's
    shared-DB table poll): forge and mule are separate containers
    with separate Postgres clusters in the lab. Config via env:
    `FORGE_TRIGGER_BASE` / `FORGE_TRIGGER_KEY` / `FORGE_TRIGGER_AGENTS`
    (empty agents ⇒ disabled, no loop). Stale rows (>6 h) and rows
    with no/unknown `wake_id` are acked with a warn (a misconfigured
    belief cannot block the queue). Delivery is at-least-once:
    failed ACKs re-fire on the next poll (bounded by the wake's
    rate_limit_per_hour); failed submissions stay pending; rate-
    limited fires are acked. Tests:
    `internal/wake/memory_trigger_test.go` (fake forge trigger API +
    real mule store on scratch PG: fire+ack, no-wake_id ack, ack-
    failure retry, stale skip, disabled lane). Spec: spec.md "Wakes
    — Memory trigger lane".

### Testing Strategy

- Unit tests for pure functions (event parsing, config conversion)
- Integration tests for full execution paths
- Manual testing for streaming and WebSocket behavior

