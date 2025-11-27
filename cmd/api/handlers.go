package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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
	"github.com/mule-ai/mule/pkg/database"
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
}

func NewAPIHandler(db *internaldb.DB) *apiHandler {
	store := primitive.NewPGStore(db.DB) // Access the underlying *sql.DB
	jobStore := job.NewPGStore(db.DB)    // Access the underlying *sql.DB
	validator := validation.NewValidator()
	wasmModuleMgr := manager.NewWasmModuleManager(db)
	workflowMgr := manager.NewWorkflowManager(db)

	// Create WASM executor
	wasmExecutor := engine.NewWASMExecutor(db.DB)

	// Create agent runtime (without workflow engine initially)
	runtime := agent.NewRuntime(store, jobStore)

	// Create workflow engine
	workflowEngine := engine.NewEngine(store, jobStore, runtime, wasmExecutor, engine.Config{
		Workers: 5, // Default to 5 workers
	})

	// Set workflow engine on runtime (requires a setter method)
	runtime.SetWorkflowEngine(workflowEngine)

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
		types = append(types, map[string]string{
			"id":       "workflow/" + strings.ToLower(w.Name),
			"object":   "model",
			"owned_by": "mule",
		})
	}

	resp := map[string]interface{}{
		"data": types,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// Chat completions handler implements OpenAI-compatible endpoint
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
		resp, err := h.runtime.ExecuteAgent(ctx, &req)
		if err != nil {
			api.HandleError(w, fmt.Errorf("failed to execute agent: %w", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	} else if strings.HasPrefix(req.Model, "async/workflow/") {
		// Async workflow execution - submit job and return immediately
		newJob, err := h.runtime.ExecuteWorkflow(ctx, &req)
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
		// Sync workflow execution - wait for completion and return ChatCompletionResponse
		newJob, err := h.runtime.ExecuteWorkflow(ctx, &req)
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

func (h *apiHandler) createProviderHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var provider primitive.Provider
	if err := json.NewDecoder(r.Body).Decode(&provider); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
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

	// Make request to provider's /v1/models endpoint
	client := &http.Client{}
	req, err := http.NewRequestWithContext(ctx, "GET", provider.APIBaseURL+"/models", nil)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to create request: %w", err), http.StatusInternalServerError)
		return
	}

	// Add API key if available
	if provider.APIKeyEnc != "" {
		req.Header.Set("Authorization", "Bearer "+provider.APIKeyEnc)
	}

	resp, err := client.Do(req)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to fetch models from provider: %w", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Read and return the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to read response: %w", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(body); err != nil {
		log.Printf("Failed to write response body: %v", err)
	}
}

// Tool handlers
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

func (h *apiHandler) createToolHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var tool primitive.Tool
	if err := json.NewDecoder(r.Body).Decode(&tool); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
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

// Agent handlers
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

func (h *apiHandler) createAgentHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var agent primitive.Agent
	if err := json.NewDecoder(r.Body).Decode(&agent); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}
	if err := h.store.CreateAgent(ctx, &agent); err != nil {
		api.HandleError(w, fmt.Errorf("failed to create agent: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(agent)
}

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

func (h *apiHandler) updateAgentHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	id := vars["id"]

	var agent primitive.Agent
	if err := json.NewDecoder(r.Body).Decode(&agent); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}
	agent.ID = id

	if err := h.store.UpdateAgent(ctx, &agent); err != nil {
		if err == primitive.ErrNotFound {
			api.HandleError(w, fmt.Errorf("agent not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to update agent: %w", err), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(agent)
}

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

	if err := h.store.AssignToolToAgent(ctx, agentID, request.ToolID); err != nil {
		api.HandleError(w, fmt.Errorf("failed to assign tool to agent: %w", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

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

// Workflow handlers
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

func (h *apiHandler) createWorkflowHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var workflow primitive.Workflow
	if err := json.NewDecoder(r.Body).Decode(&workflow); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
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

// Job management handlers
func (h *apiHandler) listJobsHandler(w http.ResponseWriter, r *http.Request) {
	jobs, err := h.jobStore.ListJobs()
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to list jobs: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(jobs)
}

func (h *apiHandler) createJobHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkflowID string                 `json:"workflow_id"`
		InputData  map[string]interface{} `json:"input_data"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
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
			ID:         uuid.New().String(),
			WorkflowID: req.WorkflowID,
			Status:     job.StatusQueued,
			InputData:  req.InputData,
			CreatedAt:  time.Now(),
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
			ID:           uuid.New().String(),
			WorkflowID:   "", // Empty for WASM executions
			WasmModuleID: &wasmModuleID,
			Status:       job.StatusRunning, // Start as running since we're executing immediately
			InputData:    req.InputData,
			CreatedAt:    time.Now(),
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

			// Execute the WASM module with the new context
			result, err := h.workflowEngine.GetWASMExecutor().Execute(execCtx, *newJob.WasmModuleID, req.InputData)

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

func (h *apiHandler) getJobHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	job, err := h.jobStore.GetJob(id)
	if err != nil {
		if err.Error() == "job not found" {
			api.HandleError(w, fmt.Errorf("job not found: %s", id), http.StatusNotFound)
		} else {
			api.HandleError(w, fmt.Errorf("failed to get job: %w", err), http.StatusInternalServerError)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(job)
}

func (h *apiHandler) listJobStepsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	jobID := vars["id"]

	steps, err := h.jobStore.ListJobSteps(jobID)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to list job steps: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(steps)
}

// WASM Module handlers
func (h *apiHandler) listWasmModulesHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	modules, err := h.wasmModuleMgr.ListWasmModules(ctx)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to list WASM modules: %w", err), http.StatusInternalServerError)
		return
	}

	// Ensure we return an empty array instead of null when there are no modules
	if modules == nil {
		modules = make([]*database.WasmModule, 0)
	}

	resp := map[string]interface{}{
		"data": modules,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

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

	// Get file
	file, _, err := r.FormFile("module_data")
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to get module file: %w", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

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

	// Create WASM module
	module, err := h.wasmModuleMgr.CreateWasmModule(ctx, name, description, moduleData)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to create WASM module: %w", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(module)
}

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

	// Get file (optional)
	var moduleData []byte = nil
	file, _, err := r.FormFile("module_data")
	if err == nil && file != nil {
		defer file.Close()

		// Read file data
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

	// Update WASM module
	module, err := h.wasmModuleMgr.UpdateWasmModule(ctx, id, name, description, moduleData)
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
