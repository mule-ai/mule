-- Initial database schema for Mule v2 AI workflow platform
-- This creates the complete schema with VARCHAR UUID primary keys matching the Go models

-- Providers table
CREATE TABLE IF NOT EXISTS providers (
    id VARCHAR(255) PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    api_base_url TEXT NOT NULL,
    api_key_encrypted BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Tools table
CREATE TABLE IF NOT EXISTS tools (
    id VARCHAR(255) PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Agents table
CREATE TABLE IF NOT EXISTS agents (
    id VARCHAR(255) PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    provider_id VARCHAR(255) NOT NULL REFERENCES providers(id) ON DELETE CASCADE,
    system_prompt TEXT,
    tools JSONB,
    description TEXT,
    model_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Workflows table
CREATE TABLE IF NOT EXISTS workflows (
    id VARCHAR(255) PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    is_async BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Workflow Steps table
CREATE TABLE IF NOT EXISTS workflow_steps (
    id VARCHAR(255) PRIMARY KEY,
    workflow_id VARCHAR(255) NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    step_order INT NOT NULL,
    step_type TEXT NOT NULL CHECK (step_type IN ('agent', 'wasm_module')),
    config JSONB NOT NULL,
    agent_id VARCHAR(255) REFERENCES agents(id),
    wasm_module_id VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (workflow_id, step_order)
);

-- WASM Modules table
CREATE TABLE IF NOT EXISTS wasm_modules (
    id VARCHAR(255) PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    description TEXT,
    module_data BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Agent-Tools junction table (many-to-many relationship)
CREATE TABLE IF NOT EXISTS agent_tools (
    agent_id VARCHAR(255) NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    tool_id VARCHAR(255) NOT NULL REFERENCES tools(id) ON DELETE CASCADE,
    PRIMARY KEY (agent_id, tool_id)
);

-- Jobs table
CREATE TABLE IF NOT EXISTS jobs (
    id VARCHAR(255) PRIMARY KEY,
    workflow_id VARCHAR(255) REFERENCES workflows(id),
    agent_id VARCHAR(255) REFERENCES agents(id),
    status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled')) DEFAULT 'queued',
    input_data JSONB,
    output_data JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Job Steps table
CREATE TABLE IF NOT EXISTS job_steps (
    id VARCHAR(255) PRIMARY KEY,
    job_id VARCHAR(255) NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    workflow_step_id VARCHAR(255) REFERENCES workflow_steps(id),
    step_order INT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('queued', 'running', 'completed', 'failed')) DEFAULT 'queued',
    input_data JSONB,
    output_data JSONB,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (job_id, step_order)
);

-- Artifacts table
CREATE TABLE IF NOT EXISTS artifacts (
    id VARCHAR(255) PRIMARY KEY,
    job_step_id VARCHAR(255) NOT NULL REFERENCES job_steps(id) ON DELETE CASCADE,
    job_id VARCHAR(255) REFERENCES jobs(id),
    name TEXT NOT NULL,
    data BYTEA NOT NULL,
    mime_type TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_agents_provider_id ON agents(provider_id);
CREATE INDEX IF NOT EXISTS idx_workflow_steps_workflow_id ON workflow_steps(workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_steps_agent_id ON workflow_steps(agent_id);
CREATE INDEX IF NOT EXISTS idx_workflow_steps_wasm_module_id ON workflow_steps(wasm_module_id);
CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
CREATE INDEX IF NOT EXISTS idx_jobs_workflow_id ON jobs(workflow_id);
CREATE INDEX IF NOT EXISTS idx_job_steps_status ON job_steps(status);
CREATE INDEX IF NOT EXISTS idx_job_steps_job_id ON job_steps(job_id);
CREATE INDEX IF NOT EXISTS idx_job_steps_workflow_step_id ON job_steps(workflow_step_id);
CREATE INDEX IF NOT EXISTS idx_artifacts_job_id ON artifacts(job_id);