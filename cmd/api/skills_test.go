package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"

	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/internal/validation"
	dbmodels "github.com/mule-ai/mule/pkg/database"
)

// TestSkillStore implements primitive.PrimitiveStore for testing
type TestSkillStore struct {
	Skills      map[string]*primitive.Skill
	AgentSkills map[string][]string // agentID -> []skillID
	Agents      []*primitive.Agent
}

func NewTestSkillStore() *TestSkillStore {
	return &TestSkillStore{
		Skills:      make(map[string]*primitive.Skill),
		AgentSkills: make(map[string][]string),
		Agents:      make([]*primitive.Agent, 0),
	}
}

// Skill CRUD methods
func (s *TestSkillStore) CreateSkill(ctx context.Context, skill *primitive.Skill) error {
	if skill.ID == "" {
		skill.ID = "skill-" + skillUuidGen()
	}
	s.Skills[skill.ID] = skill
	return nil
}

func (s *TestSkillStore) GetSkill(ctx context.Context, id string) (*primitive.Skill, error) {
	if skill, ok := s.Skills[id]; ok {
		return skill, nil
	}
	return nil, primitive.ErrNotFound
}

func (s *TestSkillStore) ListSkills(ctx context.Context) ([]*primitive.Skill, error) {
	skills := make([]*primitive.Skill, 0, len(s.Skills))
	for _, skill := range s.Skills {
		skills = append(skills, skill)
	}
	return skills, nil
}

func (s *TestSkillStore) UpdateSkill(ctx context.Context, skill *primitive.Skill) error {
	if _, ok := s.Skills[skill.ID]; !ok {
		return primitive.ErrNotFound
	}
	s.Skills[skill.ID] = skill
	return nil
}

func (s *TestSkillStore) DeleteSkill(ctx context.Context, id string) error {
	if _, ok := s.Skills[id]; !ok {
		return primitive.ErrNotFound
	}
	delete(s.Skills, id)
	// Also remove from all agent assignments
	for agentID := range s.AgentSkills {
		skills := s.AgentSkills[agentID]
		for i, sid := range skills {
			if sid == id {
				s.AgentSkills[agentID] = append(skills[:i], skills[i+1:]...)
				break
			}
		}
	}
	return nil
}

// Agent skill association methods
func (s *TestSkillStore) GetAgentSkills(ctx context.Context, agentID string) ([]*primitive.Skill, error) {
	skillIDs, ok := s.AgentSkills[agentID]
	if !ok {
		return []*primitive.Skill{}, nil
	}
	skills := make([]*primitive.Skill, 0, len(skillIDs))
	for _, skillID := range skillIDs {
		if skill, ok := s.Skills[skillID]; ok {
			skills = append(skills, skill)
		}
	}
	return skills, nil
}

func (s *TestSkillStore) AssignSkillToAgent(ctx context.Context, agentID, skillID string) error {
	if _, ok := s.Skills[skillID]; !ok {
		return primitive.ErrNotFound
	}
	// Check agent exists
	found := false
	for _, a := range s.Agents {
		if a.ID == agentID {
			found = true
			break
		}
	}
	if !found {
		return primitive.ErrNotFound
	}
	s.AgentSkills[agentID] = append(s.AgentSkills[agentID], skillID)
	return nil
}

func (s *TestSkillStore) RemoveSkillFromAgent(ctx context.Context, agentID, skillID string) error {
	skills := s.AgentSkills[agentID]
	for i, sid := range skills {
		if sid == skillID {
			s.AgentSkills[agentID] = append(skills[:i], skills[i+1:]...)
			return nil
		}
	}
	return primitive.ErrNotFound
}

func (s *TestSkillStore) SetAgentSkills(ctx context.Context, agentID string, skillIDs []string) error {
	// Validate all skills exist
	for _, skillID := range skillIDs {
		if _, ok := s.Skills[skillID]; !ok {
			return primitive.ErrNotFound
		}
	}
	s.AgentSkills[agentID] = skillIDs
	return nil
}

// Stub methods for TestSkillStore
func (s *TestSkillStore) CreateProvider(ctx context.Context, p *primitive.Provider) error { return nil }
func (s *TestSkillStore) GetProvider(ctx context.Context, id string) (*primitive.Provider, error) {
	return nil, nil
}
func (s *TestSkillStore) ListProviders(ctx context.Context) ([]*primitive.Provider, error) {
	return nil, nil
}
func (s *TestSkillStore) UpdateProvider(ctx context.Context, p *primitive.Provider) error { return nil }
func (s *TestSkillStore) DeleteProvider(ctx context.Context, id string) error             { return nil }
func (s *TestSkillStore) CreateTool(ctx context.Context, t *primitive.Tool) error         { return nil }
func (s *TestSkillStore) GetTool(ctx context.Context, id string) (*primitive.Tool, error) {
	return nil, nil
}
func (s *TestSkillStore) ListTools(ctx context.Context) ([]*primitive.Tool, error) { return nil, nil }
func (s *TestSkillStore) UpdateTool(ctx context.Context, t *primitive.Tool) error  { return nil }
func (s *TestSkillStore) DeleteTool(ctx context.Context, id string) error          { return nil }
func (s *TestSkillStore) GetAgentTools(ctx context.Context, agentID string) ([]*primitive.Tool, error) {
	return nil, nil
}
func (s *TestSkillStore) AssignToolToAgent(ctx context.Context, agentID, toolID string) error {
	return nil
}
func (s *TestSkillStore) RemoveToolFromAgent(ctx context.Context, agentID, toolID string) error {
	return nil
}
func (s *TestSkillStore) CreateAgent(ctx context.Context, a *primitive.Agent) error { return nil }
func (s *TestSkillStore) GetAgent(ctx context.Context, id string) (*primitive.Agent, error) {
	return nil, nil
}
func (s *TestSkillStore) ListAgents(ctx context.Context) ([]*primitive.Agent, error)      { return nil, nil }
func (s *TestSkillStore) UpdateAgent(ctx context.Context, a *primitive.Agent) error       { return nil }
func (s *TestSkillStore) DeleteAgent(ctx context.Context, id string) error                { return nil }
func (s *TestSkillStore) CreateWorkflow(ctx context.Context, w *primitive.Workflow) error { return nil }
func (s *TestSkillStore) GetWorkflow(ctx context.Context, id string) (*primitive.Workflow, error) {
	return nil, nil
}
func (s *TestSkillStore) ListWorkflows(ctx context.Context) ([]*primitive.Workflow, error) {
	return nil, nil
}
func (s *TestSkillStore) UpdateWorkflow(ctx context.Context, w *primitive.Workflow) error { return nil }
func (s *TestSkillStore) DeleteWorkflow(ctx context.Context, id string) error             { return nil }
func (s *TestSkillStore) CreateWorkflowStep(ctx context.Context, ws *primitive.WorkflowStep) error {
	return nil
}
func (s *TestSkillStore) GetWorkflowStep(ctx context.Context, id string) (*primitive.WorkflowStep, error) {
	return nil, nil
}
func (s *TestSkillStore) ListWorkflowSteps(ctx context.Context, workflowID string) ([]*primitive.WorkflowStep, error) {
	return nil, nil
}
func (s *TestSkillStore) UpdateWorkflowStep(ctx context.Context, ws *primitive.WorkflowStep) error {
	return nil
}
func (s *TestSkillStore) DeleteWorkflowStep(ctx context.Context, id string) error         { return nil }
func (s *TestSkillStore) CreateSetting(ctx context.Context, set *primitive.Setting) error { return nil }
func (s *TestSkillStore) GetSetting(ctx context.Context, key string) (*primitive.Setting, error) {
	return nil, nil
}
func (s *TestSkillStore) ListSettings(ctx context.Context) ([]*primitive.Setting, error) {
	return nil, nil
}
func (s *TestSkillStore) UpdateSetting(ctx context.Context, set *primitive.Setting) error { return nil }
func (s *TestSkillStore) DeleteSetting(ctx context.Context, key string) error             { return nil }
func (s *TestSkillStore) CreateWasmModule(ctx context.Context, w *primitive.WasmModule) error {
	return nil
}
func (s *TestSkillStore) GetWasmModule(ctx context.Context, id string) (*primitive.WasmModule, error) {
	return nil, nil
}
func (s *TestSkillStore) ListWasmModules(ctx context.Context) ([]*primitive.WasmModuleListItem, error) {
	return nil, nil
}
func (s *TestSkillStore) UpdateWasmModule(ctx context.Context, w *primitive.WasmModule) error {
	return nil
}
func (s *TestSkillStore) DeleteWasmModule(ctx context.Context, id string) error { return nil }
func (s *TestSkillStore) GetMemoryConfig(ctx context.Context, id string) (*primitive.MemoryConfig, error) {
	return nil, nil
}
func (s *TestSkillStore) UpdateMemoryConfig(ctx context.Context, config *primitive.MemoryConfig) error {
	return nil
}

// Job methods - needed for store interface
func (s *TestSkillStore) GetJob(ctx context.Context, id string) (*dbmodels.Job, error) {
	return nil, nil
}
func (s *TestSkillStore) ListJobs(ctx context.Context) ([]*dbmodels.Job, error) { return nil, nil }
func (s *TestSkillStore) CreateJob(ctx context.Context, j *dbmodels.Job) error  { return nil }
func (s *TestSkillStore) UpdateJob(ctx context.Context, j *dbmodels.Job) error  { return nil }

// UUID generator helper (simple counter for tests)
var skillTestUuidCounter = 0

func skillUuidGen() string {
	skillTestUuidCounter++
	return fmt.Sprintf("test-%d", skillTestUuidCounter)
}

// TestableSkillHandler is a version of apiHandler that allows injecting skill operations
type TestableSkillHandler struct {
	store     primitive.PrimitiveStore
	validator *validation.Validator
}

// listSkillsHandler tests listing skills
func (h *TestableSkillHandler) listSkillsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	skills, err := h.store.ListSkills(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to list skills: %v", err), http.StatusInternalServerError)
		return
	}
	if skills == nil {
		skills = make([]*primitive.Skill, 0)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{"data": skills}); err != nil {
		log.Printf("failed to encode skills response: %v", err)
	}
}

// createSkillHandler tests creating a skill
func (h *TestableSkillHandler) createSkillHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Path        string `json:"path"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	skill := &primitive.Skill{
		ID:          "skill-" + skillUuidGen(),
		Name:        req.Name,
		Description: req.Description,
		Path:        req.Path,
		Enabled:     enabled,
	}
	if err := h.store.CreateSkill(ctx, skill); err != nil {
		http.Error(w, fmt.Sprintf("failed to create skill: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(skill); err != nil {
		log.Printf("failed to encode skill response: %v", err)
	}
}

// getSkillHandler tests getting a skill
func (h *TestableSkillHandler) getSkillHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]
	skill, err := h.store.GetSkill(ctx, id)
	if err != nil {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(skill); err != nil {
		log.Printf("failed to encode skill response: %v", err)
	}
}

// updateSkillHandler tests updating a skill
func (h *TestableSkillHandler) updateSkillHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Path        string `json:"path"`
		Enabled     *bool  `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}

	skill, err := h.store.GetSkill(ctx, id)
	if err != nil {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}

	skill.Name = req.Name
	skill.Description = req.Description
	skill.Path = req.Path
	if req.Enabled != nil {
		skill.Enabled = *req.Enabled
	}

	if err := h.store.UpdateSkill(ctx, skill); err != nil {
		http.Error(w, fmt.Sprintf("failed to update skill: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(skill); err != nil {
		log.Printf("failed to encode skill response: %v", err)
	}
}

// deleteSkillHandler tests deleting a skill
func (h *TestableSkillHandler) deleteSkillHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	if err := h.store.DeleteSkill(ctx, id); err != nil {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getAgentSkillsHandler tests getting skills for an agent
func (h *TestableSkillHandler) getAgentSkillsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	agentID := vars["id"]

	skills, err := h.store.GetAgentSkills(ctx, agentID)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get agent skills: %v", err), http.StatusInternalServerError)
		return
	}
	if skills == nil {
		skills = make([]*primitive.Skill, 0)
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{"data": skills}); err != nil {
		log.Printf("failed to encode skills response: %v", err)
	}
}

// assignSkillsToAgentHandler tests assigning skills to an agent
func (h *TestableSkillHandler) assignSkillsToAgentHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	agentID := vars["id"]

	var req struct {
		SkillIDs []string `json:"skill_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate skill IDs exist
	for _, skillID := range req.SkillIDs {
		_, err := h.store.GetSkill(ctx, skillID)
		if err != nil {
			http.Error(w, fmt.Sprintf("skill not found: %s", skillID), http.StatusNotFound)
			return
		}
	}

	// Remove existing and add new skills
	for _, skillID := range req.SkillIDs {
		if err := h.store.AssignSkillToAgent(ctx, agentID, skillID); err != nil {
			http.Error(w, fmt.Sprintf("failed to assign skill: %v", err), http.StatusInternalServerError)
			return
		}
	}

	skills, _ := h.store.GetAgentSkills(ctx, agentID)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{"data": skills}); err != nil {
		log.Printf("failed to encode skills response: %v", err)
	}
}

// removeSkillFromAgentHandler tests removing a skill from an agent
func (h *TestableSkillHandler) removeSkillFromAgentHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	agentID := vars["id"]
	skillID := vars["skillId"]

	if err := h.store.RemoveSkillFromAgent(ctx, agentID, skillID); err != nil {
		http.Error(w, "skill not found on agent", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func TestSkillsAPI(t *testing.T) {
	mockStore := NewTestSkillStore()
	validator := validation.NewValidator()

	handler := &TestableSkillHandler{
		store:     mockStore,
		validator: validator,
	}

	router := mux.NewRouter()
	router.HandleFunc("/api/v1/skills", handler.listSkillsHandler).Methods("GET")
	router.HandleFunc("/api/v1/skills", handler.createSkillHandler).Methods("POST")
	router.HandleFunc("/api/v1/skills/{id}", handler.getSkillHandler).Methods("GET")
	router.HandleFunc("/api/v1/skills/{id}", handler.updateSkillHandler).Methods("PUT")
	router.HandleFunc("/api/v1/skills/{id}", handler.deleteSkillHandler).Methods("DELETE")

	t.Run("list skills - empty", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/skills", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)

		// Should return data array (empty)
		data, ok := response["data"]
		assert.True(t, ok, "Response should have 'data' field")
		skills, ok := data.([]interface{})
		assert.True(t, ok, "data should be an array")
		assert.Len(t, skills, 0)
	})

	t.Run("create skill - success", func(t *testing.T) {
		skillReq := map[string]interface{}{
			"name":        "test-skill",
			"description": "A test skill",
			"path":        "/path/to/skill",
			"enabled":     true,
		}

		body, _ := json.Marshal(skillReq)
		req := httptest.NewRequest("POST", "/api/v1/skills", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusCreated, w.Code)

		var response primitive.Skill
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "test-skill", response.Name)
		assert.Equal(t, "A test skill", response.Description)
		assert.Equal(t, "/path/to/skill", response.Path)
		assert.True(t, response.Enabled)
		assert.NotEmpty(t, response.ID)
	})

	t.Run("create skill - missing name", func(t *testing.T) {
		skillReq := map[string]interface{}{
			"path": "/path/to/skill",
		}

		body, _ := json.Marshal(skillReq)
		req := httptest.NewRequest("POST", "/api/v1/skills", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("create skill - missing path", func(t *testing.T) {
		skillReq := map[string]interface{}{
			"name": "test-skill",
		}

		body, _ := json.Marshal(skillReq)
		req := httptest.NewRequest("POST", "/api/v1/skills", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("list skills - with data", func(t *testing.T) {
		// Create a fresh store for this test to avoid interference from other tests
		localStore := NewTestSkillStore()

		// Create a skill first
		skill := &primitive.Skill{
			ID:          "skill-1",
			Name:        "existing-skill",
			Description: "An existing skill",
			Path:        "/path/to/existing",
			Enabled:     true,
		}
		if err := localStore.CreateSkill(context.Background(), skill); err != nil {
			t.Fatalf("failed to create skill: %v", err)
		}

		// Create a local handler with this store
		localHandler := &TestableSkillHandler{
			store:     localStore,
			validator: validation.NewValidator(),
		}

		localRouter := mux.NewRouter()
		localRouter.HandleFunc("/api/v1/skills", localHandler.listSkillsHandler).Methods("GET")

		req := httptest.NewRequest("GET", "/api/v1/skills", nil)
		w := httptest.NewRecorder()

		localRouter.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)

		data := response["data"].([]interface{})
		assert.Len(t, data, 1)
	})

	t.Run("get skill - success", func(t *testing.T) {
		skill := &primitive.Skill{
			ID:          "skill-get-test",
			Name:        "get-test-skill",
			Description: "A skill for get testing",
			Path:        "/path/to/get",
			Enabled:     true,
		}
		if err := mockStore.CreateSkill(context.Background(), skill); err != nil {
			t.Fatalf("failed to create skill: %v", err)
		}

		req := httptest.NewRequest("GET", "/api/v1/skills/skill-get-test", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response primitive.Skill
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "get-test-skill", response.Name)
	})

	t.Run("get skill - not found", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/skills/nonexistent", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("update skill - success", func(t *testing.T) {
		skill := &primitive.Skill{
			ID:          "skill-update-test",
			Name:        "update-test-skill",
			Description: "Original description",
			Path:        "/original/path",
			Enabled:     true,
		}
		if err := mockStore.CreateSkill(context.Background(), skill); err != nil {
			t.Fatalf("failed to create skill: %v", err)
		}

		updateReq := map[string]interface{}{
			"name":        "updated-skill-name",
			"description": "Updated description",
			"path":        "/new/path",
			"enabled":     false,
		}

		body, _ := json.Marshal(updateReq)
		req := httptest.NewRequest("PUT", "/api/v1/skills/skill-update-test", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response primitive.Skill
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)
		assert.Equal(t, "updated-skill-name", response.Name)
		assert.Equal(t, "Updated description", response.Description)
		assert.Equal(t, "/new/path", response.Path)
		assert.False(t, response.Enabled)
	})

	t.Run("update skill - not found", func(t *testing.T) {
		updateReq := map[string]interface{}{
			"name": "nonexistent-skill",
			"path": "/some/path",
		}

		body, _ := json.Marshal(updateReq)
		req := httptest.NewRequest("PUT", "/api/v1/skills/nonexistent", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("delete skill - success", func(t *testing.T) {
		skill := &primitive.Skill{
			ID:   "skill-delete-test",
			Name: "delete-test-skill",
			Path: "/path/to/delete",
		}
		if err := mockStore.CreateSkill(context.Background(), skill); err != nil {
			t.Fatalf("failed to create skill: %v", err)
		}

		req := httptest.NewRequest("DELETE", "/api/v1/skills/skill-delete-test", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)

		// Verify it's deleted
		_, err := mockStore.GetSkill(context.Background(), "skill-delete-test")
		assert.Error(t, err)
	})

	t.Run("delete skill - not found", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/v1/skills/nonexistent", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestAgentSkillsAPI(t *testing.T) {
	mockStore := NewTestSkillStore()

	// Create an agent
	agent := &primitive.Agent{
		ID:           "agent-test-1",
		Name:         "test-agent",
		ProviderID:   "provider-1",
		ModelID:      "test-model",
		SystemPrompt: "You are helpful",
	}
	mockStore.Agents = append(mockStore.Agents, agent)

	// Create some skills
	skill1 := &primitive.Skill{
		ID:   "skill-1",
		Name: "skill-one",
		Path: "/path/to/skill1",
	}
	skill2 := &primitive.Skill{
		ID:   "skill-2",
		Name: "skill-two",
		Path: "/path/to/skill2",
	}
	if err := mockStore.CreateSkill(context.Background(), skill1); err != nil {
		t.Fatalf("failed to create skill: %v", err)
	}
	if err := mockStore.CreateSkill(context.Background(), skill2); err != nil {
		t.Fatalf("failed to create skill: %v", err)
	}

	validator := validation.NewValidator()

	handler := &TestableSkillHandler{
		store:     mockStore,
		validator: validator,
	}

	router := mux.NewRouter()
	router.HandleFunc("/api/v1/agents/{id}/skills", handler.getAgentSkillsHandler).Methods("GET")
	router.HandleFunc("/api/v1/agents/{id}/skills", handler.assignSkillsToAgentHandler).Methods("PUT")
	router.HandleFunc("/api/v1/agents/{id}/skills/{skillId}", handler.removeSkillFromAgentHandler).Methods("DELETE")

	t.Run("get agent skills - empty", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agents/agent-test-1/skills", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)

		data := response["data"].([]interface{})
		assert.Len(t, data, 0)
	})

	t.Run("assign skills to agent", func(t *testing.T) {
		assignReq := map[string]interface{}{
			"skill_ids": []string{"skill-1", "skill-2"},
		}

		body, _ := json.Marshal(assignReq)
		req := httptest.NewRequest("PUT", "/api/v1/agents/agent-test-1/skills", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)

		data := response["data"].([]interface{})
		assert.Len(t, data, 2)
	})

	t.Run("get agent skills - with skills", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/agents/agent-test-1/skills", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		assert.NoError(t, err)

		data := response["data"].([]interface{})
		assert.Len(t, data, 2)
	})

	t.Run("assign skills - invalid skill id", func(t *testing.T) {
		assignReq := map[string]interface{}{
			"skill_ids": []string{"invalid-skill"},
		}

		body, _ := json.Marshal(assignReq)
		req := httptest.NewRequest("PUT", "/api/v1/agents/agent-test-1/skills", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("remove skill from agent", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/v1/agents/agent-test-1/skills/skill-1", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNoContent, w.Code)

		// Verify skill is removed
		skills, _ := mockStore.GetAgentSkills(context.Background(), "agent-test-1")
		assert.Len(t, skills, 1)
		assert.Equal(t, "skill-2", skills[0].ID)
	})

	t.Run("remove skill - not found", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/v1/agents/agent-test-1/skills/nonexistent", nil)
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// Keep time import used
var _ = time.Time{}
