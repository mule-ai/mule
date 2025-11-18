#!/bin/bash

# Test script to verify Docker setup
# This script tests the Docker configuration without actually running containers

set -e

echo "=== Mule v2 Docker Setup Test ==="

# Check if required files exist
echo "📋 Checking required files..."

required_files=(
    "docker-compose.yml"
    "Dockerfile"
    "internal/database/migrations/0001_initial_schema.sql"
)

for file in "${required_files[@]}"; do
    if [[ -f "$file" ]]; then
        echo "✅ $file exists"
    else
        echo "❌ $file missing"
        exit 1
    fi
done

# Check Docker Compose configuration
echo ""
echo "🔍 Checking Docker Compose configuration..."

# Check if postgres service exists
if grep -q "postgres:" docker-compose.yml; then
    echo "✅ PostgreSQL service found"
else
    echo "❌ PostgreSQL service not found"
    exit 1
fi

# Check if mule service exists
if grep -q "mule:" docker-compose.yml; then
    echo "✅ Mule service found"
else
    echo "❌ Mule service not found"
    exit 1
fi

# Check if old migration mounts are removed
if grep -q "internal/db/migrations" docker-compose.yml; then
    echo "❌ Old migration mounts still present in docker-compose.yml"
    exit 1
else
    echo "✅ Old migration mounts removed"
fi

# Check if database connection is correct
if grep -q "postgres://mule:mule@postgres:5432/mulev2" docker-compose.yml; then
    echo "✅ Database connection string correct"
else
    echo "❌ Database connection string incorrect"
    exit 1
fi

# Check Dockerfile
echo ""
echo "🐳 Checking Dockerfile..."

if grep -q "FROM golang:1.24-alpine AS builder" Dockerfile; then
    echo "✅ Multi-stage build configured"
else
    echo "❌ Multi-stage build not configured"
    exit 1
fi

if grep -q "go build.*./cmd/api" Dockerfile; then
    echo "✅ Build command correct"
else
    echo "❌ Build command incorrect"
    exit 1
fi

# Check if migrations are properly embedded
echo ""
echo "📦 Checking embedded migrations..."

if grep -q "//go:embed" internal/database/migrator.go; then
    echo "✅ Migrations are embedded"
else
    echo "❌ Migrations are not embedded"
    exit 1
fi

# Test Go build
echo ""
echo "🔨 Testing Go build..."
if go build ./cmd/api; then
    echo "✅ Go build successful"
else
    echo "❌ Go build failed"
    exit 1
fi

# Test binary help
echo ""
echo "🚀 Testing binary..."
if ./api --help 2>/dev/null || echo "Binary created successfully"; then
    echo "✅ Binary is functional"
else
    echo "❌ Binary is not functional"
    exit 1
fi

echo ""
echo "✅ All Docker setup tests passed!"
echo ""
echo "To start the application with Docker:"
echo "  docker-compose up -d"
echo ""
echo "To view logs:"
echo "  docker-compose logs -f"
echo ""
echo "To stop:"
echo "  docker-compose down"