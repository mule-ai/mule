package primitive

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockStore implements Store interface for testing
type MockStore struct {
	providers     map[string]Provider
	tools         map[string]Tool
	agents        map[string]Agent
	workflows     map[string]Workflow
	workflowSteps map[string]WorkflowStep
}

func NewMockStore() *MockStore {
	return &MockStore{
		providers:     make(map[string]Provider),
		tools:         make(map[string]Tool),
		agents:        make(map[string]Agent),
		workflows:     make(map[string]Workflow),
		workflowSteps: make(map[string]WorkflowStep),
	}
}

func (m *MockStore) CreateProvider(ctx context.Context, provider *Provider) error {
	m.providers[provider.ID] = *provider
	return nil
}

func (m *MockStore) GetProvider(ctx context.Context, id string) (*Provider, error) {
	provider, exists := m.providers[id]
	if !exists {
		return nil, ErrNotFound
	}
	return &provider, nil
}

func (m *MockStore) ListProviders(ctx context.Context) ([]Provider, error) {
	var providers []Provider
	for _, p := range m.providers {
		providers = append(providers, p)
	}
	return providers, nil
}

func (m *MockStore) UpdateProvider(ctx context.Context, provider *Provider) error {
	if _, exists := m.providers[provider.ID]; !exists {
		return ErrNotFound
	}
	m.providers[provider.ID] = *provider
	return nil
}

func (m *MockStore) DeleteProvider(ctx context.Context, id string) error {
	if _, exists := m.providers[id]; !exists {
		return ErrNotFound
	}
	delete(m.providers, id)
	return nil
}

func (m *MockStore) CreateTool(ctx context.Context, tool *Tool) error {
	m.tools[tool.ID] = *tool
	return nil
}

func (m *MockStore) GetTool(ctx context.Context, id string) (*Tool, error) {
	tool, exists := m.tools[id]
	if !exists {
		return nil, ErrNotFound
	}
	return &tool, nil
}

func (m *MockStore) ListTools(ctx context.Context) ([]Tool, error) {
	var tools []Tool
	for _, t := range m.tools {
		tools = append(tools, t)
	}
	return tools, nil
}

func (m *MockStore) UpdateTool(ctx context.Context, tool *Tool) error {
	if _, exists := m.tools[tool.ID]; !exists {
		return ErrNotFound
	}
	m.tools[tool.ID] = *tool
	return nil
}

func (m *MockStore) DeleteTool(ctx context.Context, id string) error {
	if _, exists := m.tools[id]; !exists {
		return ErrNotFound
	}
	delete(m.tools, id)
	return nil
}

func (m *MockStore) CreateAgent(ctx context.Context, agent *Agent) error {
	m.agents[agent.ID] = *agent
	return nil
}

func (m *MockStore) GetAgent(ctx context.Context, id string) (*Agent, error) {
	agent, exists := m.agents[id]
	if !exists {
		return nil, ErrNotFound
	}
	return &agent, nil
}

func (m *MockStore) ListAgents(ctx context.Context) ([]Agent, error) {
	var agents []Agent
	for _, a := range m.agents {
		agents = append(agents, a)
	}
	return agents, nil
}

func (m *MockStore) UpdateAgent(ctx context.Context, agent *Agent) error {
	if _, exists := m.agents[agent.ID]; !exists {
		return ErrNotFound
	}
	m.agents[agent.ID] = *agent
	return nil
}

func (m *MockStore) DeleteAgent(ctx context.Context, id string) error {
	if _, exists := m.agents[id]; !exists {
		return ErrNotFound
	}
	delete(m.agents, id)
	return nil
}

func (m *MockStore) CreateWorkflow(ctx context.Context, workflow *Workflow) error {
	m.workflows[workflow.ID] = *workflow
	return nil
}

func (m *MockStore) GetWorkflow(ctx context.Context, id string) (*Workflow, error) {
	workflow, exists := m.workflows[id]
	if !exists {
		return nil, ErrNotFound
	}
	return &workflow, nil
}

func (m *MockStore) ListWorkflows(ctx context.Context) ([]Workflow, error) {
	var workflows []Workflow
	for _, w := range m.workflows {
		workflows = append(workflows, w)
	}
	return workflows, nil
}

func (m *MockStore) UpdateWorkflow(ctx context.Context, workflow *Workflow) error {
	if _, exists := m.workflows[workflow.ID]; !exists {
		return ErrNotFound
	}
	m.workflows[workflow.ID] = *workflow
	return nil
}

func (m *MockStore) DeleteWorkflow(ctx context.Context, id string) error {
	if _, exists := m.workflows[id]; !exists {
		return ErrNotFound
	}
	delete(m.workflows, id)
	return nil
}

func (m *MockStore) CreateWorkflowStep(ctx context.Context, step *WorkflowStep) error {
	m.workflowSteps[step.ID] = *step
	return nil
}

func (m *MockStore) ListWorkflowSteps(ctx context.Context, workflowID string) ([]WorkflowStep, error) {
	var steps []WorkflowStep
	for _, step := range m.workflowSteps {
		if step.WorkflowID == workflowID {
			steps = append(steps, step)
		}
	}
	return steps, nil
}

func TestProvider(t *testing.T) {
	provider := &Provider{
		ID:         "test-provider",
		Name:       "Test Provider",
		APIBaseURL: "https://api.openai.com",
		APIKeyEnc:  "sk-test",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	assert.Equal(t, "test-provider", provider.ID)
	assert.Equal(t, "Test Provider", provider.Name)
	assert.Equal(t, "https://api.openai.com", provider.APIBaseURL)
	assert.Equal(t, "sk-test", provider.APIKeyEnc)
}

func TestTool(t *testing.T) {
	tool := &Tool{
		ID:          "test-tool",
		Name:        "Test Tool",
		Type:        "api",
		Description: "Test tool",
		Config: map[string]interface{}{
			"url": "https://api.example.com",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	assert.Equal(t, "test-tool", tool.ID)
	assert.Equal(t, "Test Tool", tool.Name)
	assert.Equal(t, "api", tool.Type)
	assert.Equal(t, "https://api.example.com", tool.Config["url"])
}

func TestAgent(t *testing.T) {
	agent := &Agent{
		ID:           "test-agent",
		Name:         "Test Agent",
		Description:  "Test agent description",
		ProviderID:   "test-provider",
		ModelID:      "gpt-4",
		SystemPrompt: "You are a test agent",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	assert.Equal(t, "test-agent", agent.ID)
	assert.Equal(t, "Test Agent", agent.Name)
	assert.Equal(t, "gpt-4", agent.ModelID)
	assert.Equal(t, "test-provider", agent.ProviderID)
}

func TestWorkflow(t *testing.T) {
	workflow := &Workflow{
		ID:          "test-workflow",
		Name:        "Test Workflow",
		Description: "Test workflow",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	assert.Equal(t, "test-workflow", workflow.ID)
	assert.Equal(t, "Test Workflow", workflow.Name)
}

func TestMockStore(t *testing.T) {
	ctx := context.Background()
	store := NewMockStore()

	// Test Provider
	provider := &Provider{
		ID:         "test-provider",
		Name:       "Test Provider",
		APIBaseURL: "https://api.openai.com",
		APIKeyEnc:  "sk-test",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	err := store.CreateProvider(ctx, provider)
	require.NoError(t, err)

	retrieved, err := store.GetProvider(ctx, "test-provider")
	require.NoError(t, err)
	assert.Equal(t, provider.Name, retrieved.Name)

	providers, err := store.ListProviders(ctx)
	require.NoError(t, err)
	assert.Len(t, providers, 1)

	provider.Name = "Updated Provider"
	err = store.UpdateProvider(ctx, provider)
	require.NoError(t, err)

	err = store.DeleteProvider(ctx, "test-provider")
	require.NoError(t, err)

	_, err = store.GetProvider(ctx, "test-provider")
	assert.Error(t, err)
	assert.Equal(t, ErrNotFound, err)

	// Test Tool
	tool := &Tool{
		ID:          "test-tool",
		Name:        "Test Tool",
		Type:        "api",
		Description: "Test tool",
		Config:      map[string]interface{}{"url": "https://api.example.com"},
	}

	err = store.CreateTool(ctx, tool)
	require.NoError(t, err)

	retrievedTool, err := store.GetTool(ctx, "test-tool")
	require.NoError(t, err)
	assert.Equal(t, tool.Name, retrievedTool.Name)

	// Test Agent
	agent := &Agent{
		ID:           "test-agent",
		Name:         "Test Agent",
		Description:  "Test agent description",
		ProviderID:   "test-provider",
		ModelID:      "gpt-4",
		SystemPrompt: "You are a test agent",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	err = store.CreateAgent(ctx, agent)
	require.NoError(t, err)

	retrievedAgent, err := store.GetAgent(ctx, "test-agent")
	require.NoError(t, err)
	assert.Equal(t, agent.Name, retrievedAgent.Name)

	// Test Workflow
	step := WorkflowStep{
		ID:         "step1",
		WorkflowID: "test-workflow",
		StepOrder:  1,
		StepType:   "agent",
		AgentID:    &[]string{"test-agent"}[0],
		Config:     map[string]interface{}{},
		CreatedAt:  time.Now(),
	}

	workflow := &Workflow{
		ID:          "test-workflow",
		Name:        "Test Workflow",
		Description: "Test workflow",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	err = store.CreateWorkflow(ctx, workflow)
	require.NoError(t, err)

	err = store.CreateWorkflowStep(ctx, &step)
	require.NoError(t, err)

	retrievedWorkflow, err := store.GetWorkflow(ctx, "test-workflow")
	require.NoError(t, err)
	assert.Equal(t, workflow.Name, retrievedWorkflow.Name)
}
