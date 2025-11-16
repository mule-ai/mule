package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"

	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/internal/validation"
)

func TestAPIEndpoints(t *testing.T) {
	// Create a mock store to test the API handlers
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
	}

	handler := &apiHandler{
		store:     mockStore,
		validator: validation.NewValidator(),
	}

	router := mux.NewRouter()
	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})
	router.HandleFunc("/v1/models", handler.modelsHandler).Methods("GET")

	t.Run("health endpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "OK", w.Body.String())
	})

	t.Run("models endpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/v1/models", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response struct {
			Data []struct {
				ID      string `json:"id"`
				Object  string `json:"object"`
				OwnedBy string `json:"owned_by"`
			} `json:"data"`
		}

		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Len(t, response.Data, 2) // One agent + one workflow

		// Check agent model
		agentModel := response.Data[0]
		assert.Equal(t, "agent/test-agent", agentModel.ID)
		assert.Equal(t, "model", agentModel.Object)
		assert.Equal(t, "mule", agentModel.OwnedBy)

		// Check workflow model
		workflowModel := response.Data[1]
		assert.Equal(t, "workflow/test-workflow", workflowModel.ID)
		assert.Equal(t, "model", workflowModel.Object)
		assert.Equal(t, "mule", workflowModel.OwnedBy)
	})
}

func TestPrimitiveManagementEndpoints(t *testing.T) {
	mockStore := &MockPrimitiveStore{}

	handler := &apiHandler{
		store:     mockStore,
		validator: validation.NewValidator(),
	}

	router := mux.NewRouter()

	// Provider endpoints
	router.HandleFunc("/api/v1/providers", handler.listProvidersHandler).Methods("GET")
	router.HandleFunc("/api/v1/providers", handler.createProviderHandler).Methods("POST")

	t.Run("list providers empty", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/providers", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response []primitive.Provider
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Len(t, response, 0)
	})

	t.Run("create provider", func(t *testing.T) {
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

		var response primitive.Provider
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, provider.Name, response.Name)
		assert.Equal(t, provider.APIBaseURL, response.APIBaseURL)
		assert.NotEmpty(t, response.ID)
	})
}

// MockPrimitiveStore implements primitive.PrimitiveStore for testing
type MockPrimitiveStore struct {
	Agents    []*primitive.Agent
	Workflows []*primitive.Workflow
	Providers []*primitive.Provider
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
	return nil, nil
}
