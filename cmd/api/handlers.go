package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/mule-ai/mule/internal/agent"
	"github.com/mule-ai/mule/internal/api"
	internaldb "github.com/mule-ai/mule/internal/database"
	"github.com/mule-ai/mule/internal/engine"
	"github.com/mule-ai/mule/internal/manager"
	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/internal/validation"
	dbmodels "github.com/mule-ai/mule/pkg/database"
	"github.com/mule-ai/mule/pkg/job"
)

type apiHandler struct {
	db             *internaldb.DB
	store          primitive.PrimitiveStore
	runtime        *agent.Runtime
	jobStore       job.JobStore
	validator      *validation.Validator
	wasmModuleMgr  *manager.WasmModuleManager
	wasmExecutor   *engine.WASMExecutor
	workflowEngine *engine.Engine
	workflowMgr    *manager.WorkflowManager
	skillMgr       *manager.SkillManager
}

func NewAPIHandler(db *internaldb.DB) *apiHandler {
	store := primitive.NewPGStore(db.DB) // Access the underlying *sql.DB
	jobStore := job.NewPGStore(db.DB)    // Access the underlying *sql.DB
	validator := validation.NewValidator()
	workflowMgr := manager.NewWorkflowManager(db)

	// Create agent runtime (without workflow engine initially)
	runtime := agent.NewRuntime(store, jobStore)

	// Create WASM executor (will be updated with workflow engine after engine creation)
	wasmExecutor := engine.NewWASMExecutor(db.DB, store, runtime, nil)

	// Create WASM module manager with WASM executor reference for cache invalidation
	wasmModuleMgr := manager.NewWasmModuleManager(store, wasmExecutor)

	// Create workflow engine
	workflowEngine := engine.NewEngine(store, jobStore, runtime, wasmExecutor, engine.Config{
		Workers: 5, // Default to 5 workers
	})

	// Update WASM executor with workflow engine
	// This is a bit of a hack, but it works because we're updating the same instance
	wasmExecutor.WorkflowEngine = workflowEngine

	// Set workflow engine on runtime (requires a setter method)
	runtime.SetWorkflowEngine(workflowEngine)

	// Create skill manager
	skillMgr := manager.NewSkillManager(db)

	return &apiHandler{
		db:             db,
		store:          store,
		runtime:        runtime,
		jobStore:       jobStore,
		validator:      validator,
		wasmModuleMgr:  wasmModuleMgr,
		wasmExecutor:   wasmExecutor,
		workflowEngine: workflowEngine,
		workflowMgr:    workflowMgr,
		skillMgr:       skillMgr,
	}
}

func (h *apiHandler) modelsHandler(w http.ResponseWriter, r *http.Request) {
	agents, err := h.store.ListAgents(r.Context())
	if err != nil {
		api.HandleError(w, err, http.StatusInternalServerError)
		return
	}
	workflows, err := h.store.ListWorkflows(r.Context())
	if err != nil {
		api.HandleError(w, err, http.StatusInternalServerError)
		return
	}

	types := []map[string]string{}
	for _, a := range agents {
		types = append(types, map[string]string{
			"id":       "agent/" + strings.ToLower(a.Name),
			"object":   "model",
			"owned_by": "mule",
		})
	}
	for _, w := range workflows {
		// Always list sync workflow endpoint
		types = append(types, map[string]string{
			"id":       "workflow/" + strings.ToLower(w.Name),
			"object":   "model",
			"owned_by": "mule",
		})
		// Also list async workflow endpoint if is_async is true
		if w.IsAsync {
			types = append(types, map[string]string{
				"id":       "async/workflow/" + strings.ToLower(w.Name),
				"object":   "model",
				"owned_by": "mule",
			})
		}
	}

	resp := map[string]interface{}{
		"data": types,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// chatCompletionsHandler handles OpenAI-compatible chat completions API requests.
// Supports agent execution (model starting with "agent/") and workflow execution
// (model starting with "workflow/" or "async/workflow/").
//
// Request body: ChatCompletionRequest with model and messages
// Response: ChatCompletionResponse for sync execution, AsyncJobResponse for async
func (h *apiHandler) chatCompletionsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse request
	var req agent.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.HandleError(w, fmt.Errorf("failed to decode request: %w", err), http.StatusBadRequest)
		return
	}

	// Validate request
	if req.Model == "" {
		api.HandleError(w, fmt.Errorf("model is required"), http.StatusBadRequest)
		return
	}

	if len(req.Messages) == 0 {
		api.HandleError(w, fmt.Errorf("at least one message is required"), http.StatusBadRequest)
		return
	}

	// Determine if this is an agent or workflow execution
	if strings.HasPrefix(req.Model, "agent/") {
		// Execute agent
		resp, err := h.runtime.ExecuteAgentWithWorkingDir(ctx, &req, req.WorkingDirectory)
		if err != nil {
			api.HandleError(w, fmt.Errorf("failed to execute agent: %w", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	} else if strings.HasPrefix(req.Model, "async/workflow/") {
		// Async workflow execution - submit job and return immediately
		newJob, err := h.runtime.ExecuteWorkflowWithWorkingDir(ctx, &req, req.WorkingDirectory)
		if err != nil {
			api.HandleError(w, fmt.Errorf("failed to execute workflow: %w", err), http.StatusInternalServerError)
			return
		}

		// Return job info immediately for async execution
		resp := &agent.AsyncJobResponse{
			ID:      newJob.ID,
			Object:  "async.job",
			Status:  string(newJob.Status),
			Message: "The workflow has been started",
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	} else if strings.HasPrefix(req.Model, "workflow/") {
		// Workflow execution - check if workflow is async
		workflowName := strings.TrimPrefix(req.Model, "workflow/")

		// Find the workflow to check if it's async
		workflows, err := h.store.ListWorkflows(ctx)
		if err != nil {
			api.HandleError(w, fmt.Errorf("failed to list workflows: %w", err), http.StatusInternalServerError)
			return
		}

		var targetWorkflow *primitive.Workflow
		for _, wf := range workflows {
			if strings.ToLower(wf.Name) == workflowName {
				targetWorkflow = wf
				break
			}
		}

		if targetWorkflow == nil {
			api.HandleError(w, fmt.Errorf("workflow '%s' not found", workflowName), http.StatusNotFound)
			return
		}

		// If the workflow is marked as async, execute asynchronously regardless of model prefix
		if targetWorkflow.IsAsync {
			newJob, err := h.runtime.ExecuteWorkflowWithWorkingDir(ctx, &req, req.WorkingDirectory)
			if err != nil {
				api.HandleError(w, fmt.Errorf("failed to execute workflow: %w", err), http.StatusInternalServerError)
				return
			}

			// Return job info immediately for async execution
			resp := &agent.AsyncJobResponse{
				ID:      newJob.ID,
				Object:  "async.job",
				Status:  string(newJob.Status),
				Message: "The workflow has been started",
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		// Sync workflow execution - wait for completion and return ChatCompletionResponse
		newJob, err := h.runtime.ExecuteWorkflowWithWorkingDir(ctx, &req, req.WorkingDirectory)
		if err != nil {
			api.HandleError(w, fmt.Errorf("failed to execute workflow: %w", err), http.StatusInternalServerError)
			return
		}

		// Get workflow timeout from database
		workflowTimeout := 5 * time.Minute // Default timeout
		if setting, err := h.store.GetSetting(r.Context(), "timeout_workflow_seconds"); err == nil {
			if timeoutSeconds, err := strconv.Atoi(setting.Value); err == nil && timeoutSeconds > 0 {
				workflowTimeout = time.Duration(timeoutSeconds) * time.Second
			}
		}

		// Wait for job completion with timeout
		ctx, cancel := context.WithTimeout(ctx, workflowTimeout)
		defer cancel()

		for {
			select {
			case <-ctx.Done():
				api.HandleError(w, fmt.Errorf("workflow execution timed out"), http.StatusInternalServerError)
				return
			case <-time.After(500 * time.Millisecond):
				// Check job status
				updatedJob, err := h.jobStore.GetJob(newJob.ID)
				if err != nil {
					api.HandleError(w, fmt.Errorf("failed to get job status: %w", err), http.StatusInternalServerError)
					return
				}

				switch updatedJob.Status {
				case job.StatusCompleted:
					// Extract response and usage from output_data
					responseText := ""
					if resp, exists := updatedJob.OutputData["prompt"]; exists {
						responseText = fmt.Sprintf("%v", resp)
					}

					// Extract usage if available
					usage := agent.ChatCompletionUsage{
						PromptTokens:     0,
						CompletionTokens: 0,
						TotalTokens:      0,
					}

					if usageData, exists := updatedJob.OutputData["usage"]; exists {
						if usageMap, ok := usageData.(map[string]interface{}); ok {
							if promptTokens, ok := usageMap["prompt_tokens"].(float64); ok {
								usage.PromptTokens = int(promptTokens)
							}
							if completionTokens, ok := usageMap["completion_tokens"].(float64); ok {
								usage.CompletionTokens = int(completionTokens)
							}
							if totalTokens, ok := usageMap["total_tokens"].(float64); ok {
								usage.TotalTokens = int(totalTokens)
							}
						}
					}

					// Return OpenAI-compatible completion response
					resp := &agent.ChatCompletionResponse{
						ID:      fmt.Sprintf("chatcmpl-%s", updatedJob.ID),
						Object:  "chat.completion",
						Created: time.Now().Unix(),
						Model:   req.Model,
						Choices: []agent.ChatCompletionChoice{
							{
								Index: 0,
								Message: agent.ChatCompletionMessage{
									Role:    "assistant",
									Content: responseText,
								},
								FinishReason: "stop",
							},
						},
						Usage: usage,
					}

					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(resp)
					return
				case job.StatusFailed:
					// Extract error from output_data
					errorMsg := "unknown error"
					if errData, exists := updatedJob.OutputData["error"]; exists {
						errorMsg = fmt.Sprintf("%v", errData)
					}
					api.HandleError(w, fmt.Errorf("workflow execution failed: %s", errorMsg), http.StatusInternalServerError)
					return
				case job.StatusRunning, job.StatusQueued:
					// Continue waiting
					continue
				}
			}
		}
	} else {
		api.HandleError(w, fmt.Errorf("model must start with 'agent/', 'workflow/', or 'async/workflow/'"), http.StatusBadRequest)
		return
	}
}

// Provider handlers

// listProvidersHandler returns all configured AI providers.
// GET /api/v1/providers
// Response: Array of Provider objects
func (h *apiHandler) listProvidersHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	providers, err := h.store.ListProviders(ctx)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to list providers: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(providers)
}

// createProviderHandler creates a new AI provider configuration.
// POST /api/v1/providers
// Request body: Provider object with name, type, api_base_url, api_key
// Response: Created Provider object with generated ID
func (h *apiHandler) createProviderHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var provider primitive.Provider
	if err := json.NewDecoder(r.Body).Decode(&provider); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}

	// Validate provider fields
	if errors := h.validator.ValidateProvider(&provider); len(errors) > 0 {
		api.HandleError(w, fmt.Errorf("%s", errors.Error()), http.StatusBadRequest)
		return
	}

	if err := h.store.CreateProvider(ctx, &provider); err != nil {
		// Check if it's a unique constraint violation
		if strings.Contains(err.Error(), "duplicate key value violates unique constraint") {
			api.HandleError(w, fmt.Errorf("a provider with this name already exists"), http.StatusConflict)
		} else {
			api.HandleError(w, fmt.Errorf("failed to create provider: %w", err), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(provider)
}

// getProviderHandler retrieves a provider by ID.
// GET /api/v1/providers/{id}
// Response: Provider object
func (h *apiHandler) getProviderHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	provider, err := h.store.GetProvider(ctx, id)
	if err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("provider not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to get provider: %w", err), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(provider)
}

// updateProviderHandler updates an existing provider.
// PUT /api/v1/providers/{id}
// Request body: Provider object with updated fields
// Response: Updated Provider object
func (h *apiHandler) updateProviderHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	var provider primitive.Provider
	if err := json.NewDecoder(r.Body).Decode(&provider); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}
	provider.ID = id

	if err := h.store.UpdateProvider(ctx, &provider); err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("provider not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to update provider: %w", err), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(provider)
}

// deleteProviderHandler removes a provider by ID.
// DELETE /api/v1/providers/{id}
// Response: 204 No Content on success
func (h *apiHandler) deleteProviderHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	if err := h.store.DeleteProvider(ctx, id); err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("provider not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to delete provider: %w", err), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getProviderModelsHandler retrieves available models for a provider using pi --list-models.
// GET /api/v1/providers/{id}/models
// Response: Object with data array containing model {id, name} objects
func (h *apiHandler) getProviderModelsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	// Get the provider
	provider, err := h.store.GetProvider(ctx, id)
	if err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("provider not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to get provider: %w", err), http.StatusInternalServerError)
		}
		return
	}

	// Use pi --list-models to get available models for this provider
	cmd := exec.CommandContext(ctx, "pi", "--list-models", provider.Name)
	output, err := cmd.Output()
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to list models: %w", err), http.StatusInternalServerError)
		return
	}

	// Parse the output into a models list
	// Output format is:
	// provider        model                          context  max-out  thinking  images
	// local-llm       llamacpp/qwen3-30b-a3b         40K      32K      yes       no

	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		// No models found
		resp := map[string]interface{}{
			"data": []map[string]string{},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	// Skip header line and parse model lines
	var models []map[string]string
	for i := 1; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		// Parse the line - it has fixed-width columns
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			// First field is provider, second is model
			modelProvider := fields[0]
			modelID := fields[1]

			// Only include models from this provider
			if modelProvider == provider.Name {
				models = append(models, map[string]string{
					"id":   modelID,
					"name": modelID,
				})
			}
		}
	}

	resp := map[string]interface{}{
		"data": models,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// Tool handlers

// listToolsHandler returns all available tools.
// GET /api/v1/tools
// Response: Array of Tool objects
func (h *apiHandler) listToolsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tools, err := h.store.ListTools(ctx)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to list tools: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tools)
}

// createToolHandler creates a new tool.
// POST /api/v1/tools
// Request body: Tool object with name, description, and config
// Response: Created Tool object with generated ID
func (h *apiHandler) createToolHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var tool primitive.Tool
	if err := json.NewDecoder(r.Body).Decode(&tool); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}

	// Validate tool fields
	if validationErrors := h.validator.ValidateTool(&tool); len(validationErrors) > 0 {
		api.HandleError(w, fmt.Errorf("%s", validationErrors.Error()), http.StatusBadRequest)
		return
	}

	if err := h.store.CreateTool(ctx, &tool); err != nil {
		api.HandleError(w, fmt.Errorf("failed to create tool: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(tool)
}

// getToolHandler retrieves a tool by ID.
// GET /api/v1/tools/{id}
// Response: Tool object
func (h *apiHandler) getToolHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	tool, err := h.store.GetTool(ctx, id)
	if err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("tool not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to get tool: %w", err), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tool)
}

// updateToolHandler updates an existing tool.
// PUT /api/v1/tools/{id}
// Request body: Tool object with updated fields
// Response: Updated Tool object
func (h *apiHandler) updateToolHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	var tool primitive.Tool
	if err := json.NewDecoder(r.Body).Decode(&tool); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}
	tool.ID = id

	if err := h.store.UpdateTool(ctx, &tool); err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("tool not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to update tool: %w", err), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tool)
}

// deleteToolHandler removes a tool by ID.
// DELETE /api/v1/tools/{id}
// Response: 204 No Content on success
func (h *apiHandler) deleteToolHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	if err := h.store.DeleteTool(ctx, id); err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("tool not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to delete tool: %w", err), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Skill handlers

// listSkillsHandler returns all configured skills.
// GET /api/v1/skills
// Response: Object with data array containing Skill objects
func (h *apiHandler) listSkillsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	skills, err := h.skillMgr.ListSkills(ctx)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to list skills: %w", err), http.StatusInternalServerError)
		return
	}

	// Ensure we return an empty array instead of null when there are no skills
	if skills == nil {
		skills = make([]*dbmodels.Skill, 0)
	}

	resp := map[string]interface{}{
		"data": skills,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// createSkillHandler creates a new skill for pi agents.
// POST /api/v1/skills
// Request body: {name, description, path, enabled}
// Response: Created Skill object with generated ID
func (h *apiHandler) createSkillHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Path        string `json:"path"`
		Enabled     bool   `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" {
		api.HandleError(w, fmt.Errorf("name is required"), http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		api.HandleError(w, fmt.Errorf("path is required"), http.StatusBadRequest)
		return
	}

	skill, err := h.skillMgr.CreateSkill(ctx, req.Name, req.Description, req.Path, req.Enabled)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to create skill: %w", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(skill)
}

// getSkillHandler retrieves a skill by ID.
// GET /api/v1/skills/{id}
// Response: Skill object
func (h *apiHandler) getSkillHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	skill, err := h.skillMgr.GetSkill(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			api.HandleError(w, fmt.Errorf("skill not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to get skill: %w", err), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(skill)
}

// updateSkillHandler updates an existing skill.
// PUT /api/v1/skills/{id}
// Request body: {name, description, path, enabled}
// Response: Updated Skill object
func (h *apiHandler) updateSkillHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Path        string `json:"path"`
		Enabled     bool   `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" {
		api.HandleError(w, fmt.Errorf("name is required"), http.StatusBadRequest)
		return
	}
	if req.Path == "" {
		api.HandleError(w, fmt.Errorf("path is required"), http.StatusBadRequest)
		return
	}

	skill, err := h.skillMgr.UpdateSkill(ctx, id, req.Name, req.Description, req.Path, req.Enabled)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			api.HandleError(w, fmt.Errorf("skill not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to update skill: %w", err), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(skill)
}

// deleteSkillHandler removes a skill by ID.
// DELETE /api/v1/skills/{id}
// Response: 204 No Content on success
func (h *apiHandler) deleteSkillHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	err := h.skillMgr.DeleteSkill(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			api.HandleError(w, fmt.Errorf("skill not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to delete skill: %w", err), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Agent handlers

// listAgentsHandler returns all configured agents.
// GET /api/v1/agents
// Response: Array of Agent objects
func (h *apiHandler) listAgentsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agents, err := h.store.ListAgents(ctx)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to list agents: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(agents)
}

// createAgentHandler creates a new agent configuration.
// POST /api/v1/agents
// Request body: Agent object with optional skill_ids array
// Response: Created Agent object with generated ID
func (h *apiHandler) createAgentHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Use a struct that includes skill_ids for decoding
	var request struct {
		primitive.Agent
		SkillIDs []string `json:"skill_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}

	agent := request.Agent

	// Validate agent fields
	if errors := h.validator.ValidateAgent(&agent); len(errors) > 0 {
		api.HandleError(w, fmt.Errorf("%s", errors.Error()), http.StatusBadRequest)
		return
	}

	// Validate skill IDs if provided
	if len(request.SkillIDs) > 0 {
		skillErrors := h.validator.ValidateSkillIDs(ctx, h.store, request.SkillIDs)
		if len(skillErrors) > 0 {
			api.HandleError(w, fmt.Errorf("%s", skillErrors.Error()), http.StatusBadRequest)
			return
		}
	}

	// Generate ID if not provided
	if agent.ID == "" {
		agent.ID = uuid.New().String()
	}

	if err := h.store.CreateAgent(ctx, &agent); err != nil {
		api.HandleError(w, fmt.Errorf("failed to create agent: %w", err), http.StatusInternalServerError)
		return
	}

	// Assign skills if skill_ids were provided
	if len(request.SkillIDs) > 0 {
		if err := h.store.SetAgentSkills(ctx, agent.ID, request.SkillIDs); err != nil {
			api.HandleError(w, fmt.Errorf("failed to assign skills to agent: %w", err), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(agent)
}

// getAgentHandler retrieves an agent by ID.
// GET /api/v1/agents/{id}
// Response: Agent object
func (h *apiHandler) getAgentHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	agent, err := h.store.GetAgent(ctx, id)
	if err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("agent not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to get agent: %w", err), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(agent)
}

// updateAgentHandler updates an existing agent.
// PUT /api/v1/agents/{id}
// Request body: Agent object with optional skill_ids array
// Response: Updated Agent object
func (h *apiHandler) updateAgentHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	// Use a struct that includes skill_ids for decoding
	var request struct {
		primitive.Agent
		SkillIDs []string `json:"skill_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}

	agent := request.Agent
	agent.ID = id

	if err := h.store.UpdateAgent(ctx, &agent); err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("agent not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to update agent: %w", err), http.StatusInternalServerError)
		}
		return
	}

	// Update skills if skill_ids were provided
	// Note: We only update skills if skill_ids is explicitly provided in the request.
	// An empty array means "remove all skills", while not including the field means "keep existing skills"
	// To detect if the field was included, we check if JSON had the field.
	// However, for simplicity, we'll always update if skill_ids is present in the request struct
	// (even if empty) to allow explicit skill management via update.
	// The only way to know if skill_ids was "not provided" is to check if the decoder actually set it.
	// Since we can't easily detect that with json.Decoder, we'll check the raw request body.
	if r.ContentLength > 0 {
		bodyBytes, err := io.ReadAll(r.Body)
		if err == nil {
			var rawRequest map[string]interface{}
			if json.Unmarshal(bodyBytes, &rawRequest) == nil {
				if _, hasSkillIDs := rawRequest["skill_ids"]; hasSkillIDs {
					if err := h.store.SetAgentSkills(ctx, agent.ID, request.SkillIDs); err != nil {
						api.HandleError(w, fmt.Errorf("failed to update agent skills: %w", err), http.StatusInternalServerError)
						return
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(agent)
}

// deleteAgentHandler removes an agent by ID.
// DELETE /api/v1/agents/{id}
// Response: 204 No Content on success
func (h *apiHandler) deleteAgentHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	if err := h.store.DeleteAgent(ctx, id); err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("agent not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to delete agent: %w", err), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getAgentToolsHandler retrieves tools assigned to an agent.
// GET /api/v1/agents/{id}/tools
// Response: Array of Tool objects assigned to the agent
func (h *apiHandler) getAgentToolsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	agentID := vars["id"]

	tools, err := h.store.GetAgentTools(ctx, agentID)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to get agent tools: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(tools)
}

// assignToolToAgentHandler assigns a tool to an agent.
// POST /api/v1/agents/{id}/tools
// Request body: {tool_id}
// Response: 201 Created on success
func (h *apiHandler) assignToolToAgentHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	agentID := vars["id"]

	var request struct {
		ToolID string `json:"tool_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}

	// Validate tool_id is provided
	if strings.TrimSpace(request.ToolID) == "" {
		api.HandleError(w, fmt.Errorf("tool_id is required"), http.StatusBadRequest)
		return
	}

	if err := h.store.AssignToolToAgent(ctx, agentID, request.ToolID); err != nil {
		api.HandleError(w, fmt.Errorf("failed to assign tool to agent: %w", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

// removeToolFromAgentHandler removes a tool from an agent.
// DELETE /api/v1/agents/{id}/tools/{toolId}
// Response: 204 No Content on success
func (h *apiHandler) removeToolFromAgentHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	agentID := vars["id"]
	toolID := vars["toolId"]

	if err := h.store.RemoveToolFromAgent(ctx, agentID, toolID); err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("tool not assigned to agent"), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to remove tool from agent: %w", err), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Agent Skills handlers

// getAgentSkillsHandler retrieves skills assigned to an agent.
// GET /api/v1/agents/{id}/skills
// Response: Object with data array containing Skill objects
func (h *apiHandler) getAgentSkillsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	agentID := vars["id"]

	skills, err := h.store.GetAgentSkills(ctx, agentID)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to get agent skills: %w", err), http.StatusInternalServerError)
		return
	}

	// Ensure we return an empty array instead of null when there are no skills
	if skills == nil {
		skills = make([]*primitive.Skill, 0)
	}

	resp := map[string]interface{}{
		"data": skills,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// assignSkillsToAgentHandler assigns skills to an agent.
// PUT /api/v1/agents/{id}/skills
// Request body: {skill_ids: ["id1", "id2", ...]}
// Response: Updated list of skills assigned to the agent
func (h *apiHandler) assignSkillsToAgentHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	agentID := vars["id"]

	var request struct {
		SkillIDs []string `json:"skill_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}

	if len(request.SkillIDs) == 0 {
		api.HandleError(w, fmt.Errorf("at least one skill_id is required"), http.StatusBadRequest)
		return
	}

	// Validate that all skill IDs exist in the database
	skillErrors := h.validator.ValidateSkillIDs(ctx, h.store, request.SkillIDs)
	if len(skillErrors) > 0 {
		api.HandleError(w, fmt.Errorf("%s", skillErrors.Error()), http.StatusBadRequest)
		return
	}

	// Assign each skill to the agent
	for _, skillID := range request.SkillIDs {
		if err := h.store.AssignSkillToAgent(ctx, agentID, skillID); err != nil {
			api.HandleError(w, fmt.Errorf("failed to assign skill %s to agent: %w", skillID, err), http.StatusInternalServerError)
			return
		}
	}

	// Return the updated list of skills for the agent
	skills, err := h.store.GetAgentSkills(ctx, agentID)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to get agent skills: %w", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(skills)
}

// removeSkillFromAgentHandler removes a skill from an agent.
// DELETE /api/v1/agents/{id}/skills/{skillId}
// Response: 204 No Content on success
func (h *apiHandler) removeSkillFromAgentHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	agentID := vars["id"]
	skillID := vars["skillId"]

	if err := h.store.RemoveSkillFromAgent(ctx, agentID, skillID); err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("skill not assigned to agent"), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to remove skill from agent: %w", err), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Workflow handlers

// listWorkflowsHandler returns all configured workflows.
// GET /api/v1/workflows
// Response: Array of Workflow objects
func (h *apiHandler) listWorkflowsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	workflows, err := h.store.ListWorkflows(ctx)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to list workflows: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(workflows)
}

// createWorkflowHandler creates a new workflow.
// POST /api/v1/workflows
// Request body: Workflow object with name, description, is_async flag
// Response: Created Workflow object with generated ID
func (h *apiHandler) createWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var workflow primitive.Workflow
	if err := json.NewDecoder(r.Body).Decode(&workflow); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}

	// Validate workflow fields
	if errors := h.validator.ValidateWorkflow(&workflow); len(errors) > 0 {
		api.HandleError(w, fmt.Errorf("%s", errors.Error()), http.StatusBadRequest)
		return
	}

	if err := h.store.CreateWorkflow(ctx, &workflow); err != nil {
		api.HandleError(w, fmt.Errorf("failed to create workflow: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(workflow)
}

// getWorkflowHandler retrieves a workflow by ID.
// GET /api/v1/workflows/{id}
// Response: Workflow object
func (h *apiHandler) getWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	workflow, err := h.store.GetWorkflow(ctx, id)
	if err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("workflow not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to get workflow: %w", err), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(workflow)
}

// updateWorkflowHandler updates an existing workflow.
// PUT /api/v1/workflows/{id}
// Request body: Workflow object with updated fields
// Response: Updated Workflow object
func (h *apiHandler) updateWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	var workflow primitive.Workflow
	if err := json.NewDecoder(r.Body).Decode(&workflow); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}
	workflow.ID = id

	if err := h.store.UpdateWorkflow(ctx, &workflow); err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("workflow not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to update workflow: %w", err), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(workflow)
}

// deleteWorkflowHandler removes a workflow by ID.
// DELETE /api/v1/workflows/{id}
// Response: 204 No Content on success
func (h *apiHandler) deleteWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	if err := h.store.DeleteWorkflow(ctx, id); err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("workflow not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to delete workflow: %w", err), http.StatusInternalServerError)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Workflow step handlers

// listWorkflowStepsHandler returns all steps for a workflow.
// GET /api/v1/workflows/{id}/steps
// Response: Array of WorkflowStep objects ordered by step_order
func (h *apiHandler) listWorkflowStepsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	workflowID := vars["id"]

	steps, err := h.store.ListWorkflowSteps(ctx, workflowID)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to list workflow steps: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(steps)
}

// createWorkflowStepHandler creates a new step in a workflow.
// POST /api/v1/workflows/{id}/steps
// Request body: WorkflowStep object with step_type, agent_id or wasm_module_id, config
// Response: Created WorkflowStep object with generated ID and auto-assigned step_order
func (h *apiHandler) createWorkflowStepHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	workflowID := vars["id"]

	var step primitive.WorkflowStep
	if err := json.NewDecoder(r.Body).Decode(&step); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}
	step.WorkflowID = workflowID

	// Validate workflow step fields
	if errors := h.validator.ValidateWorkflowStep(&step); len(errors) > 0 {
		api.HandleError(w, fmt.Errorf("%s", errors.Error()), http.StatusBadRequest)
		return
	}

	// If step_order is not provided or is 0, auto-assign the next available order
	if step.StepOrder <= 0 {
		// Get existing steps to determine the next step_order
		existingSteps, err := h.store.ListWorkflowSteps(ctx, workflowID)
		if err != nil {
			api.HandleError(w, fmt.Errorf("failed to list existing workflow steps: %w", err), http.StatusInternalServerError)
			return
		}

		// Find the maximum step_order and increment by 1
		maxOrder := 0
		for _, existingStep := range existingSteps {
			if existingStep.StepOrder > maxOrder {
				maxOrder = existingStep.StepOrder
			}
		}
		step.StepOrder = maxOrder + 1
	}

	if err := h.store.CreateWorkflowStep(ctx, &step); err != nil {
		api.HandleError(w, fmt.Errorf("failed to create workflow step: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(step)
}

// updateWorkflowStepHandler updates an existing workflow step.
// PUT /api/v1/workflows/{workflow_id}/steps/{step_id}
// Request body: WorkflowStep object with updated fields
// Response: Updated WorkflowStep object
func (h *apiHandler) updateWorkflowStepHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	workflowID := vars["workflow_id"]
	stepID := vars["step_id"]

	var step primitive.WorkflowStep
	if err := json.NewDecoder(r.Body).Decode(&step); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}

	// Verify the step belongs to the specified workflow
	existingStep, err := h.store.ListWorkflowSteps(ctx, workflowID)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to verify workflow step: %w", err), http.StatusInternalServerError)
		return
	}

	stepExists := false
	for _, s := range existingStep {
		if s.ID == stepID {
			stepExists = true
			break
		}
	}

	if !stepExists {
		api.HandleError(w, fmt.Errorf("workflow step not found in specified workflow"), http.StatusNotFound)
		return
	}

	// Update the step using the workflow manager
	updatedStep, err := h.workflowMgr.UpdateWorkflowStep(ctx, stepID, step.StepOrder, step.StepType, step.AgentID, step.WasmModuleID, step.Config)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to update workflow step: %w", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updatedStep)
}

// deleteWorkflowStepHandler removes a step from a workflow.
// DELETE /api/v1/workflows/{workflow_id}/steps/{step_id}
// Response: 204 No Content on success
func (h *apiHandler) deleteWorkflowStepHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	workflowID := vars["workflow_id"]
	stepID := vars["step_id"]

	// Verify the step belongs to the specified workflow
	existingSteps, err := h.store.ListWorkflowSteps(ctx, workflowID)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to verify workflow step: %w", err), http.StatusInternalServerError)
		return
	}

	stepExists := false
	for _, s := range existingSteps {
		if s.ID == stepID {
			stepExists = true
			break
		}
	}

	if !stepExists {
		api.HandleError(w, fmt.Errorf("workflow step not found in specified workflow"), http.StatusNotFound)
		return
	}

	// Delete the step using the workflow manager
	if err := h.workflowMgr.DeleteWorkflowStep(ctx, stepID); err != nil {
		api.HandleError(w, fmt.Errorf("failed to delete workflow step: %w", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// reorderWorkflowStepsHandler reorders steps in a workflow.
// POST /api/v1/workflows/{id}/reorder
// Request body: {step_ids: ["id1", "id2", ...]} in desired execution order
// Response: Updated list of WorkflowStep objects
func (h *apiHandler) reorderWorkflowStepsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	workflowID := vars["id"]

	// Verify the workflow exists
	_, err := h.store.GetWorkflow(ctx, workflowID)
	if err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("workflow not found: %s", workflowID), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to get workflow: %w", err), http.StatusInternalServerError)
		}
		return
	}

	// Parse the request body to get the new order
	var req struct {
		StepIDs []string `json:"step_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}

	// Validate step_ids is provided
	if len(req.StepIDs) == 0 {
		api.HandleError(w, fmt.Errorf("step_ids is required"), http.StatusBadRequest)
		return
	}

	// Use the workflow manager to reorder steps
	if err := h.workflowMgr.ReorderWorkflowSteps(ctx, workflowID, req.StepIDs); err != nil {
		api.HandleError(w, fmt.Errorf("failed to reorder workflow steps: %w", err), http.StatusInternalServerError)
		return
	}

	// Return the updated steps
	updatedSteps, err := h.store.ListWorkflowSteps(ctx, workflowID)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to get updated workflow steps: %w", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updatedSteps)
}

// Job management handlers

// listJobsHandler returns paginated list of jobs with optional filtering.
// GET /api/v1/jobs
// Query params: page, page_size, status, search, workflow_name
// Response: Object with jobs array, pagination info (page, page_size, total_count, total_pages)
func (h *apiHandler) listJobsHandler(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	pageStr := r.URL.Query().Get("page")
	pageSizeStr := r.URL.Query().Get("page_size")
	statusStr := r.URL.Query().Get("status")
	searchStr := r.URL.Query().Get("search")
	workflowNameStr := r.URL.Query().Get("workflow_name")

	// Parse page
	page := 1
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	// Parse page size
	pageSize := 20
	if pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 && ps <= 100 {
			pageSize = ps
		}
	}

	// Parse status
	var status *job.Status
	if statusStr != "" {
		s := job.Status(statusStr)
		status = &s
	}

	// Create options
	opts := job.ListJobsOptions{
		Page:         page,
		PageSize:     pageSize,
		Status:       status,
		Search:       searchStr,
		WorkflowName: workflowNameStr,
	}

	jobs, totalCount, err := h.jobStore.ListJobs(opts)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to list jobs: %w", err), http.StatusInternalServerError)
		return
	}

	// Enrich jobs with workflow and WASM module names
	ctx := r.Context()
	enrichedJobs := make([]*job.EnhancedJob, len(jobs))

	for i, j := range jobs {
		enrichedJob := &job.EnhancedJob{
			Job: j,
		}

		// If this is a workflow job, get the workflow name
		if j.WorkflowID != "" {
			workflow, err := h.store.GetWorkflow(ctx, j.WorkflowID)
			if err == nil {
				enrichedJob.WorkflowName = workflow.Name
			}
		}

		// If this is a WASM module job, get the WASM module name
		if j.WasmModuleID != nil {
			wasmModule, err := h.store.GetWasmModule(ctx, *j.WasmModuleID)
			if err == nil {
				enrichedJob.WasmModuleName = wasmModule.Name
			}
		}

		enrichedJobs[i] = enrichedJob
	}

	// Create response with pagination info
	response := struct {
		Jobs       []*job.EnhancedJob `json:"jobs"`
		Page       int                `json:"page"`
		PageSize   int                `json:"page_size"`
		TotalCount int                `json:"total_count"`
		TotalPages int                `json:"total_pages"`
	}{
		Jobs:       enrichedJobs,
		Page:       page,
		PageSize:   pageSize,
		TotalCount: totalCount,
		TotalPages: (totalCount + pageSize - 1) / pageSize,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// createJobHandler creates a new job for workflow or WASM execution.
// POST /api/v1/jobs
// Request body: {workflow_id, input_data, working_directory?}
// Response: Job object with status "queued" for workflows or "running" for direct WASM execution
func (h *apiHandler) createJobHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkflowID       string                 `json:"workflow_id"`
		InputData        map[string]interface{} `json:"input_data"`
		WorkingDirectory string                 `json:"working_directory,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}

	// Validate workflow_id is provided
	if strings.TrimSpace(req.WorkflowID) == "" {
		api.HandleError(w, fmt.Errorf("workflow_id is required"), http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Check if this is a WASM module execution (workflow_id is a WASM module ID)
	// Try to get the workflow first
	workflow, err := h.store.GetWorkflow(ctx, req.WorkflowID)

	var newJob *job.Job

	if err == nil && workflow != nil {
		// This is a valid workflow ID, create a queued job for workflow execution
		newJob = &job.Job{
			ID:               uuid.New().String(),
			WorkflowID:       req.WorkflowID,
			Status:           job.StatusQueued,
			InputData:        req.InputData,
			WorkingDirectory: req.WorkingDirectory,
			CreatedAt:        time.Now(),
		}

		if err := h.jobStore.CreateJob(newJob); err != nil {
			api.HandleError(w, fmt.Errorf("failed to create job: %w", err), http.StatusInternalServerError)
			return
		}
	} else {
		// Not a valid workflow ID, check if it's a WASM module ID
		_, err := h.wasmModuleMgr.GetWasmModule(ctx, req.WorkflowID)
		if err != nil {
			api.HandleError(w, fmt.Errorf("invalid workflow_id or wasm_module_id: %s", req.WorkflowID), http.StatusBadRequest)
			return
		}

		// This is a WASM module, execute it directly
		wasmModuleID := req.WorkflowID // The frontend sends WASM module ID in workflow_id field
		newJob = &job.Job{
			ID:               uuid.New().String(),
			WorkflowID:       "", // Empty for WASM executions
			WasmModuleID:     &wasmModuleID,
			Status:           job.StatusRunning, // Start as running since we're executing immediately
			InputData:        req.InputData,
			WorkingDirectory: req.WorkingDirectory,
			CreatedAt:        time.Now(),
		}

		// Create the job record first
		if err := h.jobStore.CreateJob(newJob); err != nil {
			api.HandleError(w, fmt.Errorf("failed to create job: %w", err), http.StatusInternalServerError)
			return
		}

		// Execute the WASM module directly
		go func() {
			// Create a new context that isn't tied to the HTTP request
			execCtx := context.Background()

			now := time.Now()
			// Update job status to running
			newJob.Status = job.StatusRunning
			newJob.StartedAt = &now
			if err := h.jobStore.UpdateJob(newJob); err != nil {
				log.Printf("Failed to update job status: %v", err)
			}

			// Execute the WASM module with the new context and working directory
			result, err := h.workflowEngine.GetWASMExecutor().Execute(execCtx, *newJob.WasmModuleID, req.InputData, req.WorkingDirectory)

			// Update job with results
			now = time.Now()
			if err != nil {
				newJob.Status = job.StatusFailed
				newJob.OutputData = map[string]interface{}{
					"error": err.Error(),
				}
			} else {
				newJob.Status = job.StatusCompleted
				newJob.OutputData = result
			}
			newJob.CompletedAt = &now

			if updateErr := h.jobStore.UpdateJob(newJob); updateErr != nil {
				log.Printf("Failed to update job after WASM execution: %v", updateErr)
			}
		}()
	}

	// Return response in format expected by frontend: {data: {...}}
	response := map[string]interface{}{
		"data": newJob,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(response)
}

// getJobHandler retrieves a job by ID with enriched workflow/WASM module names.
// GET /api/v1/jobs/{id}
// Response: EnhancedJob object with workflow_name and wasm_module_name populated
func (h *apiHandler) getJobHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	j, err := h.jobStore.GetJob(id)
	if err != nil {
		if err.Error() == "job not found" {
			api.HandleError(w, fmt.Errorf("job not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to get job: %w", err), http.StatusInternalServerError)
		}
		return
	}

	// Enrich job with workflow and WASM module names
	ctx := r.Context()
	enrichedJob := &job.EnhancedJob{
		Job: j,
	}

	// If this is a workflow job, get the workflow name
	if j.WorkflowID != "" {
		workflow, err := h.store.GetWorkflow(ctx, j.WorkflowID)
		if err == nil {
			enrichedJob.WorkflowName = workflow.Name
		}
	}

	// If this is a WASM module job, get the WASM module name
	if j.WasmModuleID != nil {
		wasmModule, err := h.store.GetWasmModule(ctx, *j.WasmModuleID)
		if err == nil {
			enrichedJob.WasmModuleName = wasmModule.Name
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(enrichedJob)
}

// listJobStepsHandler returns all steps for a job with enriched names.
// GET /api/v1/jobs/{id}/steps
// Response: Array of EnhancedJobStep objects with agent_name and wasm_module_name populated
func (h *apiHandler) listJobStepsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	steps, err := h.jobStore.ListJobSteps(jobID)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to list job steps: %w", err), http.StatusInternalServerError)
		return
	}

	// Enrich job steps with agent or WASM module names
	ctx := r.Context()
	enrichedSteps := make([]*job.EnhancedJobStep, len(steps))

	for i, step := range steps {
		enrichedStep := &job.EnhancedJobStep{
			JobStep: step,
		}

		// Get the workflow step to determine if it's an agent or WASM step
		workflowStep, err := h.workflowMgr.GetWorkflowStep(ctx, step.WorkflowStepID)
		if err == nil {
			// If this is an agent step, get the agent name
			if workflowStep.AgentID != nil {
				agent, err := h.store.GetAgent(ctx, *workflowStep.AgentID)
				if err == nil {
					enrichedStep.AgentName = agent.Name
				}
			}

			// If this is a WASM module step, get the WASM module name
			if workflowStep.WasmModuleID != nil {
				wasmModule, err := h.store.GetWasmModule(ctx, *workflowStep.WasmModuleID)
				if err == nil {
					enrichedStep.WasmModuleName = wasmModule.Name
				}
			}
		}

		enrichedSteps[i] = enrichedStep
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(enrichedSteps)
}

// cancelJobHandler attempts to cancel a running or queued job.
// DELETE /api/v1/jobs/{id}
// Response: Object with message and job id on success
func (h *apiHandler) cancelJobHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	if err := h.jobStore.CancelJob(jobID); err != nil {
		if err.Error() == "job not found or cannot be cancelled" {
			api.HandleError(w, fmt.Errorf("job not found or cannot be cancelled: %s", jobID), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to cancel job: %w", err), http.StatusInternalServerError)
		}
		return
	}

	// Return success response
	response := map[string]interface{}{
		"message": "Job cancelled successfully",
		"id":      jobID,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

// WASM Module handlers

// listWasmModulesHandler returns all uploaded WASM modules.
// GET /api/v1/wasm-modules
// Response: Object with data array containing WasmModuleListItem objects
func (h *apiHandler) listWasmModulesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	modules, err := h.wasmModuleMgr.ListWasmModules(ctx)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to list WASM modules: %w", err), http.StatusInternalServerError)
		return
	}

	// Ensure we return an empty array instead of null when there are no modules
	if modules == nil {
		modules = make([]*primitive.WasmModuleListItem, 0)
	}

	resp := map[string]interface{}{
		"data": modules,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// createWasmModuleHandler uploads a new WASM module.
// POST /api/v1/wasm-modules
// Content-Type: multipart/form-data
// Form fields: name (required), description, config (JSON), module_data (file, required)
// Response: Created WasmModule object with generated ID
func (h *apiHandler) createWasmModuleHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse multipart form with 32MB max memory
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		api.HandleError(w, fmt.Errorf("failed to parse form: %w", err), http.StatusBadRequest)
		return
	}

	// Get form values
	name := r.FormValue("name")
	description := r.FormValue("description")
	config := r.FormValue("config")

	// Validate name is provided
	if strings.TrimSpace(name) == "" {
		api.HandleError(w, fmt.Errorf("name is required"), http.StatusBadRequest)
		return
	}

	// Get file
	file, _, err := r.FormFile("module_data")
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to get module file: %w", err), http.StatusBadRequest)
		return
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil {
			log.Printf("Error closing file: %v", closeErr)
		}
	}()

	// Read file data
	moduleData := make([]byte, 0)
	buf := make([]byte, 1024)
	for {
		n, err := file.Read(buf)
		if err != nil && err.Error() != "EOF" {
			api.HandleError(w, fmt.Errorf("failed to read module file: %w", err), http.StatusBadRequest)
			return
		}
		if n == 0 {
			break
		}
		moduleData = append(moduleData, buf[:n]...)
	}

	// Parse config as JSON if provided
	var configMap map[string]interface{}
	if config != "" {
		// Validate that config is valid JSON
		if err := json.Unmarshal([]byte(config), &configMap); err != nil {
			api.HandleError(w, fmt.Errorf("config must be valid JSON: %w", err), http.StatusBadRequest)
			return
		}
	} else {
		configMap = make(map[string]interface{})
	}

	// Create WASM module
	module, err := h.wasmModuleMgr.CreateWasmModule(ctx, name, description, moduleData, configMap)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to create WASM module: %w", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(module)
}

// getWasmModuleHandler retrieves a WASM module by ID.
// GET /api/v1/wasm-modules/{id}
// Response: WasmModule object with binary module_data
func (h *apiHandler) getWasmModuleHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	module, err := h.wasmModuleMgr.GetWasmModule(ctx, id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			api.HandleError(w, fmt.Errorf("WASM module not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to get WASM module: %w", err), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(module)
}

// updateWasmModuleHandler updates an existing WASM module.
// PUT /api/v1/wasm-modules/{id}
// Content-Type: multipart/form-data
// Form fields: name, description, config (JSON), module_data (file, optional)
// Response: Updated WasmModule object
func (h *apiHandler) updateWasmModuleHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	// Parse multipart form with 32MB max memory
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		api.HandleError(w, fmt.Errorf("failed to parse form: %w", err), http.StatusBadRequest)
		return
	}

	// Get form values
	name := r.FormValue("name")
	description := r.FormValue("description")
	config := r.FormValue("config")

	// Get file (optional)
	var moduleData []byte = nil
	file, _, err := r.FormFile("module_data")
	if err != nil && err != http.ErrMissingFile {
		api.HandleError(w, fmt.Errorf("failed to get module file: %w", err), http.StatusBadRequest)
		return
	}

	if err == nil && file != nil {
		defer func() {
			if closeErr := file.Close(); closeErr != nil {
				log.Printf("Error closing file: %v", closeErr)
			}
		}()

		// Read file data
		moduleData = make([]byte, 0)
		buf := make([]byte, 1024)
		for {
			n, err := file.Read(buf)
			if err != nil && err.Error() != "EOF" {
				api.HandleError(w, fmt.Errorf("failed to read module file: %w", err), http.StatusBadRequest)
				return
			}
			if n == 0 {
				break
			}
			moduleData = append(moduleData, buf[:n]...)
		}
	}

	// Parse config as JSON if provided
	var configMap map[string]interface{} = nil
	if config != "" {
		// Validate that config is valid JSON
		if err := json.Unmarshal([]byte(config), &configMap); err != nil {
			api.HandleError(w, fmt.Errorf("config must be valid JSON: %w", err), http.StatusBadRequest)
			return
		}
	}

	// Update WASM module
	module, err := h.wasmModuleMgr.UpdateWasmModule(ctx, id, name, description, moduleData, configMap)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			api.HandleError(w, fmt.Errorf("WASM module not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to update WASM module: %w", err), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(module)
}

// deleteWasmModuleHandler removes a WASM module by ID.
// DELETE /api/v1/wasm-modules/{id}
// Response: 204 No Content on success
func (h *apiHandler) deleteWasmModuleHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	if err := h.wasmModuleMgr.DeleteWasmModule(ctx, id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			api.HandleError(w, fmt.Errorf("WASM module not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to delete WASM module: %w", err), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Settings handlers

// listSettingsHandler returns all application settings.
// GET /api/v1/settings
// Response: Array of Setting objects with key-value pairs
func (h *apiHandler) listSettingsHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	settings, err := h.store.ListSettings(ctx)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to list settings: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(settings)
}

// getSettingHandler retrieves a setting by key.
// GET /api/v1/settings/{key}
// Response: Setting object with key and value
func (h *apiHandler) getSettingHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	key := vars["key"]

	setting, err := h.store.GetSetting(ctx, key)
	if err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("setting not found: %s", key), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to get setting: %w", err), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(setting)
}

// updateSettingHandler updates or creates a setting.
// PUT /api/v1/settings/{key}
// Request body: Setting object with matching key
// Response: Updated Setting object
func (h *apiHandler) updateSettingHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	key := vars["key"]

	var setting primitive.Setting
	if err := json.NewDecoder(r.Body).Decode(&setting); err != nil {
		api.HandleError(w, fmt.Errorf("failed to decode request: %w", err), http.StatusBadRequest)
		return
	}

	// Ensure the key in the URL matches the key in the body
	if setting.Key != key {
		api.HandleError(w, fmt.Errorf("key mismatch: URL key %s does not match body key %s", key, setting.Key), http.StatusBadRequest)
		return
	}

	if err := h.store.UpdateSetting(ctx, &setting); err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("setting not found: %s", key), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to update setting: %w", err), http.StatusInternalServerError)
		}
		return
	}

	// Return the updated setting
	updatedSetting, err := h.store.GetSetting(ctx, key)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to get updated setting: %w", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(updatedSetting)
}
