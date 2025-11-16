package primitive

import (
	"context"
	"errors"
	"time"
)

// Provider represents AI provider configuration.
type Provider struct {
	ID         string
	Name       string
	APIBaseURL string
	APIKeyEnc  string // encrypted API key
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Tool represents an external or internal tool.
type Tool struct {
	ID          string
	Name        string
	Description string
	Type        string
	Config      map[string]interface{}
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Agent represents an AI agent.
type Agent struct {
	ID           string
	Name         string
	Description  string
	ProviderID   string
	ModelID      string
	SystemPrompt string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Workflow represents an ordered sequence of steps.
type Workflow struct {
	ID          string
	Name        string
	Description string
	IsAsync     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// WorkflowStep represents a single step in a workflow.
type WorkflowStep struct {
	ID           string
	WorkflowID   string
	StepOrder    int
	StepType     string // "AGENT" or "WASM"
	AgentID      *string
	WasmModuleID *string
	Config       map[string]interface{}
	CreatedAt    time.Time
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
