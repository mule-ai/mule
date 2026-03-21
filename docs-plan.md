# Mule AI Documentation Automation - Development Plan

## Phase 1: Documentation Review

- [ ] **Phase 1: Documentation Review**
  - **Objective:** Review existing documentation for accuracy and completeness
  - **Deliverables:** List of documentation areas needing improvement
  - **Estimated Duration:** 15 minutes
  - **Dependencies:** None

**Tasks:**
- [ ] Review content/docs/getting-started.md for accuracy
- [ ] Review content/docs/Advanced/ directory structure and content
- [ ] Review content/docs/Repositories/ directory for completeness
- [ ] Review content/docs/Settings/ directory for accuracy
- [ ] Check for broken links within documentation
- [ ] Identify documentation gaps based on current features

---

## Phase 2: Getting Started Improvements

- [ ] **Phase 2: Getting Started Improvements**
  - **Objective:** Improve the getting started guide
  - **Deliverables:** Updated getting-started.md with better instructions
  - **Estimated Duration:** 20 minutes
  - **Dependencies:** Phase 1 complete

**Tasks:**
- [ ] Update installation instructions if needed
- [ ] Add prerequisites section if missing
- [ ] Improve first-run experience documentation
- [ ] Add troubleshooting section if helpful
- [ ] Add screenshots or diagrams if beneficial

---

## Phase 3: Advanced Documentation

- [ ] **Phase 3: Advanced Documentation**
  - **Objective:** Improve advanced feature documentation
  - **Deliverables:** Updated advanced docs with new features
  - **Estimated Duration:** 25 minutes
  - **Dependencies:** Phase 1 complete

**Tasks:**
- [ ] Review integrations.md - add any new integrations
- [ ] Review multi-agent.md - update with latest patterns
- [ ] Review rag.md - add new RAG capabilities
- [ ] Review udiff.md - update with changes
- [ ] Review validation.md - add new validation features
- [ ] Add code examples where helpful

---

## Phase 4: Settings Documentation

- [ ] **Phase 4: Settings Documentation**
  - **Objective:** Ensure settings documentation is accurate
  - **Deliverables:** Updated settings docs
  - **Estimated Duration:** 15 minutes
  - **Dependencies:** Phase 1 complete

**Tasks:**
- [ ] Review agents.md - add new agent capabilities
- [ ] Review ai-providers.md - add new providers if supported
- [ ] Review general.md - update general settings
- [ ] Review system-agent.md - update system agent docs
- [ ] Review workflows.md - add new workflow features

---

## Phase 5: Repository Documentation

- [ ] **Phase 5: Repository Documentation**
  - **Objective:** Ensure repository docs are comprehensive
  - **Deliverables:** Updated repository docs
  - **Estimated Duration:** 15 minutes
  - **Dependencies:** Phase 1 complete

**Tasks:**
- [ ] Review adding-a-repository.md - update instructions
- [ ] Review interacting-with-a-repository.md - add new features
- [ ] Add examples for common repository operations

---

## Phase 6: New Documentation

- [ ] **Phase 6: New Documentation**
  - **Objective:** Add documentation for new features or improvements
  - **Deliverables:** New documentation pages or significant additions
  - **Estimated Duration:** 25 minutes
  - **Dependencies:** Phase 1 complete

**Tasks:**
- [ ] Research latest Mule project developments from SPEC.md and commits
- [ ] Identify features that need documentation
- [ ] Create new documentation pages if needed
- [ ] Add tutorials for common use cases
- [ ] Update _index.md files with navigation improvements

---

## Phase 7: Documentation Polish

- [ ] **Phase 7: Documentation Polish**
  - **Objective:** Ensure all documentation is consistent and polished
  - **Deliverables:** Finalized documentation changes
  - **Estimated Duration:** 15 minutes
  - **Dependencies:** Phases 2-6 complete

**Tasks:**
- [ ] Verify all Markdown files are properly formatted
- [ ] Check frontmatter on all documentation pages
- [ ] Ensure consistent tone and style
- [ ] Verify links work correctly
- [ ] Stage changes for commit

---

## Task Execution Notes

The ralph-sh methodology will execute each unchecked task sequentially using the pi CLI agent. Each task should:
1. Be specific and actionable
2. Complete within the 30-minute timeout
3. Update docs-progress.md after completion
4. Check off the checkbox in this plan

### Code Validation (Critical!)

**Before updating any documentation, you MUST validate against the actual code.** The documentation site is at `/data/jbutler/git/mule-ai/muleai.io` but the source code is at `/data/jbutler/git/mule-ai/mule`.

#### Key Code Locations to Explore:

**For Getting Started & Installation:**
- `cmd/api/` - Main API entry point and server setup
- `docker-compose.yml` - Docker configuration
- `Dockerfile` - Container build process
- `Makefile` - Build targets
- `go.mod` - Go version and dependencies

**For Agents Documentation:**
- `internal/agent/` - Agent runtime (runtime.go, pirc/ for pi integration)
- `internal/primitive/primitive.go` - Core primitive types including Agent
- `cmd/api/handlers.go` - API endpoints for agent CRUD operations
- Check `Agent` struct for all available fields and capabilities

**For Workflows Documentation:**
- `internal/engine/engine.go` - Workflow execution engine
- `internal/manager/workflow.go` - Workflow management
- `internal/primitive/primitive.go` - Workflow and WorkflowStep types
- Check how steps are executed and what step types exist

**For Settings/Configuration:**
- `internal/database/store_pg.go` - Database schema and configuration storage
- `cmd/api/handlers.go` - API endpoints (search for "setting" or "config")
- `config.toml` in mule project - Configuration file format
- Check what environment variables are supported

**For Repository Operations:**
- `internal/` directory for repository-related code
- `cmd/` for CLI or API handlers
- Look for repository sync logic, label handling, branch naming

**For AI Providers:**
- `internal/provider/` - Provider implementations
- `internal/primitive/primitive.go` - Provider struct definition
- Check supported provider types and configuration

**For Advanced Features (Multi-agent, RAG, WASM):**
- `internal/agent/pirc/` - PI RPC integration for agent execution
- `internal/engine/wasm.go` - WASM module execution
- Look for RAG-related code (search for "rag", "vector", "embedding")
- `examples/wasm/` - WASM module examples

#### Validation Steps:
1. Read the documentation page you plan to update
2. Navigate to the relevant code location above
3. Verify what the documentation says matches what the code actually does
4. If they differ, update the documentation to match the code
5. Check for features mentioned in docs that don't exist in code (remove if inaccurate)
6. Check for features in code that aren't documented (add if important)

### Agent Guidelines:
- You are "Mule" - a software agent focused on AI development and Golang
- Follow the project's documentation standards
- Keep changes purely additive - don't remove existing content without good reason
- Maintain consistent tone and style throughout
- Focus on clarity and usefulness for readers
- **ALWAYS validate documentation against source code before making changes**

### Documentation Standards:
- Use Markdown with proper heading hierarchy (h1 → h2 → h3)
- Include frontmatter on all pages:
  ```yaml
  title: "Page Title"
  description: "Brief description"
  date: YYYY-MM-DD
  ```
- Add code blocks with language specification
- Use relative links where possible
- Include examples for complex features

### Improvement Priorities:
1. **High Impact:** New feature documentation, missing configuration details
2. **Medium Impact:** Better examples, clearer explanations
3. **Low Impact:** Formatting improvements, link fixes
