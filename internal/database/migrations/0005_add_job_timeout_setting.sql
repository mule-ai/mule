-- Add job timeout setting
INSERT INTO settings (id, key, value, description, category)
VALUES ('timeout_job_seconds', 'timeout_job_seconds', '3600', 'Job execution timeout in seconds (default 1 hour)', 'engine')
ON CONFLICT (key) DO NOTHING;