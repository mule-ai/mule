package database

import (
	"time"
)

// Provider represents an AI provider configuration
type Provider struct {
	ID              string    `json:"id" db:"id"`
	Name            string    `json:"name" db:"name"`
	APIBaseURL      string    `json:"api_base_url" db:"api_base_url"`
	APIKeyEncrypted string    `json:"api_key_encrypted" db:"api_key_encrypted"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time `json:"updated_at" db:"updated_at"`
}

// Tool represents a tool that can be used by agents
type Tool struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	Metadata    []byte    `json:"metadata" db:"metadata"` // JSON metadata
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// Agent represents an AI agent configuration
type Agent struct {
	ID           string    `json:"id" db:"id"`
	Name         string    `json:"name" db:"name"`
	Description  string    `json:"description" db:"description"`
	ProviderID   string    `json:"provider_id" db:"provider_id"`
	ModelID      string    `json:"model_id" db:"model_id"`
	SystemPrompt string    `json:"system_prompt" db:"system_prompt"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

// AgentTool represents the many-to-many relationship between agents and tools
type AgentTool struct {
	AgentID string `json:"agent_id" db:"agent_id"`
	ToolID  string `json:"tool_id" db:"tool_id"`
}

// Workflow represents a workflow definition
type Workflow struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	IsAsync     bool      `json:"is_async" db:"is_async"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// WorkflowStep represents a step in a workflow
type WorkflowStep struct {
	ID           string    `json:"id" db:"id"`
	WorkflowID   string    `json:"workflow_id" db:"workflow_id"`
	StepOrder    int       `json:"step_order" db:"step_order"`
	Type         string    `json:"type" db:"type"` // "AGENT" or "WASM"
	AgentID      *string   `json:"agent_id,omitempty" db:"agent_id"`
	WasmModuleID *string   `json:"wasm_module_id,omitempty" db:"wasm_module_id"`
	Config       []byte    `json:"config" db:"config"` // JSON configuration
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// WasmModule represents a WebAssembly module
type WasmModule struct {
	ID          string    `json:"id" db:"id"`
	Name        string    `json:"name" db:"name"`
	Description string    `json:"description" db:"description"`
	ModuleData  []byte    `json:"module_data" db:"module_data"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// Job represents a workflow execution instance
type Job struct {
	ID          string     `json:"id" db:"id"`
	WorkflowID  string     `json:"workflow_id" db:"workflow_id"`
	Status      string     `json:"status" db:"status"`           // "QUEUED", "RUNNING", "COMPLETED", "FAILED"
	InputData   []byte     `json:"input_data" db:"input_data"`   // JSON input data
	OutputData  []byte     `json:"output_data" db:"output_data"` // JSON output data
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	StartedAt   *time.Time `json:"started_at,omitempty" db:"started_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty" db:"completed_at"`
}

// JobStep represents the execution of a single step within a job
type JobStep struct {
	ID             string     `json:"id" db:"id"`
	JobID          string     `json:"job_id" db:"job_id"`
	WorkflowStepID string     `json:"workflow_step_id" db:"workflow_step_id"`
	Status         string     `json:"status" db:"status"`           // "PENDING", "RUNNING", "COMPLETED", "FAILED"
	InputData      []byte     `json:"input_data" db:"input_data"`   // JSON input data
	OutputData     []byte     `json:"output_data" db:"output_data"` // JSON output data
	StartedAt      *time.Time `json:"started_at,omitempty" db:"started_at"`
	CompletedAt    *time.Time `json:"completed_at,omitempty" db:"completed_at"`
}

// Artifact represents persistent data produced during job executions
type Artifact struct {
	ID        string    `json:"id" db:"id"`
	JobID     string    `json:"job_id" db:"job_id"`
	Name      string    `json:"name" db:"name"`
	MimeType  string    `json:"mime_type" db:"mime_type"`
	Data      []byte    `json:"data" db:"data"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// WasmModuleSource represents the source code for a WASM module
type WasmModuleSource struct {
	ID                string    `json:"id" db:"id"`
	WasmModuleID      string    `json:"wasm_module_id" db:"wasm_module_id"`
	Language          string    `json:"language" db:"language"`
	SourceCode        string    `json:"source_code" db:"source_code"`
	Version           int       `json:"version" db:"version"`
	CompilationStatus string    `json:"compilation_status" db:"compilation_status"`
	CompilationError  *string   `json:"compilation_error,omitempty" db:"compilation_error"`
	CompiledAt        *time.Time `json:"compiled_at,omitempty" db:"compiled_at"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}
