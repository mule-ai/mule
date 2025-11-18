package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
	_ "github.com/lib/pq"
)

func TestDatabaseSchema(t *testing.T) {
	// Skip if no test database is available
	if testing.Short() {
		t.Skip("Skipping database test in short mode")
	}

	// Use a test database
	config := Config{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "postgres",
		DBName:   "mulev2_test",
		SSLMode:  "disable",
	}

	db, err := NewDB(config)
	if err != nil {
		t.Skipf("Could not connect to test database: %v", err)
	}
	defer db.Close()

	// Test schema initialization
	err = db.InitSchema()
	assert.NoError(t, err, "Schema initialization should not fail")

	// Test that all expected tables exist
	tables := []string{
		"providers",
		"tools",
		"agents",
		"agent_tools",
		"workflows",
		"workflow_steps",
		"wasm_modules",
		"jobs",
		"job_steps",
		"artifacts",
	}

	for _, table := range tables {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT FROM information_schema.tables 
				WHERE table_schema = 'public' 
				AND table_name = $1
			)`, table).Scan(&exists)
		
		assert.NoError(t, err, "Should be able to check table existence for %s", table)
		assert.True(t, exists, "Table %s should exist", table)
	}

	// Test that all expected columns exist in agents table
	agentColumns := []string{
		"id",
		"name", 
		"description",
		"provider_id",
		"model_id",
		"system_prompt",
		"created_at",
		"updated_at",
	}

	for _, column := range agentColumns {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT FROM information_schema.columns 
				WHERE table_schema = 'public' 
				AND table_name = 'agents'
				AND column_name = $1
			)`, column).Scan(&exists)
		
		assert.NoError(t, err, "Should be able to check column existence for agents.%s", column)
		assert.True(t, exists, "Column agents.%s should exist", column)
	}

	// Test that all expected columns exist in tools table
	toolColumns := []string{
		"id",
		"name",
		"description", 
		"type",
		"config",
		"created_at",
		"updated_at",
	}

	for _, column := range toolColumns {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT FROM information_schema.columns 
				WHERE table_schema = 'public' 
				AND table_name = 'tools'
				AND column_name = $1
			)`, column).Scan(&exists)
		
		assert.NoError(t, err, "Should be able to check column existence for tools.%s", column)
		assert.True(t, exists, "Column tools.%s should exist", column)
	}

	// Test that all expected columns exist in workflows table
	workflowColumns := []string{
		"id",
		"name",
		"description",
		"is_async",
		"created_at", 
		"updated_at",
	}

	for _, column := range workflowColumns {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT FROM information_schema.columns 
				WHERE table_schema = 'public' 
				AND table_name = 'workflows'
				AND column_name = $1
			)`, column).Scan(&exists)
		
		assert.NoError(t, err, "Should be able to check column existence for workflows.%s", column)
		assert.True(t, exists, "Column workflows.%s should exist", column)
	}

	// Test foreign key constraints
	// Test that agents.provider_id references providers.id
	var constraintExists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT FROM information_schema.table_constraints tc
			JOIN information_schema.key_column_usage kcu 
				ON tc.constraint_name = kcu.constraint_name
			JOIN information_schema.constraint_column_usage ccu 
				ON ccu.constraint_name = tc.constraint_name
			WHERE tc.constraint_type = 'FOREIGN KEY' 
			AND tc.table_name = 'agents'
			AND kcu.column_name = 'provider_id'
			AND ccu.table_name = 'providers'
			AND ccu.column_name = 'id'
		)`).Scan(&constraintExists)
	
	assert.NoError(t, err, "Should be able to check foreign key constraint")
	assert.True(t, constraintExists, "Foreign key constraint agents.provider_id -> providers.id should exist")

	// Test that workflow_steps.workflow_id references workflows.id
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT FROM information_schema.table_constraints tc
			JOIN information_schema.key_column_usage kcu 
				ON tc.constraint_name = kcu.constraint_name
			JOIN information_schema.constraint_column_usage ccu 
				ON ccu.constraint_name = tc.constraint_name
			WHERE tc.constraint_type = 'FOREIGN KEY' 
			AND tc.table_name = 'workflow_steps'
			AND kcu.column_name = 'workflow_id'
			AND ccu.table_name = 'workflows'
			AND ccu.column_name = 'id'
		)`).Scan(&constraintExists)
	
	assert.NoError(t, err, "Should be able to check foreign key constraint")
	assert.True(t, constraintExists, "Foreign key constraint workflow_steps.workflow_id -> workflows.id should exist")

	// Test that indexes exist
	indexes := []string{
		"idx_agents_provider_id",
		"idx_workflow_steps_workflow_id", 
		"idx_jobs_workflow_id",
		"idx_job_steps_job_id",
		"idx_artifacts_job_id",
	}

	for _, index := range indexes {
		var exists bool
		err := db.QueryRow(`
			SELECT EXISTS (
				SELECT FROM pg_indexes 
				WHERE schemaname = 'public' 
				AND indexname = $1
			)`, index).Scan(&exists)
		
		assert.NoError(t, err, "Should be able to check index existence for %s", index)
		assert.True(t, exists, "Index %s should exist", index)
	}

	// Test check constraints
	// Test that jobs.status has the correct check constraint
	var checkExists bool
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT FROM information_schema.check_constraints cc
			JOIN information_schema.constraint_column_usage ccu 
				ON cc.constraint_name = ccu.constraint_name
			WHERE cc.check_clause LIKE '%QUEUED%' 
			AND ccu.table_name = 'jobs'
			AND ccu.column_name = 'status'
		)`).Scan(&checkExists)
	
	assert.NoError(t, err, "Should be able to check constraint")
	assert.True(t, checkExists, "Check constraint on jobs.status should exist")

	// Test that workflow_steps.type has the correct check constraint
	err = db.QueryRow(`
		SELECT EXISTS (
			SELECT FROM information_schema.check_constraints cc
			JOIN information_schema.constraint_column_usage ccu 
				ON cc.constraint_name = ccu.constraint_name
			WHERE cc.check_clause LIKE '%AGENT%' 
			AND ccu.table_name = 'workflow_steps'
			AND ccu.column_name = 'type'
		)`).Scan(&checkExists)
	
	assert.NoError(t, err, "Should be able to check constraint")
	assert.True(t, checkExists, "Check constraint on workflow_steps.type should exist")
}

func TestDatabaseConnection(t *testing.T) {
	// Test with invalid connection string
	config := Config{
		Host:     "nonexistent",
		Port:     5432,
		User:     "invalid",
		Password: "invalid",
		DBName:   "invalid",
		SSLMode:  "disable",
	}

	_, err := NewDB(config)
	assert.Error(t, err, "Should fail to connect with invalid credentials")
}