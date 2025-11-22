package engine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mule-ai/mule/internal/agent"
	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/pkg/job"
)

func TestWorkflowExecution(t *testing.T) {
	// Create mock stores
	mockStore := &MockPrimitiveStore{
		Workflows: []*primitive.Workflow{
			{
				ID:          "workflow-1",
				Name:        "test-workflow",
				Description: "Test Workflow",
			},
		},
		WorkflowSteps: []*primitive.WorkflowStep{
			{
				ID:         "step-1",
				WorkflowID: "workflow-1",
				StepType:   "agent",
				AgentID:    &[]string{"agent-1"}[0],
				StepOrder:  1,
			},
		},
		Agents: []*primitive.Agent{
			{
				ID:           "agent-1",
				Name:         "test-agent",
				Description:  "Test Agent",
				ProviderID:   "provider-1",
				ModelID:      "gemini-1.5-flash",
				SystemPrompt: "You are a helpful assistant",
			},
		},
		Providers: []*primitive.Provider{
			{
				ID:         "provider-1",
				Name:       "Test Provider",
				APIBaseURL: "https://api.test.com",
				APIKeyEnc:  "test-api-key",
			},
		},
	}

	mockJobStore := &MockJobStore{
		Jobs: make(map[string]*job.Job),
	}

	// Create components
	agentRuntime := agent.NewRuntime(mockStore, mockJobStore)
	wasmExecutor := NewWASMExecutor(nil) // Use nil DB for testing

	// Create engine with single worker for testing
	engine := NewEngine(mockStore, mockJobStore, agentRuntime, wasmExecutor, Config{Workers: 1})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Start the engine
	err := engine.Start(ctx)
	assert.NoError(t, err)
	defer engine.Stop()

	// Create a test job
	testJob := &job.Job{
		ID:         "job-1",
		WorkflowID: "workflow-1",
		Status:     job.StatusQueued,
		InputData:  map[string]interface{}{"message": "Hello, workflow!"},
		CreatedAt:  time.Now(),
	}

	// Store the job
	err = mockJobStore.CreateJob(testJob)
	assert.NoError(t, err)

	// Wait a bit for the job to be processed
	time.Sleep(2 * time.Second)

	// Check job status
	retrievedJob, err := mockJobStore.GetJob("job-1")
	assert.NoError(t, err)

	// The job should have been processed (may have failed due to invalid API key, but should be attempted)
	assert.NotEqual(t, job.StatusQueued, retrievedJob.Status)
}

func TestEngineConfiguration(t *testing.T) {
	mockStore := &MockPrimitiveStore{}
	mockJobStore := &MockJobStore{Jobs: make(map[string]*job.Job)}
	agentRuntime := agent.NewRuntime(mockStore, mockJobStore)

	// Test engine creation with different configurations
	t.Run("single worker", func(t *testing.T) {
		engine := NewEngine(mockStore, mockJobStore, agentRuntime, NewWASMExecutor(nil), Config{Workers: 1})
		assert.Equal(t, 1, engine.workers)
	})

	t.Run("multiple workers", func(t *testing.T) {
		engine := NewEngine(mockStore, mockJobStore, agentRuntime, NewWASMExecutor(nil), Config{Workers: 5})
		assert.Equal(t, 5, engine.workers)
	})
}

// MockPrimitiveStore implements primitive.PrimitiveStore for testing
type MockPrimitiveStore struct {
	Workflows     []*primitive.Workflow
	WorkflowSteps []*primitive.WorkflowStep
	Agents        []*primitive.Agent
	Providers     []*primitive.Provider
}

func (m *MockPrimitiveStore) CreateProvider(ctx context.Context, p *primitive.Provider) error {
	return nil
}

func (m *MockPrimitiveStore) GetProvider(ctx context.Context, id string) (*primitive.Provider, error) {
	for _, p := range m.Providers {
		if p.ID == id {
			return p, nil
		}
	}
	return nil, primitive.ErrNotFound
}

func (m *MockPrimitiveStore) ListProviders(ctx context.Context) ([]*primitive.Provider, error) {
	return m.Providers, nil
}

func (m *MockPrimitiveStore) UpdateProvider(ctx context.Context, p *primitive.Provider) error {
	return nil
}

func (m *MockPrimitiveStore) DeleteProvider(ctx context.Context, id string) error {
	return nil
}

func (m *MockPrimitiveStore) CreateTool(ctx context.Context, t *primitive.Tool) error {
	return nil
}

func (m *MockPrimitiveStore) GetTool(ctx context.Context, id string) (*primitive.Tool, error) {
	return nil, primitive.ErrNotFound
}

func (m *MockPrimitiveStore) ListTools(ctx context.Context) ([]*primitive.Tool, error) {
	return nil, nil
}

func (m *MockPrimitiveStore) UpdateTool(ctx context.Context, t *primitive.Tool) error {
	return nil
}

func (m *MockPrimitiveStore) DeleteTool(ctx context.Context, id string) error {
	return nil
}

func (m *MockPrimitiveStore) CreateAgent(ctx context.Context, a *primitive.Agent) error {
	return nil
}

func (m *MockPrimitiveStore) GetAgent(ctx context.Context, id string) (*primitive.Agent, error) {
	for _, a := range m.Agents {
		if a.ID == id {
			return a, nil
		}
	}
	return nil, primitive.ErrNotFound
}

func (m *MockPrimitiveStore) ListAgents(ctx context.Context) ([]*primitive.Agent, error) {
	return m.Agents, nil
}

func (m *MockPrimitiveStore) UpdateAgent(ctx context.Context, a *primitive.Agent) error {
	return nil
}

func (m *MockPrimitiveStore) DeleteAgent(ctx context.Context, id string) error {
	return nil
}

func (m *MockPrimitiveStore) CreateWorkflow(ctx context.Context, w *primitive.Workflow) error {
	return nil
}

func (m *MockPrimitiveStore) GetWorkflow(ctx context.Context, id string) (*primitive.Workflow, error) {
	for _, w := range m.Workflows {
		if w.ID == id {
			return w, nil
		}
	}
	return nil, primitive.ErrNotFound
}

func (m *MockPrimitiveStore) ListWorkflows(ctx context.Context) ([]*primitive.Workflow, error) {
	return m.Workflows, nil
}

func (m *MockPrimitiveStore) UpdateWorkflow(ctx context.Context, w *primitive.Workflow) error {
	return nil
}

func (m *MockPrimitiveStore) DeleteWorkflow(ctx context.Context, id string) error {
	return nil
}

func (m *MockPrimitiveStore) CreateWorkflowStep(ctx context.Context, s *primitive.WorkflowStep) error {
	return nil
}

func (m *MockPrimitiveStore) ListWorkflowSteps(ctx context.Context, workflowID string) ([]*primitive.WorkflowStep, error) {
	var steps []*primitive.WorkflowStep
	for _, step := range m.WorkflowSteps {
		if step.WorkflowID == workflowID {
			steps = append(steps, step)
		}
	}
	return steps, nil
}

func (m *MockPrimitiveStore) GetAgentTools(ctx context.Context, agentID string) ([]*primitive.Tool, error) {
	// Return empty tools for testing - no tools by default
	return []*primitive.Tool{}, nil
}

func (m *MockPrimitiveStore) AssignToolToAgent(ctx context.Context, agentID, toolID string) error {
	// Mock implementation - can be extended for testing
	return nil
}

func (m *MockPrimitiveStore) RemoveToolFromAgent(ctx context.Context, agentID, toolID string) error {
	// Mock implementation - can be extended for testing
	return nil
}

// MockJobStore implements job.JobStore for testing
type MockJobStore struct {
	Jobs map[string]*job.Job
}

func (m *MockJobStore) CreateJob(j *job.Job) error {
	m.Jobs[j.ID] = j
	return nil
}

func (m *MockJobStore) GetJob(id string) (*job.Job, error) {
	if job, exists := m.Jobs[id]; exists {
		return job, nil
	}
	return nil, job.ErrJobNotFound
}

func (m *MockJobStore) ListJobs() ([]*job.Job, error) {
	var jobs []*job.Job
	for _, j := range m.Jobs {
		jobs = append(jobs, j)
	}
	return jobs, nil
}

func (m *MockJobStore) UpdateJob(j *job.Job) error {
	if _, exists := m.Jobs[j.ID]; exists {
		m.Jobs[j.ID] = j
		return nil
	}
	return job.ErrJobNotFound
}

func (m *MockJobStore) DeleteJob(id string) error {
	if _, exists := m.Jobs[id]; exists {
		delete(m.Jobs, id)
		return nil
	}
	return job.ErrJobNotFound
}

func (m *MockJobStore) CreateJobStep(s *job.JobStep) error {
	return nil
}

func (m *MockJobStore) GetJobStep(id string) (*job.JobStep, error) {
	return nil, job.ErrJobStepNotFound
}

func (m *MockJobStore) ListJobSteps(jobID string) ([]*job.JobStep, error) {
	return nil, nil
}

func (m *MockJobStore) UpdateJobStep(s *job.JobStep) error {
	return nil
}

func (m *MockJobStore) DeleteJobStep(id string) error {
	return nil
}

func (m *MockJobStore) ListJobsByStatus(status job.Status) ([]*job.Job, error) {
	var jobs []*job.Job
	for _, j := range m.Jobs {
		if j.Status == status {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

func (m *MockJobStore) GetNextQueuedJob() (*job.Job, error) {
	for _, j := range m.Jobs {
		if j.Status == job.StatusQueued {
			return j, nil
		}
	}
	return nil, job.ErrJobNotFound
}

func (m *MockJobStore) MarkJobRunning(jobID string) error {
	if jobItem, exists := m.Jobs[jobID]; exists {
		jobItem.Status = job.StatusRunning
		return nil
	}
	return job.ErrJobNotFound
}

func (m *MockJobStore) MarkJobCompleted(jobID string, outputData map[string]interface{}) error {
	if jobItem, exists := m.Jobs[jobID]; exists {
		jobItem.Status = job.StatusCompleted
		jobItem.OutputData = outputData
		return nil
	}
	return job.ErrJobNotFound
}

func (m *MockJobStore) MarkJobFailed(jobID string, err error) error {
	if jobItem, exists := m.Jobs[jobID]; exists {
		jobItem.Status = job.StatusFailed
		return nil
	}
	return job.ErrJobNotFound
}
