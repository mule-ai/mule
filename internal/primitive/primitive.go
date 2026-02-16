package primitive

import (
	"context"
	"errors"
	"time"
)

// Provider represents AI provider configuration.
type Provider struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	APIBaseURL string    `json:"api_base_url"`
	APIKeyEnc  string    `json:"api_key_encrypted"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Tool represents an external or internal tool.
type Tool struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// Agent represents an AI agent.
type Agent struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	ProviderID   string                 `json:"provider_id"`
	ModelID      string                 `json:"model_id"`
	SystemPrompt string                 `json:"system_prompt"`
	PIConfig     map[string]interface{} `json:"pi_config"` // PI-specific configuration
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

// Skill represents a pi agent skill.
type Skill struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Path        string    `json:"path"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// Workflow represents an ordered sequence of steps.
type Workflow struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsAsync     bool      `json:"is_async"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// MemoryConfig represents configuration for the genai memory tool.
type MemoryConfig struct {
	ID                string    `json:"id"`
	DatabaseURL       string    `json:"database_url"`
	EmbeddingProvider string    `json:"embedding_provider"`
	EmbeddingModel    string    `json:"embedding_model"`
	EmbeddingDims     int       `json:"embedding_dims"`
	DefaultTTLSeconds int       `json:"default_ttl_seconds"`
	DefaultTopK       int       `json:"default_top_k"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Setting represents an application configuration setting.
type Setting struct {
	ID          string    `json:"id"`
	Key         string    `json:"key"`
	Value       string    `json:"value"`
	Description string    `json:"description"`
	Category    string    `json:"category"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// WorkflowStep represents a single step in a workflow.
type WorkflowStep struct {
	ID           string                 `json:"id"`
	WorkflowID   string                 `json:"workflow_id"`
	StepOrder    int                    `json:"step_order"`
	StepType     string                 `json:"type"`
	AgentID      *string                `json:"agent_id"`
	WasmModuleID *string                `json:"wasm_module_id"`
	Config       map[string]interface{} `json:"config"`
	CreatedAt    time.Time              `json:"created_at"`
}

// WasmModule represents a WebAssembly module.
type WasmModule struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	ModuleData  []byte                 `json:"module_data"`
	Config      map[string]interface{} `json:"config"` // JSON configuration schema/metadata
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// WasmModuleListItem represents a lightweight version of a WebAssembly module for listing.
type WasmModuleListItem struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Config      map[string]interface{} `json:"config"` // JSON configuration schema/metadata
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// PrimitiveStore defines interface for primitive management.
type PrimitiveStore interface {
	CreateProvider(ctx context.Context, p *Provider) error
	GetProvider(ctx context.Context, id string) (*Provider, error)
	ListProviders(ctx context.Context) ([]*Provider, error)
	UpdateProvider(ctx context.Context, p *Provider) error
	DeleteProvider(ctx context.Context, id string) error

	CreateTool(ctx context.Context, t *Tool) error
	GetTool(ctx context.Context, id string) (*Tool, error)
	ListTools(ctx context.Context) ([]*Tool, error)
	UpdateTool(ctx context.Context, t *Tool) error
	DeleteTool(ctx context.Context, id string) error

	CreateAgent(ctx context.Context, a *Agent) error
	GetAgent(ctx context.Context, id string) (*Agent, error)
	ListAgents(ctx context.Context) ([]*Agent, error)
	UpdateAgent(ctx context.Context, a *Agent) error
	DeleteAgent(ctx context.Context, id string) error

	CreateWorkflow(ctx context.Context, w *Workflow) error
	GetWorkflow(ctx context.Context, id string) (*Workflow, error)
	ListWorkflows(ctx context.Context) ([]*Workflow, error)
	UpdateWorkflow(ctx context.Context, w *Workflow) error
	DeleteWorkflow(ctx context.Context, id string) error

	CreateWorkflowStep(ctx context.Context, s *WorkflowStep) error
	GetWorkflowStep(ctx context.Context, id string) (*WorkflowStep, error)
	ListWorkflowSteps(ctx context.Context, workflowID string) ([]*WorkflowStep, error)

	// WASM module methods
	CreateWasmModule(ctx context.Context, w *WasmModule) error
	GetWasmModule(ctx context.Context, id string) (*WasmModule, error)
	ListWasmModules(ctx context.Context) ([]*WasmModuleListItem, error)
	UpdateWasmModule(ctx context.Context, w *WasmModule) error
	DeleteWasmModule(ctx context.Context, id string) error

	// GetAgentTools retrieves tools associated with an agent
	GetAgentTools(ctx context.Context, agentID string) ([]*Tool, error)

	// AssignToolToAgent assigns a tool to an agent
	AssignToolToAgent(ctx context.Context, agentID, toolID string) error

	// RemoveToolFromAgent removes a tool from an agent
	RemoveToolFromAgent(ctx context.Context, agentID, toolID string) error

	// Skill methods
	CreateSkill(ctx context.Context, s *Skill) error
	GetSkill(ctx context.Context, id string) (*Skill, error)
	ListSkills(ctx context.Context) ([]*Skill, error)
	UpdateSkill(ctx context.Context, s *Skill) error
	DeleteSkill(ctx context.Context, id string) error

	// GetAgentSkills retrieves skills associated with an agent
	GetAgentSkills(ctx context.Context, agentID string) ([]*Skill, error)

	// AssignSkillToAgent assigns a skill to an agent
	AssignSkillToAgent(ctx context.Context, agentID, skillID string) error

	// RemoveSkillFromAgent removes a skill from an agent
	RemoveSkillFromAgent(ctx context.Context, agentID, skillID string) error

	// SetAgentSkills replaces all skills for an agent
	SetAgentSkills(ctx context.Context, agentID string, skillIDs []string) error

	// Memory configuration methods
	GetMemoryConfig(ctx context.Context, id string) (*MemoryConfig, error)
	UpdateMemoryConfig(ctx context.Context, config *MemoryConfig) error

	// Settings methods
	GetSetting(ctx context.Context, key string) (*Setting, error)
	ListSettings(ctx context.Context) ([]*Setting, error)
	UpdateSetting(ctx context.Context, setting *Setting) error
}

// ErrNotFound is returned when a requested primitive is not found.
var ErrNotFound = errors.New("not found")
