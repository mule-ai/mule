-- Migration: Add query optimization indexes
-- Date: 2026-03-21
-- Reason: Improve performance for frequently-used list queries

-- Composite index for ListWorkflowSteps query:
-- SELECT ... FROM workflow_steps WHERE workflow_id = $1 ORDER BY step_order
-- This index covers both the WHERE clause and the ORDER BY clause
CREATE INDEX IF NOT EXISTS idx_workflow_steps_workflow_id_order ON workflow_steps(workflow_id, step_order);

-- Composite index for ListJobSteps query:
-- SELECT ... FROM job_steps WHERE job_id = $1 ORDER BY created_at
-- This index covers both the WHERE clause and the ORDER BY clause
CREATE INDEX IF NOT EXISTS idx_job_steps_job_id_created_at ON job_steps(job_id, created_at);

-- Index for ListAgents ordering by created_at DESC:
-- SELECT ... FROM agents ORDER BY created_at DESC
CREATE INDEX IF NOT EXISTS idx_agents_created_at_desc ON agents(created_at DESC);

-- Index for ListWorkflows ordering by created_at DESC:
-- SELECT ... FROM workflows ORDER BY created_at DESC
CREATE INDEX IF NOT EXISTS idx_workflows_created_at_desc ON workflows(created_at DESC);
