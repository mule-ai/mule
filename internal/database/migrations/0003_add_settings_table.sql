-- Settings table for storing application configuration
CREATE TABLE IF NOT EXISTS settings (
    id VARCHAR(255) PRIMARY KEY,
    key TEXT NOT NULL UNIQUE,
    value TEXT NOT NULL,
    description TEXT,
    category TEXT NOT NULL DEFAULT 'general',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Insert default workflow timeout setting (5 minutes in seconds)
INSERT INTO settings (id, key, value, description, category)
VALUES ('timeout_workflow_seconds', 'timeout_workflow_seconds', '300', 'Workflow execution timeout in seconds', 'api')
ON CONFLICT (key) DO NOTHING;

-- Insert default request timeout setting (6 minutes in seconds)
-- This is slightly longer than the workflow timeout to prevent race conditions
INSERT INTO settings (id, key, value, description, category)
VALUES ('timeout_request_seconds', 'timeout_request_seconds', '360', 'Request timeout in seconds for API calls', 'api')
ON CONFLICT (key) DO NOTHING;

-- Insert default max tool calls setting (10 iterations)
INSERT INTO settings (id, key, value, description, category)
VALUES ('max_tool_calls', 'max_tool_calls', '10', 'Maximum number of tool calls allowed per agent execution', 'agent')
ON CONFLICT (key) DO NOTHING;

-- Index for quick lookup by key
CREATE INDEX IF NOT EXISTS idx_settings_key ON settings(key);
CREATE INDEX IF NOT EXISTS idx_settings_category ON settings(category);