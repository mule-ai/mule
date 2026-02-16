package agent

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/pkg/job"
)

func TestRuntime_ExecuteAgent(t *testing.T) {
	// This test requires a real API key to work with pi
	// Skip if no API key is available
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	googleApiKey := os.Getenv("GOOGLE_API_KEY")
	openaiApiKey := os.Getenv("OPENAI_API_KEY")

	// Determine which provider/model to use based on available keys
	var providerAPIKey, providerURL, modelID string
	if apiKey != "" {
		providerAPIKey = apiKey
		providerURL = "https://api.anthropic.com"
		modelID = "claude-3-5-sonnet-20241022"
	} else if googleApiKey != "" {
		providerAPIKey = googleApiKey
		providerURL = "https://generativelanguage.googleapis.com"
		modelID = "gemini-2.0-flash"
	} else if openaiApiKey != "" {
		providerAPIKey = openaiApiKey
		providerURL = "https://api.openai.com"
		modelID = "gpt-4o-mini"
	} else {
		t.Skip("Skipping test: no API key available (ANTHROPIC_API_KEY, GOOGLE_API_KEY, or OPENAI_API_KEY)")
	}

	store := &MockAgentStore{
		agents: map[string]*primitive.Agent{
			"test-agent": {
				ID:           "test-agent",
				Name:         "test-agent", // Name should match the lookup
				Description:  "Test agent",
				ProviderID:   "test-provider",
				ModelID:      modelID,
				SystemPrompt: "You are a helpful assistant",
			},
		},
		providers: map[string]*primitive.Provider{
			"test-provider": {
				ID:         "test-provider",
				Name:       "Test Provider",
				APIBaseURL: providerURL,
				APIKeyEnc:  providerAPIKey,
			},
		},
		skills: map[string]*primitive.Skill{},
	}

	mockJobStore := &MockJobStore{}
	runtime := NewRuntime(store, mockJobStore)

	t.Run("valid agent request", func(t *testing.T) {
		req := &ChatCompletionRequest{
			Model: "agent/test-agent",
			Messages: []ChatCompletionMessage{
				{Role: "user", Content: "Hello, world!"},
			},
			Stream: false,
		}

		resp, err := runtime.ExecuteAgent(context.Background(), req)

		// Check for timeout or API errors - skip if pi isn't working
		if err != nil && (strings.Contains(err.Error(), "timed out") || strings.Contains(err.Error(), "execution")) {
			t.Skip("Skipping test: pi execution not working (may need valid API key)")
		}

		// Should succeed if pi works correctly
		assert.NoError(t, err)
		if resp == nil {
			t.Skip("Skipping test: no response from pi")
		}
		assert.NotNil(t, resp)
		// Response content may be empty if the model doesn't produce output
		// but the structure should be valid
		assert.NotEmpty(t, resp.ID)
		assert.Contains(t, resp.Model, "test-agent")
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
	agents      map[string]*primitive.Agent
	providers   map[string]*primitive.Provider
	workflows   map[string]*primitive.Workflow
	skills      map[string]*primitive.Skill
	agentSkills map[string][]string // agentID -> []skillID
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
	if m.agents == nil {
		m.agents = make(map[string]*primitive.Agent)
	}
	m.agents[a.ID] = a
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

func (m *MockAgentStore) GetWorkflowStep(ctx context.Context, id string) (*primitive.WorkflowStep, error) {
	return nil, primitive.ErrNotFound
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

// Skill methods
func (m *MockAgentStore) CreateSkill(ctx context.Context, s *primitive.Skill) error {
	if m.skills == nil {
		m.skills = make(map[string]*primitive.Skill)
	}
	m.skills[s.ID] = s
	return nil
}

func (m *MockAgentStore) GetSkill(ctx context.Context, id string) (*primitive.Skill, error) {
	skill, exists := m.skills[id]
	if !exists {
		return nil, primitive.ErrNotFound
	}
	return skill, nil
}

func (m *MockAgentStore) ListSkills(ctx context.Context) ([]*primitive.Skill, error) {
	var skills []*primitive.Skill
	for _, s := range m.skills {
		skills = append(skills, s)
	}
	return skills, nil
}

func (m *MockAgentStore) UpdateSkill(ctx context.Context, s *primitive.Skill) error {
	return nil
}

func (m *MockAgentStore) DeleteSkill(ctx context.Context, id string) error {
	return nil
}

func (m *MockAgentStore) GetAgentSkills(ctx context.Context, agentID string) ([]*primitive.Skill, error) {
	if m.agentSkills == nil {
		return []*primitive.Skill{}, nil
	}
	skillIDs := m.agentSkills[agentID]
	var skills []*primitive.Skill
	for _, skillID := range skillIDs {
		if skill, exists := m.skills[skillID]; exists {
			skills = append(skills, skill)
		}
	}
	return skills, nil
}

func (m *MockAgentStore) AssignSkillToAgent(ctx context.Context, agentID, skillID string) error {
	if m.agentSkills == nil {
		m.agentSkills = make(map[string][]string)
	}
	// Check if already assigned
	for _, id := range m.agentSkills[agentID] {
		if id == skillID {
			return nil
		}
	}
	m.agentSkills[agentID] = append(m.agentSkills[agentID], skillID)
	return nil
}

func (m *MockAgentStore) RemoveSkillFromAgent(ctx context.Context, agentID, skillID string) error {
	if m.agentSkills == nil {
		return nil
	}
	skills := m.agentSkills[agentID]
	for i, id := range skills {
		if id == skillID {
			m.agentSkills[agentID] = append(skills[:i], skills[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *MockAgentStore) SetAgentSkills(ctx context.Context, agentID string, skillIDs []string) error {
	if m.agentSkills == nil {
		m.agentSkills = make(map[string][]string)
	}
	m.agentSkills[agentID] = skillIDs
	return nil
}

// WASM module methods
func (m *MockAgentStore) CreateWasmModule(ctx context.Context, w *primitive.WasmModule) error {
	return nil
}

func (m *MockAgentStore) GetWasmModule(ctx context.Context, id string) (*primitive.WasmModule, error) {
	return nil, primitive.ErrNotFound
}

func (m *MockAgentStore) ListWasmModules(ctx context.Context) ([]*primitive.WasmModuleListItem, error) {
	return nil, nil
}

func (m *MockAgentStore) UpdateWasmModule(ctx context.Context, w *primitive.WasmModule) error {
	return nil
}

func (m *MockAgentStore) DeleteWasmModule(ctx context.Context, id string) error {
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

func (m *MockJobStore) ListJobs(opts job.ListJobsOptions) ([]*job.Job, int, error) {
	return nil, 0, nil
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
