package database

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

// DB represents the database connection
type DB struct {
	*sql.DB
}

// Config holds database configuration
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// NewDB creates a new database connection
func NewDB(config Config) (*DB, error) {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.User, config.Password, config.DBName, config.SSLMode)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{db}, nil
}

// InitSchema initializes the database schema
func (db *DB) InitSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS providers (
		id VARCHAR(255) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		api_base_url TEXT NOT NULL,
		api_key_encrypted TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS tools (
		id VARCHAR(255) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		description TEXT,
		type VARCHAR(100) NOT NULL,
		config JSONB,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS agents (
		id VARCHAR(255) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		description TEXT,
		provider_id VARCHAR(255) REFERENCES providers(id),
		model_id VARCHAR(255) NOT NULL,
		system_prompt TEXT,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS agent_tools (
		agent_id VARCHAR(255) REFERENCES agents(id) ON DELETE CASCADE,
		tool_id VARCHAR(255) REFERENCES tools(id) ON DELETE CASCADE,
		PRIMARY KEY (agent_id, tool_id)
	);

	CREATE TABLE IF NOT EXISTS workflows (
		id VARCHAR(255) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		description TEXT,
		is_async BOOLEAN NOT NULL DEFAULT FALSE,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS workflow_steps (
		id VARCHAR(255) PRIMARY KEY,
		workflow_id VARCHAR(255) REFERENCES workflows(id) ON DELETE CASCADE,
		step_order INTEGER NOT NULL,
		type VARCHAR(20) NOT NULL CHECK (type IN ('AGENT', 'WASM')),
		agent_id VARCHAR(255) REFERENCES agents(id) ON DELETE SET NULL,
		wasm_module_id VARCHAR(255) REFERENCES wasm_modules(id) ON DELETE SET NULL,
		config JSONB,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS wasm_modules (
		id VARCHAR(255) PRIMARY KEY,
		name VARCHAR(255) NOT NULL,
		description TEXT,
		module_data BYTEA NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS jobs (
		id VARCHAR(255) PRIMARY KEY,
		workflow_id VARCHAR(255) REFERENCES workflows(id) ON DELETE CASCADE,
		wasm_module_id VARCHAR(255) REFERENCES wasm_modules(id) ON DELETE SET NULL,
		status VARCHAR(20) NOT NULL CHECK (status IN ('QUEUED', 'RUNNING', 'COMPLETED', 'FAILED')),
		input_data JSONB,
		output_data JSONB,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		started_at TIMESTAMP,
		completed_at TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS job_steps (
		id VARCHAR(255) PRIMARY KEY,
		job_id VARCHAR(255) REFERENCES jobs(id) ON DELETE CASCADE,
		workflow_step_id VARCHAR(255) REFERENCES workflow_steps(id) ON DELETE CASCADE,
		status VARCHAR(20) NOT NULL CHECK (status IN ('PENDING', 'RUNNING', 'COMPLETED', 'FAILED')),
		input_data JSONB,
		output_data JSONB,
		started_at TIMESTAMP,
		completed_at TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS artifacts (
		id VARCHAR(255) PRIMARY KEY,
		job_id VARCHAR(255) REFERENCES jobs(id) ON DELETE CASCADE,
		name VARCHAR(255) NOT NULL,
		mime_type VARCHAR(100) NOT NULL,
		data BYTEA NOT NULL,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	-- WASM Module Sources table for storing source code
	CREATE TABLE IF NOT EXISTS wasm_module_sources (
		id VARCHAR(255) PRIMARY KEY,
		wasm_module_id VARCHAR(255) NOT NULL REFERENCES wasm_modules(id) ON DELETE CASCADE,
		language TEXT NOT NULL CHECK (language IN ('go', 'rust', 'javascript', 'python')),
		source_code TEXT NOT NULL,
		version INTEGER NOT NULL DEFAULT 1,
		compilation_status TEXT NOT NULL CHECK (compilation_status IN ('pending', 'compiling', 'success', 'failed')) DEFAULT 'pending',
		compilation_error TEXT,
		compiled_at TIMESTAMP,
		created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);

	-- Indexes for better performance
	CREATE INDEX IF NOT EXISTS idx_agents_provider_id ON agents(provider_id);
	CREATE INDEX IF NOT EXISTS idx_workflow_steps_workflow_id ON workflow_steps(workflow_id);
	CREATE INDEX IF NOT EXISTS idx_jobs_workflow_id ON jobs(workflow_id);
	CREATE INDEX IF NOT EXISTS idx_jobs_wasm_module_id ON jobs(wasm_module_id);
	CREATE INDEX IF NOT EXISTS idx_job_steps_job_id ON job_steps(job_id);
	CREATE INDEX IF NOT EXISTS idx_artifacts_job_id ON artifacts(job_id);
	CREATE INDEX IF NOT EXISTS idx_wasm_module_sources_wasm_module_id ON wasm_module_sources(wasm_module_id);
	CREATE INDEX IF NOT EXISTS idx_wasm_module_sources_language ON wasm_module_sources(language);
	CREATE INDEX IF NOT EXISTS idx_wasm_module_sources_status ON wasm_module_sources(compilation_status);
	`

	_, err := db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	log.Println("Database schema initialized successfully")
	return nil
}
