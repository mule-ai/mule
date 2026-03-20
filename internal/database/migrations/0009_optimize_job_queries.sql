-- Migration: Optimize job-related queries with composite indexes
-- Date: 2026-03-20
-- Reason: Improve performance for frequently-used job queries

-- Composite index for GetNextQueuedJob query pattern:
-- SELECT ... FROM jobs WHERE status = 'queued' ORDER BY created_at ASC LIMIT 1
CREATE INDEX IF NOT EXISTS idx_jobs_status_created_at ON jobs(status, created_at);

-- Index for ListJobs pagination: ORDER BY created_at DESC
CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs(created_at DESC);

-- Composite index for ListJobs with status filter
-- This helps when filtering by status and ordering by created_at
CREATE INDEX IF NOT EXISTS idx_jobs_status_created_at_desc ON jobs(status, created_at DESC);
