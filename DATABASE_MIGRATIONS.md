# Database Migrations

This document describes the database migration system for Mule v2.

## Overview

The migration system handles schema evolution and ensures that the database structure matches the Go models. Migrations are **embedded directly in the binary** using Go's embed package.

## Key Benefits

- **No External Dependencies**: Migrations are embedded in the binary, no need to ship migration files separately
- **Working Directory Independent**: Works regardless of where the application is run
- **Docker-Friendly**: Perfect for containerized deployments
- **Single Source of Truth**: Migration versions tracked in database via `schema_migrations` table
- **Version Controlled**: Migration versions are tracked in the database

## Migration Files

Migration files are stored in `internal/database/migrations/` and executed in order:

| Migration | Purpose |
|-----------|---------|
| `0001_initial_schema.sql` | Creates the complete initial schema with all core tables |
| `0002_add_error_message_to_jobs.sql` | Adds `error_message` column to jobs table |
| `0002_add_memory_config.sql` | Adds memory vector search configuration table |
| `0002_add_wasm_source_code.sql` | Adds `source_code` column to wasm_modules table |
| `0003_add_settings_table.sql` | Creates application settings table |
| `0004_add_max_tool_calls_setting.sql` | Adds max_tool_calls setting |
| `0005_add_job_timeout_setting.sql` | Adds job_timeout setting |
| `0006_add_wasm_module_config.sql` | Adds `config` column to wasm_modules table |
| `0007_add_working_directory_to_jobs.sql` | Adds `working_directory` column to jobs table |
| `0008_add_skills_table.sql` | Creates skills system (skills, agent_skills tables, pi_config on agents) |
| `0009_optimize_job_queries.sql` | Adds indexes for job query performance |
| `0010_add_query_optimization_indexes.sql` | Adds composite indexes for workflow_steps, job_steps, agents, and workflows queries |

## Schema Details

### Core Tables

1. **providers** - AI provider configurations
   - API base URL, encrypted API key
   - Supports OpenAI-compatible APIs (Anthropic, OpenAI, Google, etc.)

2. **tools** - Available tools for agents
   - Tool definitions and metadata
   - Many-to-many relationship with agents via `agent_tools`

3. **agents** - AI agent definitions
   - References provider, system prompt, model ID
   - pi_config JSONB for pi-specific configuration (thinking level, skills, tools, extensions)
   - Many-to-many relationship with tools via `agent_tools`
   - Many-to-many relationship with skills via `agent_skills`

4. **workflows** - Workflow definitions
   - Ordered sequences of workflow steps
   - Supports async execution mode

5. **workflow_steps** - Individual workflow steps
   - Two types: "agent" (invokes agent) or "wasm_module" (executes WASM)
   - Ordered by `step_order` within a workflow

6. **wasm_modules** - WASM module storage
   - Binary module_data stored in database
   - Optional source_code for reference
   - Optional config JSONB for configuration

7. **agent_tools** - Many-to-many agent-tool relationships
   - Junction table linking agents to tools

8. **jobs** - Job execution records
   - Tracks workflow/agent/WASM execution
   - Status: queued, running, completed, failed, cancelled
   - Stores input/output/error data

9. **job_steps** - Individual job step executions
   - Tracks each step within a job
   - Status: queued, running, completed, failed

10. **artifacts** - Generated artifacts and outputs
    - Stores binary data from job steps

### Skills System Tables (Migration 0008)

11. **skills** - Pi agent skills
    - Skill name, description, path, enabled status
    - Skills provide extensibility (file operations, grep, find, bash, etc.)

12. **agent_skills** - Many-to-many agent-skill relationships
    - Junction table linking agents to skills

### Configuration Tables

13. **settings** - Application settings (Migration 0003)
    - Key-value store for configuration
    - Includes: max_tool_calls, job_timeout, memory_config

14. **memory_config** - Memory vector search configuration (Migration 0002)
    - Stores memory/semantic search settings

### Internal Table

15. **schema_migrations** - Migration tracking
    - Tracks which migrations have been applied
    - Created automatically by the migrator

### Indexes (Migration 0010)

Migration 0010 adds composite indexes to optimize common query patterns:

| Index | Table | Purpose |
|-------|-------|---------|
| `idx_workflow_steps_workflow_id_order` | workflow_steps | Optimizes `WHERE workflow_id = X ORDER BY step_order` |
| `idx_job_steps_job_id_created_at` | job_steps | Optimizes `WHERE job_id = X ORDER BY created_at` |
| `idx_agents_created_at_desc` | agents | Optimizes `ORDER BY created_at DESC` |
| `idx_workflows_created_at_desc` | workflows | Optimizes `ORDER BY created_at DESC` |

## Migration System

### Components

- **Migrator** (`internal/database/migrator.go`) - Handles running migrations
- **Embedded Filesystem** - Uses `//go:embed` to embed migration files in the binary
- **Schema Tracking** - Uses `schema_migrations` table to track applied migrations
- **Transaction Safety** - Each migration runs in a transaction

### How It Works

1. Creates `schema_migrations` table if it doesn't exist
2. Reads migration files from the embedded filesystem
3. Sorts files by name to ensure correct order
4. For each migration:
   - Checks if it has already been applied
   - If not applied, runs the migration in a transaction
   - Records the migration as applied

## Running Migrations

### Automatic (Default)

Migrations run automatically when the application starts:

```bash
./api -db "postgres://user:pass@localhost:5432/mulev2?sslmode=disable"
```

The migrations are embedded in the binary, so no external files are needed.

### Manual Testing

You can test migrations using the provided script:

```bash
./scripts/test_migrations.sh
```

## Migration Development

### Creating a New Migration

1. Create a new SQL file in `internal/database/migrations/`
2. Use the next sequential number in the filename (e.g., `0010_...`)
3. Write your SQL migration
4. Test the migration
5. Rebuild the application to embed the new migration

### Migration Guidelines

- **Idempotent**: Migrations should be safe to run multiple times (use `IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`)
- **Backwards Compatible**: Consider existing data when making changes
- **Transactional**: Keep related changes in a single migration
- **Tested**: Test migrations on both empty and populated databases

### Example Migration

```sql
-- 0010_add_user_preferences.sql

-- Add new column
ALTER TABLE users ADD COLUMN IF NOT EXISTS preferences JSONB DEFAULT '{}';

-- Update existing records
UPDATE users SET preferences = '{"theme": "light"}' WHERE preferences IS NULL;

-- Create index
CREATE INDEX IF NOT EXISTS idx_users_preferences ON users USING GIN(preferences);
```

After adding the migration file, rebuild the application:

```bash
go build ./cmd/api
```

The new migration will be automatically embedded and available.

## Troubleshooting

### Common Issues

1. **Foreign Key Constraint Errors**
   - Usually caused by type mismatches
   - Check that referenced columns have the same data type

2. **Migration Already Applied**
   - Check `schema_migrations` table
   - Remove entry if needed: `DELETE FROM schema_migrations WHERE version = 'XXXX_description.sql'`

3. **Transaction Rollback**
   - Check migration SQL for syntax errors
   - Ensure all statements are valid PostgreSQL

4. **Migration Not Found**
   - Ensure the migration file is in `internal/database/migrations/`
   - Check the filename follows the naming convention
   - Rebuild the application to embed new migrations

### Manual Inspection

```sql
-- Check applied migrations
SELECT version, applied_at FROM schema_migrations ORDER BY applied_at;

-- Check table structure
\d table_name;

-- Check all tables
\dt;

-- Check foreign key constraints
SELECT
    tc.constraint_name,
    tc.table_name,
    kcu.column_name,
    ccu.table_name AS foreign_table_name,
    ccu.column_name AS foreign_column_name
FROM information_schema.table_constraints AS tc
JOIN information_schema.key_column_usage AS kcu
    ON tc.constraint_name = kcu.constraint_name
    AND tc.table_schema = kcu.table_schema
JOIN information_schema.constraint_column_usage AS ccu
    ON ccu.constraint_name = tc.constraint_name
    AND ccu.table_schema = tc.table_schema
WHERE tc.constraint_type = 'FOREIGN KEY';
```

## Docker Deployment

The embedded migration system is perfect for Docker deployments:

```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY . .
RUN go build ./cmd/api

FROM postgres:16-alpine
# ... postgres setup ...
COPY --from=builder /app/mule /usr/local/bin/mule
# No need to copy migration files - they're embedded!
```

### Docker Compose

The `docker-compose.yml` file is configured to:

1. **PostgreSQL Service**: Runs PostgreSQL without any initialization scripts
2. **Mule Service**: Runs the application which automatically handles migrations
3. **Health Checks**: Ensures PostgreSQL is ready before starting the application

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: mulev2
      POSTGRES_USER: mule
      POSTGRES_PASSWORD: mule
    # No migration mounts needed - handled by application!

  mule:
    build: .
    depends_on:
      postgres:
        condition: service_healthy
    # Application will run migrations automatically on startup
```

### Starting with Docker

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f mule

# Stop services
docker-compose down
```

The application will automatically:
1. Wait for PostgreSQL to be ready
2. Connect to the database
3. Run all pending migrations
4. Start the API server

### Binary Distribution

When distributing the binary, no additional files are needed:

```bash
# Just the binary is enough
./api -db "postgres://...:5432/mulev2?sslmode=disable"
```

## Testing

### Unit Tests

```bash
go test ./internal/database -v
```

### Integration Tests

```bash
# Requires running PostgreSQL
./scripts/test_migrations.sh
```

### Test Database Setup

```bash
# Start PostgreSQL with Docker
docker run -d --name postgres \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=mulev2 \
  -p 5432:5432 \
  postgres:15

# Run tests
go test ./internal/database -v

# Clean up
docker stop postgres && docker rm postgres
```

## Architecture

### Embed Directive

```go
//go:embed migrations/*.sql
var migrationFS embed.FS
```

This directive compiles all SQL files into the binary, making them available via the `migrationFS` filesystem.

### Migration Execution

```go
func (m *Migrator) RunMigrations() error {
    // Read from embedded filesystem
    files, err := fs.ReadDir(migrationFS, "migrations")
    // ... execute migrations
}
```

The system reads from the embedded filesystem rather than the local filesystem, ensuring it works anywhere the binary runs.
