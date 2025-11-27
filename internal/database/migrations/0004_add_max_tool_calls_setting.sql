-- Add missing max_tool_calls setting
-- This migration adds the max_tool_calls setting that was missing from the initial settings migration

-- Insert default max tool calls setting (10 iterations)
INSERT INTO settings (id, key, value, description, category)
VALUES ('max_tool_calls', 'max_tool_calls', '10', 'Maximum number of tool calls allowed per agent execution', 'agent')
ON CONFLICT (key) DO NOTHING;