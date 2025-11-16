package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mule-ai/mule/internal/primitive"
)

func TestRuntime_ExecuteAgent(t *testing.T) {
	// Create mock store
	store := &MockAgentStore{
		agents: map[string]*primitive.Agent{
			"test-agent": {
				ID:           "test-agent",
				Name:         "test-agent", // Name should match the lookup
				Description:  "Test agent",
				ProviderID:   "test-provider",
				ModelID:      "gemini-1.5-flash",
				SystemPrompt: "You are a helpful assistant",
			},
		},
		providers: map[string]*primitive.Provider{
			"test-provider": {
				ID:         "test-provider",
				Name:       "Test Provider",
				APIBaseURL: "https://api.test.com",
				APIKeyEnc:  "test-api-key",
			},
		},
	}

	runtime := NewRuntime(store)

	t.Run("valid agent request", func(t *testing.T) {
		req := &ChatCompletionRequest{
			Model: "agent/test-agent",
			Messages: []ChatCompletionMessage{
				{Role: "user", Content: "Hello, world!"},
			},
			Stream: false,
		}

		// Note: This will fail in real test without proper API key, but tests the structure
		resp, err := runtime.ExecuteAgent(context.Background(), req)

		// We expect an error due to invalid API key, but the request parsing should work
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "API key not valid")
	})

	t.Run("agent not found", func(t *testing.T) {
		req := &ChatCompletionRequest{
			Model: "agent/nonexistent",
			Messages: []ChatCompletionMessage{
				{Role: "user", Content: "Hello"},
			},
			Stream: false,
		}

		resp, err := runtime.ExecuteAgent(context.Background(), req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("invalid model format", func(t *testing.T) {
		req := &ChatCompletionRequest{
			Model: "invalid-format",
			Messages: []ChatCompletionMessage{
				{Role: "user", Content: "Hello"},
			},
			Stream: false,
		}

		resp, err := runtime.ExecuteAgent(context.Background(), req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestRuntime_ExecuteWorkflow(t *testing.T) {
	store := &MockAgentStore{
		workflows: map[string]*primitive.Workflow{
			"test-workflow": {
				ID:          "test-workflow",
				Name:        "test-workflow", // Name should match the lookup
				Description: "Test workflow",
			},
		},
	}

	runtime := NewRuntime(store)

	t.Run("valid workflow request", func(t *testing.T) {
		req := &ChatCompletionRequest{
			Model: "workflow/test-workflow",
			Messages: []ChatCompletionMessage{
				{Role: "user", Content: "Hello"},
			},
			Stream: false,
		}

		resp, err := runtime.ExecuteWorkflow(context.Background(), req)
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "async.job", resp.Object)
		assert.Equal(t, "queued", resp.Status)
		assert.Contains(t, resp.Message, "started")
	})

	t.Run("workflow not found", func(t *testing.T) {
		req := &ChatCompletionRequest{
			Model: "workflow/nonexistent",
			Messages: []ChatCompletionMessage{
				{Role: "user", Content: "Hello"},
			},
			Stream: false,
		}

		resp, err := runtime.ExecuteWorkflow(context.Background(), req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "not found")
	})
}

// MockAgentStore implements primitive.PrimitiveStore for testing
type MockAgentStore struct {
	agents    map[string]*primitive.Agent
	providers map[string]*primitive.Provider
	workflows map[string]*primitive.Workflow
}

func (m *MockAgentStore) CreateProvider(ctx context.Context, p *primitive.Provider) error {
	return nil
}

func (m *MockAgentStore) GetProvider(ctx context.Context, id string) (*primitive.Provider, error) {
	provider, exists := m.providers[id]
	if !exists {
		return nil, primitive.ErrNotFound
	}
	return provider, nil
}

func (m *MockAgentStore) ListProviders(ctx context.Context) ([]*primitive.Provider, error) {
	var providers []*primitive.Provider
	for _, p := range m.providers {
		providers = append(providers, p)
	}
	return providers, nil
}

func (m *MockAgentStore) UpdateProvider(ctx context.Context, p *primitive.Provider) error {
	return nil
}

func (m *MockAgentStore) DeleteProvider(ctx context.Context, id string) error {
	return nil
}

func (m *MockAgentStore) CreateTool(ctx context.Context, t *primitive.Tool) error {
	return nil
}

func (m *MockAgentStore) GetTool(ctx context.Context, id string) (*primitive.Tool, error) {
	return nil, primitive.ErrNotFound
}

func (m *MockAgentStore) ListTools(ctx context.Context) ([]*primitive.Tool, error) {
	return nil, nil
}

func (m *MockAgentStore) UpdateTool(ctx context.Context, t *primitive.Tool) error {
	return nil
}

func (m *MockAgentStore) DeleteTool(ctx context.Context, id string) error {
	return nil
}

func (m *MockAgentStore) CreateAgent(ctx context.Context, a *primitive.Agent) error {
	return nil
}

func (m *MockAgentStore) GetAgent(ctx context.Context, id string) (*primitive.Agent, error) {
	agent, exists := m.agents[id]
	if !exists {
		return nil, primitive.ErrNotFound
	}
	return agent, nil
}

func (m *MockAgentStore) ListAgents(ctx context.Context) ([]*primitive.Agent, error) {
	var agents []*primitive.Agent
	for _, a := range m.agents {
		agents = append(agents, a)
	}
	return agents, nil
}

func (m *MockAgentStore) UpdateAgent(ctx context.Context, a *primitive.Agent) error {
	return nil
}

func (m *MockAgentStore) DeleteAgent(ctx context.Context, id string) error {
	return nil
}

func (m *MockAgentStore) CreateWorkflow(ctx context.Context, w *primitive.Workflow) error {
	return nil
}

func (m *MockAgentStore) GetWorkflow(ctx context.Context, id string) (*primitive.Workflow, error) {
	workflow, exists := m.workflows[id]
	if !exists {
		return nil, primitive.ErrNotFound
	}
	return workflow, nil
}

func (m *MockAgentStore) ListWorkflows(ctx context.Context) ([]*primitive.Workflow, error) {
	var workflows []*primitive.Workflow
	for _, w := range m.workflows {
		workflows = append(workflows, w)
	}
	return workflows, nil
}

func (m *MockAgentStore) UpdateWorkflow(ctx context.Context, w *primitive.Workflow) error {
	return nil
}

func (m *MockAgentStore) DeleteWorkflow(ctx context.Context, id string) error {
	return nil
}

func (m *MockAgentStore) CreateWorkflowStep(ctx context.Context, s *primitive.WorkflowStep) error {
	return nil
}

func (m *MockAgentStore) ListWorkflowSteps(ctx context.Context, workflowID string) ([]*primitive.WorkflowStep, error) {
	return nil, nil
}
