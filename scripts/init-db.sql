-- Database initialization script for Mule
-- This script is automatically executed when the PostgreSQL container starts

-- Create the initial schema
\i /docker-entrypoint-initdb.d/0001_initial_schema.sql