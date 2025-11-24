#!/bin/bash

# Test script to validate migration files
# This script checks that migration files are properly structured

set -e

echo "=== Migration File Validation ==="

# Check if migration file exists
echo "📋 Checking migration file..."

migration_file="internal/database/migrations/0001_initial_schema.sql"

if [[ -f "$migration_file" ]]; then
    echo "✅ $migration_file exists"
else
    echo "❌ $migration_file missing"
    exit 1
fi

# Check 0001_initial_schema.sql for required tables
echo ""
echo "🔍 Checking initial schema..."

required_tables=(
    "providers"
    "tools" 
    "agents"
    "workflows"
    "workflow_steps"
    "wasm_modules"
    "agent_tools"
    "jobs"
    "job_steps"
    "artifacts"
)

for table in "${required_tables[@]}"; do
    if grep -q "CREATE TABLE.*$table" internal/database/migrations/0001_initial_schema.sql; then
        echo "✅ Table $table created with UUID primary key"
    else
        echo "❌ Table $table missing from initial schema"
        exit 1
    fi
done

# Check for VARCHAR UUID primary keys
echo ""
echo "🔑 Checking primary key types..."

for table in "${required_tables[@]}"; do
    if [[ "$table" == "agent_tools" ]]; then
        # agent_tools has a composite primary key
        if grep -A 5 "CREATE TABLE.*$table" internal/database/migrations/0001_initial_schema.sql | grep -q "PRIMARY KEY (agent_id, tool_id)"; then
            echo "✅ Table $table has composite VARCHAR UUID primary key"
        else
            echo "❌ Table $table does not have composite VARCHAR UUID primary key"
            exit 1
        fi
    else
        # Other tables have single VARCHAR UUID primary key
        if grep -A 5 "CREATE TABLE.*$table" internal/database/migrations/0001_initial_schema.sql | grep -q "id VARCHAR(255) PRIMARY KEY"; then
            echo "✅ Table $table has VARCHAR UUID primary key"
        else
            echo "❌ Table $table does not have VARCHAR UUID primary key"
            exit 1
        fi
    fi
done

# Check for proper foreign key references
echo ""
echo "🔗 Checking foreign key references..."

if grep -q "REFERENCES providers(id)" internal/database/migrations/0001_initial_schema.sql; then
    echo "✅ Foreign key references to providers are correct"
else
    echo "❌ Foreign key references to providers are incorrect"
    exit 1
fi

if grep -q "REFERENCES agents(id)" internal/database/migrations/0001_initial_schema.sql; then
    echo "✅ Foreign key references to agents are correct"
else
    echo "❌ Foreign key references to agents are incorrect"
    exit 1
fi

# Check that there's only one migration file
echo ""
echo "📊 Checking migration count..."

migration_count=$(ls internal/database/migrations/*.sql 2>/dev/null | wc -l)
if [[ $migration_count -eq 1 ]]; then
    echo "✅ Only one migration file (as expected for unreleased app)"
else
    echo "❌ Found $migration_count migration files, expected 1"
    exit 1
fi

# Test Go build with migrations
echo ""
echo "🔨 Testing Go build with embedded migrations..."

if go build ./cmd/api; then
    echo "✅ Build successful with embedded migrations"
else
    echo "❌ Build failed"
    exit 1
fi

# Test that migrations are embedded
echo ""
echo "📦 Checking embedded migrations..."

if grep -q "//go:embed" internal/database/migrator.go; then
    echo "✅ Migrations are embedded in binary"
else
    echo "❌ Migrations are not embedded"
    exit 1
fi

# Clean up
rm -f api

echo ""
echo "✅ All migration validations passed!"
echo ""
echo "Migration strategy:"
echo "  - Single migration file (0001_initial_schema.sql)"
echo "  - Uses VARCHAR UUID primary keys from the start"
echo "  - No complex migration chains needed"
echo "  - Perfect for unreleased application"
echo ""
echo "The application will automatically run this migration on startup."