package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jbutlerdev/genai"
	"github.com/mule-ai/mule/internal/settings"
	"github.com/mule-ai/mule/internal/state"
	"github.com/mule-ai/mule/pkg/agent"
)

// WorkflowExecutionRequest represents a request to execute a workflow
type WorkflowExecutionRequest struct {
	WorkflowID string            `json:"workflowId"`
	Input      agent.PromptInput `json:"input"`
	Path       string            `json:"path"`
}

// WorkflowExecutionResponse represents the response from executing a workflow
type WorkflowExecutionResponse struct {
	Results map[string]agent.WorkflowResult `json:"results"`
	Error   string                          `json:"error,omitempty"`
}

// HandleExecuteWorkflow handles requests to execute a workflow
func HandleExecuteWorkflow(w http.ResponseWriter, r *http.Request) {
	var request WorkflowExecutionRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Find the workflow by ID
	state.State.Mu.RLock()
	var workflowSettings *settings.WorkflowSettings
	for i := range state.State.Settings.Workflows {
		if state.State.Settings.Workflows[i].ID == request.WorkflowID {
			workflowSettings = &state.State.Settings.Workflows[i]
			break
		}
	}
	state.State.Mu.RUnlock()

	if workflowSettings == nil {
		http.Error(w, fmt.Sprintf("workflow with ID %s not found", request.WorkflowID), http.StatusNotFound)
		return
	}

	// Convert settings.WorkflowStep to agent.WorkflowStep
	workflowSteps := make([]agent.WorkflowStep, len(workflowSettings.Steps))
	for i, step := range workflowSettings.Steps {
		workflowSteps[i] = agent.WorkflowStep{
			ID:           step.ID,
			AgentName:    step.AgentName,
			InputMapping: step.InputMapping,
			OutputField:  step.OutputField,
			IsFirst:      step.IsFirst,
		}
	}

	// Create a map of agent names to agent instances
	agentMap := make(map[string]*agent.Agent)
	state.State.Mu.RLock()
	for _, agentOpts := range state.State.Settings.Agents {
		// Find the provider for this agent
		var provider *genai.Provider
		switch agentOpts.ProviderName {
		case genai.OLLAMA:
			provider = state.State.GenAI.Ollama
		case genai.GEMINI:
			provider = state.State.GenAI.Gemini
		}

		if provider != nil {
			// Create a copy of the agent options with the provider
			opts := agentOpts
			opts.Provider = provider
			opts.Path = request.Path
			opts.Logger = state.State.Logger.WithName("workflow-agent").WithValues("name", opts.Name)
			opts.RAG = state.State.RAG

			// Create the agent
			agentMap[agentOpts.Name] = agent.NewAgent(opts)
		}
	}
	state.State.Mu.RUnlock()

	// Execute the workflow
	results, err := agent.ExecuteWorkflow(workflowSteps, agentMap, request.Input, request.Path)

	// Prepare the response
	response := WorkflowExecutionResponse{
		Results: results,
	}
	if err != nil {
		response.Error = err.Error()
	}

	// Send the response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
