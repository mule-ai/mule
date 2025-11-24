# Database Migrations

This document describes the database migration system for Mule v2.

## Overview

The migration system handles schema evolution and ensures that the database structure matches the Go models. Since this is an unreleased application, we use a **single migration approach** with SQL files that are **embedded directly in the binary** using Go's embed package.

## Key Benefits

- **No External Dependencies**: Migrations are embedded in the binary, no need to ship migration files separately
- **Working Directory Independent**: Works regardless of where the application is run
- **Docker-Friendly**: Perfect for containerized deployments
- **Simple & Clean**: Single migration file since the app hasn't been released
- **Version Controlled**: Migration versions are tracked in the database

## Migration File

There is a single migration file stored in `internal/database/migrations/`:

```
0001_initial_schema.sql
```

This migration creates the complete schema with VARCHAR UUID primary keys that match the Go models from the start.

### Schema Details

The initial schema creates all required tables:

1. **providers** - AI provider configurations
2. **tools** - Available tools for agents  
3. **agents** - AI agent definitions
4. **workflows** - Workflow definitions
5. **workflow_steps** - Individual workflow steps
6. **wasm_modules** - WASM module storage
7. **agent_tools** - Many-to-many agent-tool relationships
8. **jobs** - Job execution records
9. **job_steps** - Individual job step executions
10. **artifacts** - Generated artifacts and outputs

All tables use VARCHAR(255) UUID primary keys to match the Go model expectations.

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
2. Use the next sequential number in the filename
3. Write your SQL migration
4. Test the migration
5. Rebuild the application to embed the new migration

### Migration Guidelines

- **Idempotent**: Migrations should be safe to run multiple times
- **Backwards Compatible**: Consider existing data when making changes
- **Transactional**: Keep related changes in a single migration
- **Tested**: Test migrations on both empty and populated databases

### Example Migration

```sql
-- 0003_add_user_preferences.sql

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
   - See migration 0002 for type conversion examples

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

## Schema Changes

### From SERIAL to UUID

Migration 0002 handles the conversion from SERIAL (INTEGER) to VARCHAR UUID primary keys:

1. Adds new UUID columns
2. Populates them with generated values
3. Drops old constraints
4. Creates new constraints
5. Cleans up old columns

This ensures data integrity during the transition.

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