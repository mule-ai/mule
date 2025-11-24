package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"

	"github.com/mule-ai/mule/internal/agent"
	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/internal/validation"
	"github.com/mule-ai/mule/pkg/job"
)

func TestChatCompletionsEndpoint(t *testing.T) {
	mockStore := &MockPrimitiveStore{
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
		Workflows: []*primitive.Workflow{
			{
				ID:          "workflow-1",
				Name:        "test-workflow",
				Description: "Test Workflow",
			},
		},
		Providers: []*primitive.Provider{
			{
				ID:         "provider-1",
				Name:       "Test Provider",
				APIBaseURL: "", // Empty to route to Google ADK instead of custom LLM
				APIKeyEnc:  "test-api-key",
			},
		},
	}

	mockJobStore := &MockJobStore{Jobs: make(map[string]*job.Job)}

	// Create runtime and set up workflow engine
	runtime := agent.NewRuntime(mockStore, mockJobStore)
	mockWorkflowEngine := &MockWorkflowEngine{}
	runtime.SetWorkflowEngine(mockWorkflowEngine)

	handler := &apiHandler{
		store:     mockStore,
		runtime:   runtime,
		jobStore:  mockJobStore,
		validator: validation.NewValidator(),
	}

	router := mux.NewRouter()
	router.HandleFunc("/v1/chat/completions", handler.chatCompletionsHandler).Methods("POST")

	t.Run("execute agent", func(t *testing.T) {
		request := map[string]interface{}{
			"model": "agent/test-agent",
			"messages": []map[string]string{
				{"role": "user", "content": "Hello, world!"},
			},
			"stream": false,
		}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		// Should fail due to invalid API key, but request structure should be valid
		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var errorResponse map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &errorResponse)
		assert.NoError(t, err)
		assert.Equal(t, "request_error", errorResponse["error"])
		// For 500 errors, the message is generic for security
		assert.Equal(t, "An internal server error occurred", errorResponse["message"])
	})

	t.Run("execute workflow", func(t *testing.T) {
		request := map[string]interface{}{
			"model": "async/workflow/test-workflow",
			"messages": []map[string]string{
				{"role": "user", "content": "Hello, workflow!"},
			},
			"stream": false,
		}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "async.job", response["object"])
		assert.Equal(t, "queued", response["status"])
		assert.Contains(t, response["message"], "started")
	})

	t.Run("invalid model format", func(t *testing.T) {
		request := map[string]interface{}{
			"model": "invalid-model",
			"messages": []map[string]string{
				{"role": "user", "content": "Hello!"},
			},
			"stream": false,
		}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var errorResponse map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &errorResponse)
		assert.NoError(t, err)
		assert.Equal(t, "request_error", errorResponse["error"])
		assert.Contains(t, errorResponse["message"], "model must start with 'agent/', 'workflow/', or 'async/workflow/'")
	})

	t.Run("agent not found", func(t *testing.T) {
		request := map[string]interface{}{
			"model": "agent/nonexistent-agent",
			"messages": []map[string]string{
				{"role": "user", "content": "Hello!"},
			},
			"stream": false,
		}

		body, _ := json.Marshal(request)
		req := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var errorResponse map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &errorResponse)
		assert.NoError(t, err)
		assert.Equal(t, "request_error", errorResponse["error"])
		assert.Equal(t, "An internal server error occurred", errorResponse["message"])
	})
}

func TestPrimitiveManagementEndpointsComprehensive(t *testing.T) {
	t.Run("provider CRUD operations", func(t *testing.T) {
		mockStore := &MockPrimitiveStore{}
		handler := &apiHandler{
			store:     mockStore,
			validator: validation.NewValidator(),
		}

		router := mux.NewRouter()
		// Provider endpoints
		router.HandleFunc("/api/v1/providers", handler.listProvidersHandler).Methods("GET")
		router.HandleFunc("/api/v1/providers", handler.createProviderHandler).Methods("POST")
		router.HandleFunc("/api/v1/providers/{id}", handler.getProviderHandler).Methods("GET")
		router.HandleFunc("/api/v1/providers/{id}", handler.updateProviderHandler).Methods("PUT")
		router.HandleFunc("/api/v1/providers/{id}", handler.deleteProviderHandler).Methods("DELETE")

		// Create provider
		provider := primitive.Provider{
			Name:       "Test Provider",
			APIBaseURL: "https://api.openai.com/v1",
			APIKeyEnc:  "test-api-key",
		}

		body, _ := json.Marshal(provider)
		req := httptest.NewRequest("POST", "/api/v1/providers", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var createdProvider primitive.Provider
		err := json.Unmarshal(w.Body.Bytes(), &createdProvider)
		assert.NoError(t, err)
		assert.NotEmpty(t, createdProvider.ID)

		// List providers
		req = httptest.NewRequest("GET", "/api/v1/providers", nil)
		w = httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var providers []primitive.Provider
		err = json.Unmarshal(w.Body.Bytes(), &providers)
		assert.NoError(t, err)
		assert.Len(t, providers, 1)

		// Get provider by ID
		req = httptest.NewRequest("GET", "/api/v1/providers/"+createdProvider.ID, nil)
		w = httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var retrievedProvider primitive.Provider
		err = json.Unmarshal(w.Body.Bytes(), &retrievedProvider)
		assert.NoError(t, err)
		assert.Equal(t, createdProvider.ID, retrievedProvider.ID)

		// Update provider
		updatedProvider := retrievedProvider
		updatedProvider.Name = "Updated Provider"

		body, _ = json.Marshal(updatedProvider)
		req = httptest.NewRequest("PUT", "/api/v1/providers/"+createdProvider.ID, bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w = httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		// Delete provider
		req = httptest.NewRequest("DELETE", "/api/v1/providers/"+createdProvider.ID, nil)
		w = httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)

		// Verify deletion
		req = httptest.NewRequest("GET", "/api/v1/providers", nil)
		w = httptest.NewRecorder()

		router.ServeHTTP(w, req)

		var providersAfterDelete []primitive.Provider
		err = json.Unmarshal(w.Body.Bytes(), &providersAfterDelete)
		assert.NoError(t, err)
		assert.Len(t, providersAfterDelete, 0)
	})

	t.Run("agent creation", func(t *testing.T) {
		mockStore := &MockPrimitiveStore{}
		handler := &apiHandler{
			store:     mockStore,
			validator: validation.NewValidator(),
		}

		router := mux.NewRouter()
		// Agent endpoints
		router.HandleFunc("/api/v1/agents", handler.listAgentsHandler).Methods("GET")
		router.HandleFunc("/api/v1/agents", handler.createAgentHandler).Methods("POST")

		// First create a provider
		provider := primitive.Provider{
			Name:       "Test Provider for Agent",
			APIBaseURL: "https://api.openai.com/v1",
			APIKeyEnc:  "test-api-key",
		}

		mockStore.Providers = append(mockStore.Providers, &provider)

		// Create agent
		agentReq := primitive.Agent{
			Name:         "Test Agent",
			Description:  "Test Agent Description",
			ProviderID:   provider.ID,
			ModelID:      "gpt-4",
			SystemPrompt: "You are a helpful assistant",
		}

		body, _ := json.Marshal(agentReq)
		req := httptest.NewRequest("POST", "/api/v1/agents", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var createdAgent primitive.Agent
		err := json.Unmarshal(w.Body.Bytes(), &createdAgent)
		assert.NoError(t, err)
		assert.NotEmpty(t, createdAgent.ID)
		assert.Equal(t, agentReq.Name, createdAgent.Name)

		// List agents
		req = httptest.NewRequest("GET", "/api/v1/agents", nil)
		w = httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var agents []primitive.Agent
		err = json.Unmarshal(w.Body.Bytes(), &agents)
		assert.NoError(t, err)
		assert.Len(t, agents, 1)
	})
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

// MockPrimitiveStore implements primitive.PrimitiveStore for testing
type MockPrimitiveStore struct {
	Agents        []*primitive.Agent
	Workflows     []*primitive.Workflow
	Providers     []*primitive.Provider
	Tools         []*primitive.Tool
	WorkflowSteps []*primitive.WorkflowStep
}

func (m *MockPrimitiveStore) CreateProvider(ctx context.Context, p *primitive.Provider) error {
	// Generate a simple ID for testing
	if p.ID == "" {
		p.ID = fmt.Sprintf("provider-%d", len(m.Providers)+1)
	}
	m.Providers = append(m.Providers, p)
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
	for i, p := range m.Providers {
		if p.ID == id {
			// Remove the provider from the slice
			m.Providers = append(m.Providers[:i], m.Providers[i+1:]...)
			return nil
		}
	}
	return primitive.ErrNotFound
}

func (m *MockPrimitiveStore) CreateTool(ctx context.Context, t *primitive.Tool) error {
	// Generate a simple ID for testing
	if t.ID == "" {
		t.ID = fmt.Sprintf("tool-%d", len(m.Tools)+1)
	}
	m.Tools = append(m.Tools, t)
	return nil
}

func (m *MockPrimitiveStore) GetTool(ctx context.Context, id string) (*primitive.Tool, error) {
	for _, t := range m.Tools {
		if t.ID == id {
			return t, nil
		}
	}
	return nil, primitive.ErrNotFound
}

func (m *MockPrimitiveStore) ListTools(ctx context.Context) ([]*primitive.Tool, error) {
	return m.Tools, nil
}

func (m *MockPrimitiveStore) UpdateTool(ctx context.Context, t *primitive.Tool) error {
	return nil
}

func (m *MockPrimitiveStore) DeleteTool(ctx context.Context, id string) error {
	for i, t := range m.Tools {
		if t.ID == id {
			// Remove the tool from the slice
			m.Tools = append(m.Tools[:i], m.Tools[i+1:]...)
			return nil
		}
	}
	return primitive.ErrNotFound
}

func (m *MockPrimitiveStore) CreateAgent(ctx context.Context, a *primitive.Agent) error {
	// Generate a simple ID for testing
	if a.ID == "" {
		a.ID = fmt.Sprintf("agent-%d", len(m.Agents)+1)
	}
	m.Agents = append(m.Agents, a)
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
	for i, a := range m.Agents {
		if a.ID == id {
			// Remove the agent from the slice
			m.Agents = append(m.Agents[:i], m.Agents[i+1:]...)
			return nil
		}
	}
	return primitive.ErrNotFound
}

func (m *MockPrimitiveStore) CreateWorkflow(ctx context.Context, w *primitive.Workflow) error {
	// Generate a simple ID for testing
	if w.ID == "" {
		w.ID = fmt.Sprintf("workflow-%d", len(m.Workflows)+1)
	}
	m.Workflows = append(m.Workflows, w)
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
	for i, w := range m.Workflows {
		if w.ID == id {
			// Remove the workflow from the slice
			m.Workflows = append(m.Workflows[:i], m.Workflows[i+1:]...)
			return nil
		}
	}
	return primitive.ErrNotFound
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

func (m *MockPrimitiveStore) GetMemoryConfig(ctx context.Context, id string) (*primitive.MemoryConfig, error) {
	// Return not found to prevent database connections in tests
	return nil, primitive.ErrNotFound
}

func (m *MockPrimitiveStore) UpdateMemoryConfig(ctx context.Context, config *primitive.MemoryConfig) error {
	// Mock implementation - just return nil for testing
	return nil
}

// MockWorkflowEngine implements agent.WorkflowEngine for testing
type MockWorkflowEngine struct{}

func (m *MockWorkflowEngine) SubmitJob(ctx context.Context, workflowID string, inputData map[string]interface{}) (*job.Job, error) {
	// Create a mock job and return it
	jobID := "test-job-id"
	now := time.Now()
	newJob := &job.Job{
		ID:         jobID,
		WorkflowID: workflowID,
		Status:     job.StatusQueued,
		InputData:  inputData,
		CreatedAt:  now,
	}
	return newJob, nil
}
