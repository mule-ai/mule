-- 0015_policies.sql — Herd H3.5: mule policy engine (custom rules +
-- auto-review, the control plane that keeps autonomous action
-- trustworthy; research §2.2 / §7.3). Mule owns the store + evaluator;
-- forge's before_tool hook (H2.5) is the enforcement point; ranch is
-- the approval UI.
--
-- A policy is a named, versioned, owner-scoped bundle of deterministic
-- rules. Forge calls POST /api/v1/policies/evaluate with
-- (agent_id, tool, input) and gets a synchronous verdict
-- {allow|deny|ask, rule_id, reason}.

CREATE TABLE policies (
    id          VARCHAR(255) PRIMARY KEY,
    owner_id    VARCHAR(255) NOT NULL,
    scope       TEXT NOT NULL CHECK (scope IN ('global','agent','org')),
    agent_id    VARCHAR(255),           -- for scope='agent'
    rules       JSONB NOT NULL,         -- array, typed in internal/validation/policy.go
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    version     INT NOT NULL DEFAULT 1, -- bumped on every change; audit
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Who/what changed a policy, and when. Every mutation (create,
-- rules/enabled change, delete, version approval, agent proposal,
-- approved-proposal materialization) writes a row in the same
-- transaction as the mutation. Proposal rows have policy_id NULL
-- until (and unless) an operator approves them.
--
-- policy_id is ON DELETE SET NULL (not CASCADE) so the 'deleted'
-- audit row SURVIVES the policy's deletion as the permanent record
-- that the policy existed and who removed it (policy_id becomes
-- NULL; the row's id + at + actor stay). Proposals (policy_id NULL
-- forever unless approved) are never affected.
CREATE TABLE policy_audit (
    id           VARCHAR(255) PRIMARY KEY,   -- doubles as the proposal id
    policy_id    VARCHAR(255) REFERENCES policies(id) ON DELETE SET NULL,
    actor        VARCHAR(255) NOT NULL,      -- user id, or 'system'
    change       TEXT NOT NULL,              -- 'created' | 'rules changed v1→v2' | 'enabled v2' | 'disabled v3' | 'deleted' | 'approved-version v2' | 'proposal' | 'approved-proposal'
    prev_version INT,
    new_version  INT,
    detail       JSONB,
    at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_policy_audit_policy ON policy_audit(policy_id, at DESC);

-- Memoized decisions (H2.5 semantics): (rule_id, agent_id,
-- action-hash) → verdict. A "yes" on an ask answer, or any
-- deterministic verdict, is recorded here so a retry of the SAME
-- action by the SAME agent does not re-ask / re-evaluate. Lookups are
-- rule-agnostic (all memo rows for agent + action_hash; the
-- most-restrictive verdict wins) so a deny memoed under one rule still
-- covers the action even after rule churn.
CREATE TABLE policy_memo (
    rule_id    VARCHAR(255) NOT NULL,
    agent_id   VARCHAR(255) NOT NULL,
    action_hash TEXT NOT NULL,
    verdict    TEXT NOT NULL CHECK (verdict IN ('allow','deny','ask')),
    reason     TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (rule_id, agent_id, action_hash)
);
CREATE INDEX idx_policy_memo_lookup ON policy_memo (agent_id, action_hash);

CREATE INDEX idx_policies_owner ON policies (owner_id, enabled);
