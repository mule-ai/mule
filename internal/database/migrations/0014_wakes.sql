-- 0014_wakes.sql — Herd H3.3: fleet-level wake-policy engine.
--
-- A "wake" is a proactivity primitive: a condition (cron schedule,
-- webhook, mule/external event, job status, ...) plus a target (a forge
-- agent conversation or a mule workflow) plus a prompt template. The
-- scheduler in internal/wake fires due wakes: it renders
-- prompt_template over the fire payload and either submits a mule job
-- (target.workflow_id) or posts a message to a forge conversation
-- (target.forge_conversation, H3.2 shape).
--
-- wake_log is the fire audit trail (throttles, failures, and the
-- rate_limit_per_hour guardrail are all visible here).

CREATE TABLE wakes (
    id              VARCHAR(255) PRIMARY KEY,
    owner_id        VARCHAR(255) NOT NULL,
    name            TEXT NOT NULL,
    kind            TEXT NOT NULL CHECK (kind IN ('cron','webhook','event','job','memory','agent_signal')),
    spec            JSONB NOT NULL,      -- per-kind, see internal/wake/types.go
    target          JSONB NOT NULL,      -- {forge_conversation: {forge_url, api_key, agent_id|conversation_id}} or {workflow_id}
    prompt_template TEXT NOT NULL,       -- Go text/template rendered with the fire payload
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    rate_limit_per_hour INT NOT NULL DEFAULT 24,  -- runaway guardrail (research §10.3)
    last_fired_at   TIMESTAMPTZ,
    next_due_at     TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE wake_log (
    wake_id VARCHAR(255) REFERENCES wakes(id) ON DELETE CASCADE,
    fired_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    payload JSONB, job_id VARCHAR(255), ok BOOLEAN, error TEXT
);

-- Scheduler scan lane: enabled wakes whose cron next_due_at has passed.
CREATE INDEX idx_wakes_due ON wakes (owner_id, enabled, next_due_at);
