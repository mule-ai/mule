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

### Testing Strategy

- Unit tests for pure functions (event parsing, config conversion)
- Integration tests for full execution paths
- Manual testing for streaming and WebSocket behavior

