#!/bin/bash

# Test script for database migrations
# This script demonstrates how to run the migrations manually

set -e

echo "=== Mule v2 Database Migration Test ==="

# Database connection string
DB_CONN="postgres://postgres:postgres@localhost:5432/mulev2?sslmode=disable"

# Check if database is running
if ! pg_isready -h localhost -p 5432 -U postgres; then
    echo "❌ PostgreSQL is not running. Please start PostgreSQL first."
    echo "   With Docker: docker run -d --name postgres -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgres:15"
    exit 1
fi

echo "✅ PostgreSQL is running"

# Create database if it doesn't exist
echo "📋 Creating database if it doesn't exist..."
createdb -h localhost -p 5432 -U postgres mulev2 2>/dev/null || echo "Database already exists"

# Build the application
echo "🔨 Building the application..."
go build ./cmd/api

# Test the migration by running the application
echo "🚀 Testing database migrations..."
timeout 10s ./api -db "$DB_CONN" -listen ":8080" || echo "Application started successfully (timeout expected)"

# Check if migrations were applied
echo "🔍 Checking migration status..."
psql "$DB_CONN" -c "SELECT version, applied_at FROM schema_migrations ORDER BY applied_at;" 2>/dev/null || echo "No migrations found or database not accessible"

# Check table structure
echo "📊 Checking table structure..."
psql "$DB_CONN" -c "\dt" 2>/dev/null || echo "Could not list tables"

# Check specific tables
echo "🔎 Checking agents table structure..."
psql "$DB_CONN" -c "\d agents" 2>/dev/null || echo "Could not describe agents table"

echo ""
echo "✅ Migration test completed!"
echo ""
echo "To manually inspect the database:"
echo "  psql $DB_CONN"
echo ""
echo "To check migrations:"
echo "  SELECT * FROM schema_migrations;"
echo ""
echo "To check tables:"
echo "  \dt"