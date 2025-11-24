-- Add error_message column to jobs table
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS error_message TEXT;