-- Add working_directory column to jobs table
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS working_directory TEXT;