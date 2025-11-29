package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/genai"

	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/pkg/job"
)

// MockGenAIClient is a mock implementation of the GenAIClient interface
type MockGenAIClient struct{}

func (m *MockGenAIClient) Models() ModelsClient {
	return &MockModelsClient{}
}

// MockModelsClient is a mock implementation of the ModelsClient interface
type MockModelsClient struct{}

func (m *MockModelsClient) GenerateContent(ctx context.Context, modelName string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, error) {
	// Return a mock response
	return &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{Text: "Mock response from test"},
					},
				},
			},
		},
	}, nil
}

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
				APIBaseURL: "", // Empty to route to Google ADK instead of custom LLM
				APIKeyEnc:  "test-api-key",
			},
		},
	}

	mockJobStore := &MockJobStore{}
	runtime := NewRuntime(store, mockJobStore)

	// Inject mock GenAI client to avoid real API calls
	mockClient := &MockGenAIClient{}
	runtime.SetGenAIClient(mockClient)

	t.Run("valid agent request", func(t *testing.T) {
		req := &ChatCompletionRequest{
			Model: "agent/test-agent",
			Messages: []ChatCompletionMessage{
				{Role: "user", Content: "Hello, world!"},
			},
			Stream: false,
		}

		resp, err := runtime.ExecuteAgent(context.Background(), req)

		// Should succeed with mock client
		assert.NoError(t, err)
		assert.NotNil(t, resp)
		assert.Equal(t, "Mock response from test", resp.Choices[0].Message.Content)
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

	mockJobStore := &MockJobStore{}
	runtime := NewRuntime(store, mockJobStore)

	t.Run("valid workflow request - engine not available", func(t *testing.T) {
		req := &ChatCompletionRequest{
			Model: "workflow/test-workflow",
			Messages: []ChatCompletionMessage{
				{Role: "user", Content: "Hello"},
			},
			Stream: false,
		}

		// Without a workflow engine set, this should fail
		resp, err := runtime.ExecuteWorkflow(context.Background(), req)
		assert.Error(t, err)
		assert.Nil(t, resp)
		assert.Contains(t, err.Error(), "workflow engine not available")
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

func (m *MockAgentStore) GetAgentTools(ctx context.Context, agentID string) ([]*primitive.Tool, error) {
	// Return empty tools for testing - no tools by default
	return []*primitive.Tool{}, nil
}

func (m *MockAgentStore) AssignToolToAgent(ctx context.Context, agentID, toolID string) error {
	// Mock implementation - can be extended for testing
	return nil
}

func (m *MockAgentStore) RemoveToolFromAgent(ctx context.Context, agentID, toolID string) error {
	// Mock implementation - can be extended for testing
	return nil
}

func (m *MockAgentStore) GetMemoryConfig(ctx context.Context, id string) (*primitive.MemoryConfig, error) {
	// Return not found to prevent database connections in tests
	return nil, primitive.ErrNotFound
}

func (m *MockAgentStore) UpdateMemoryConfig(ctx context.Context, config *primitive.MemoryConfig) error {
	// Mock implementation - just return nil for testing
	return nil
}

func (m *MockAgentStore) GetSetting(ctx context.Context, key string) (*primitive.Setting, error) {
	// Return not found to prevent database connections in tests
	return nil, primitive.ErrNotFound
}

func (m *MockAgentStore) ListSettings(ctx context.Context) ([]*primitive.Setting, error) {
	// Return empty settings for testing
	return []*primitive.Setting{}, nil
}

func (m *MockAgentStore) UpdateSetting(ctx context.Context, setting *primitive.Setting) error {
	// Mock implementation - just return nil for testing
	return nil
}

// MockJobStore implements job.JobStore for testing
type MockJobStore struct{}

func (m *MockJobStore) CreateJob(job *job.Job) error {
	return nil
}

func (m *MockJobStore) GetJob(id string) (*job.Job, error) {
	return nil, job.ErrJobNotFound
}

func (m *MockJobStore) ListJobs() ([]*job.Job, error) {
	return nil, nil
}

func (m *MockJobStore) UpdateJob(job *job.Job) error {
	return nil
}

func (m *MockJobStore) DeleteJob(id string) error {
	return nil
}

func (m *MockJobStore) CreateJobStep(step *job.JobStep) error {
	return nil
}

func (m *MockJobStore) GetJobStep(id string) (*job.JobStep, error) {
	return nil, job.ErrJobStepNotFound
}

func (m *MockJobStore) ListJobSteps(jobID string) ([]*job.JobStep, error) {
	return nil, nil
}

func (m *MockJobStore) UpdateJobStep(step *job.JobStep) error {
	return nil
}

func (m *MockJobStore) DeleteJobStep(id string) error {
	return nil
}

func (m *MockJobStore) GetNextQueuedJob() (*job.Job, error) {
	return nil, job.ErrJobNotFound
}

func (m *MockJobStore) MarkJobRunning(jobID string) error {
	return nil
}

func (m *MockJobStore) MarkJobCompleted(jobID string, outputData map[string]interface{}) error {
	return nil
}

func (m *MockJobStore) MarkJobFailed(jobID string, err error) error {
	return nil
}

func (m *MockJobStore) CancelJob(jobID string) error {
	return nil
}
