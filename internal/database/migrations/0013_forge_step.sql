-- Migration 0013: add the forge_conversation step type (Herd H3.2).
--
-- The step's full config (forge_url, api_key, agent_id, conversation_id,
-- mode, content, wait, timeout_secs) lives in the existing `config`
-- JSONB column -- no new columns. Only the step_type CHECK constraint
-- widens to admit the new type.
--
-- The constraint was created by 0001 as a table column CHECK, so
-- Postgres named it `workflow_steps_step_type_check`.

ALTER TABLE workflow_steps DROP CONSTRAINT IF EXISTS workflow_steps_step_type_check;
ALTER TABLE workflow_steps ADD CONSTRAINT workflow_steps_step_type_check
    CHECK (step_type IN ('agent', 'wasm_module', 'forge_conversation'));
