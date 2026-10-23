-- Migration 0011: Add external_ref column to agents table
-- Stores an external system's agent id (e.g. a forge agent UUID) so a
-- mule workflow agent can be addressed cross-system. This is the
-- integration point Herd's forge_conversation step uses
-- (PLAN-HERD H1.5 / H3.2).

-- External agent reference (nullable: most agents stay mule-local)
ALTER TABLE agents ADD COLUMN IF NOT EXISTS external_ref VARCHAR(255);

-- Index for reverse lookups: find mule agents pointing at a given
-- external agent id
CREATE INDEX IF NOT EXISTS idx_agents_external_ref ON agents(external_ref);
