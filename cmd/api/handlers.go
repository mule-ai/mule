package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/mule-ai/mule/internal/agent"
	"github.com/mule-ai/mule/internal/api"
	"github.com/mule-ai/mule/internal/database"
	"github.com/mule-ai/mule/internal/manager"
	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/internal/validation"
	"github.com/mule-ai/mule/pkg/job"
)

type apiHandler struct {
	store           primitive.PrimitiveStore
	runtime         *agent.Runtime
	jobStore        job.JobStore
	validator       *validation.Validator
	wasmModuleMgr   *manager.WasmModuleManager
}

func NewAPIHandler(db *database.DB) *apiHandler {
	store := primitive.NewPGStore(db.DB) // Access the underlying *sql.DB
	runtime := agent.NewRuntime(store)
	jobStore := job.NewPGStore(db.DB) // Access the underlying *sql.DB
	validator := validation.NewValidator()
	wasmModuleMgr := manager.NewWasmModuleManager(db)

	return &apiHandler{
		store:         store,
		runtime:       runtime,
		jobStore:      jobStore,
		validator:     validator,
		wasmModuleMgr: wasmModuleMgr,
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
	} else if strings.HasPrefix(req.Model, "workflow/") {
		// Execute workflow (async)
		resp, err := h.runtime.ExecuteWorkflow(ctx, &req)
		if err != nil {
			api.HandleError(w, fmt.Errorf("failed to execute workflow: %w", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	} else {
		api.HandleError(w, fmt.Errorf("model must start with 'agent/' or 'workflow/'"), http.StatusBadRequest)
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

	if err := h.store.CreateWorkflowStep(ctx, &step); err != nil {
		api.HandleError(w, fmt.Errorf("failed to create workflow step: %w", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(step)
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