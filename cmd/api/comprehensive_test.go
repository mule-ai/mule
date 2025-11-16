package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
				APIBaseURL: "https://api.test.com",
				APIKeyEnc:  "test-api-key",
			},
		},
	}

	mockJobStore := &MockJobStore{Jobs: make(map[string]*job.Job)}

	handler := &apiHandler{
		store:     mockStore,
		runtime:   agent.NewRuntime(mockStore),
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
			"model": "workflow/test-workflow",
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
		assert.Contains(t, errorResponse["message"], "model must start with 'agent/' or 'workflow/'")
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

	// Agent endpoints
	router.HandleFunc("/api/v1/agents", handler.listAgentsHandler).Methods("GET")
	router.HandleFunc("/api/v1/agents", handler.createAgentHandler).Methods("POST")

	t.Run("provider CRUD operations", func(t *testing.T) {
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
