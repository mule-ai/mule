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
	Type        string                 `json:"type"`
	Config      map[string]interface{} `json:"config"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// Agent represents an AI agent.
type Agent struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	ProviderID   string    `json:"provider_id"`
	ModelID      string    `json:"model_id"`
	SystemPrompt string    `json:"system_prompt"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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
	ListWorkflowSteps(ctx context.Context, workflowID string) ([]*WorkflowStep, error)
}

// ErrNotFound is returned when a requested primitive is not found.
var ErrNotFound = errors.New("not found")
