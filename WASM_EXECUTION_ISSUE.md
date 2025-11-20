# WASM Module Execution Issue - Investigation Report

## Problem Summary

When attempting to execute a WASM module through the Mule v2 UI, the system fails with the following symptoms:

1. **Frontend receives "undefined" job ID** after calling `POST /api/v1/jobs`
2. **35-second response time** for the job creation endpoint (indicates blocking/timeout)
3. **Subsequent 404 error** when trying to fetch job status with ID "undefined"

## Root Cause Analysis

### System Design Mismatch

The core issue is a **design mismatch** between how the system is architected and how the frontend attempts to use it:

**Designed Architecture:**
- WASM modules are meant to be executed as **workflow steps** within defined workflows
- Workflows orchestrate the execution of steps (agents and/or WASM modules)
- Jobs are created automatically when workflows are executed

**Frontend Implementation:**
- The Dashboard UI attempts to execute WASM modules **directly** by creating jobs
- This bypasses the workflow engine entirely
- The job creation endpoint was not designed for direct WASM execution

### Missing Infrastructure

1. **No job creation endpoint existed** - Added `POST /api/v1/jobs` handler
2. **No direct WASM execution path** - The endpoint creates job records but doesn't execute WASM
3. **Response format mismatch** - Frontend expects `{data: {id: "..."}}` but gets raw job object

## Technical Details

### Log Analysis

```
mule-api  | 2025/11/20 14:35:11 POST /api/v1/jobs 200 35.62s
mule-api  | 2025/11/20 14:35:13 Error: job not found: undefined
mule-api  | 2025/11/20 14:35:13 GET /api/v1/jobs/undefined 404 917.258µs
```

**Key Observations:**
- 35-second response time is abnormal and suggests blocking operation or timeout
- Frontend receives "undefined" job ID, indicating response parsing failure
- Subsequent job status check fails with 404

### Frontend Code Analysis

**File:** `frontend/src/pages/Dashboard.js`

```javascript
const jobResponse = await jobsAPI.create({
  workflow_id: selectedWasmModule,  // WASM module ID, not workflow ID!
  input_data: { input: wasmInput },
});

const jobId = jobResponse.data?.id || jobResponse.data?.job_id || jobResponse.data?.job?.id;
```

**Issues:**
1. Uses `workflow_id` field but passes WASM module ID
2. Expects nested `data` property in response (axios pattern)
3. Falls back to multiple possible field names

### Backend Code Analysis

**Current Implementation:**
- `createJobHandler` creates a job record in PostgreSQL
- Returns raw job object directly
- Does NOT execute WASM module
- Does NOT check if `workflow_id` is actually a WASM module ID

**Missing:**
- WASM module validation
- Direct WASM execution logic
- Proper response format wrapping

## What I've Learned

### 1. System Architecture
- Mule v2 is **workflow-centric** - everything flows through workflows
- WASM modules are **step primitives**, not standalone executables
- The job queue is designed for **workflow execution**, not direct module execution

### 2. Frontend Expectations
- The Dashboard UI was built with the assumption of direct WASM execution
- Uses job creation as a proxy for WASM module execution
- Expects axios-style response format with `data` wrapper

### 3. API Design Gaps
- No endpoint for direct WASM module execution
- Job creation endpoint doesn't validate workflow vs WASM module IDs
- Missing execution path for ad-hoc WASM module runs

### 4. Response Format Issues
- Frontend expects: `{data: {id: "...", ...}}`
- Backend returns: `{id: "...", ...}`
- This mismatch causes the "undefined" job ID

## Potential Solutions

### Option 1: Enhance Job Creation Endpoint
Modify `createJobHandler` to:
1. Detect if `workflow_id` is actually a WASM module ID
2. Execute WASM module directly using `wasmExecutor`
3. Return properly formatted response

**Pros:** Quick fix, leverages existing frontend code
**Cons:** Mixes concerns, bypasses workflow engine

### Option 2: Create Dedicated WASM Execution Endpoint
Add new endpoint: `POST /api/v1/wasm-modules/{id}/execute`

**Pros:** Clean separation of concerns, proper API design
**Cons:** Requires frontend changes

### Option 3: Auto-Generate Workflows
When WASM module execution is requested:
1. Dynamically create a one-step workflow
2. Execute the workflow
3. Return job from workflow execution

**Pros:** Maintains workflow-centric design
**Cons:** More complex implementation

## Recommended Next Steps

1. **Immediate Fix:** Update `createJobHandler` to detect WASM module IDs and execute them directly
2. **Response Format:** Wrap job response in `{data: ...}` format to match frontend expectations
3. **Error Handling:** Add proper error messages and validation
4. **Long-term:** Consider creating a dedicated WASM execution endpoint
5. **Testing:** Verify WASM module execution works end-to-end

## Code Locations

- **Frontend:** `frontend/src/pages/Dashboard.js` (lines 136-196)
- **Backend:** `cmd/api/handlers.go` (lines 653-693)
- **WASM Executor:** `internal/engine/wasm.go`
- **Job Store:** `pkg/job/store_pg.go`
- **API Routes:** `cmd/api/server.go` (lines 166-168)

## Related Issues

This issue is related to the broader workflow execution fixes that were recently implemented:
- Job status constraint violations (fixed)
- Synchronous/asynchronous workflow execution (fixed)
- Agent system prompts not being applied (fixed)
- Workflow step output format (fixed)

The WASM execution issue is the remaining piece needed for full workflow functionality.
