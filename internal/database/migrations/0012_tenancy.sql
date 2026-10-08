-- Migration: 0012_tenancy.sql
-- Tenancy parity (Herd H3.1): users, API keys, and owner_id scoping on
-- the primitive tables.
--
-- Mirrors forge's 002_users_and_api_keys.sql conventions, adapted to
-- mule's VARCHAR(255) id style (mule generates ids in Go, so no
-- gen_random_uuid() defaults).

-- Users table
CREATE TABLE IF NOT EXISTS users (
    id              VARCHAR(255) PRIMARY KEY,
    email           TEXT NOT NULL UNIQUE,
    name            TEXT,
    password_hash   TEXT NOT NULL,
    role            TEXT NOT NULL DEFAULT 'user',  -- 'admin' | 'user'
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT users_role_check CHECK (role IN ('user', 'admin'))
);

-- API Keys table
CREATE TABLE IF NOT EXISTS api_keys (
    id              VARCHAR(255) PRIMARY KEY,
    user_id         VARCHAR(255) NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key_hash        TEXT NOT NULL UNIQUE,  -- SHA-256 hex of the key body
    key_prefix      TEXT NOT NULL,         -- first 12 chars, for identification
    restricted      BOOLEAN NOT NULL DEFAULT FALSE,  -- no agent/workflow/job write
    expires_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- owner_id on every owner-scoped primitive table.
-- No foreign key to users: the backfill below stamps rows with the
-- well-known default bootstrap admin id, and that users row does not
-- exist yet at migration time (the Go bootstrap creates it on first
-- boot, and may override the id via MULE_ADMIN_ID).
ALTER TABLE agents    ADD COLUMN IF NOT EXISTS owner_id VARCHAR(255);
ALTER TABLE workflows ADD COLUMN IF NOT EXISTS owner_id VARCHAR(255);
ALTER TABLE jobs      ADD COLUMN IF NOT EXISTS owner_id VARCHAR(255);
ALTER TABLE skills    ADD COLUMN IF NOT EXISTS owner_id VARCHAR(255);
ALTER TABLE providers ADD COLUMN IF NOT EXISTS owner_id VARCHAR(255);

-- Backfill pre-tenancy rows to the default bootstrap admin
-- (00000000-0000-4000-8000-000000000001, the constant the Go
-- bootstrap uses when MULE_ADMIN_ID is unset) so existing
-- single-operator data stays reachable by the admin key. After
-- cutover, rows with NULL owner_id are admin-only.
UPDATE agents    SET owner_id = '00000000-0000-4000-8000-000000000001' WHERE owner_id IS NULL;
UPDATE workflows SET owner_id = '00000000-0000-4000-8000-000000000001' WHERE owner_id IS NULL;
UPDATE jobs      SET owner_id = '00000000-0000-4000-8000-000000000001' WHERE owner_id IS NULL;
UPDATE skills    SET owner_id = '00000000-0000-4000-8000-000000000001' WHERE owner_id IS NULL;
UPDATE providers SET owner_id = '00000000-0000-4000-8000-000000000001' WHERE owner_id IS NULL;

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id);
CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_agents_owner_id ON agents(owner_id);
CREATE INDEX IF NOT EXISTS idx_workflows_owner_id ON workflows(owner_id);
CREATE INDEX IF NOT EXISTS idx_jobs_owner_id ON jobs(owner_id);
CREATE INDEX IF NOT EXISTS idx_skills_owner_id ON skills(owner_id);
CREATE INDEX IF NOT EXISTS idx_providers_owner_id ON providers(owner_id);
