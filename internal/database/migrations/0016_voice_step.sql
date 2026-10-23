-- 0016: allow the `voice_call` step type (Herd H6.3).
--
-- H6.3 added the in-process voice-loop engine (internal/voice) as a
-- workflow step type, but the constraint from 0013 still rejects it:
-- a POST /workflows/:id/steps with type "voice_call" violates
-- workflow_steps_step_type_check (500).
--
-- Same pattern as 0013 (drop + re-add named).
ALTER TABLE workflow_steps DROP CONSTRAINT IF EXISTS workflow_steps_step_type_check;
ALTER TABLE workflow_steps ADD CONSTRAINT workflow_steps_step_type_check
    CHECK (step_type IN ('agent', 'wasm_module', 'forge_conversation', 'voice_call'));
