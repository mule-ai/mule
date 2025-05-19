package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mule-ai/mule/internal/config"
	"github.com/mule-ai/mule/internal/settings"
	"github.com/mule-ai/mule/internal/state"
	"github.com/mule-ai/mule/pkg/agent"
)

func HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	state.State.Mu.RLock()
	defer state.State.Mu.RUnlock()

	err := json.NewEncoder(w).Encode(state.State.Settings)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func HandleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var settings settings.Settings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := handleSettingsChange(settings); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func handleSettingsChange(newSettings settings.Settings) error {
	state.State.Mu.Lock()
	state.State.Settings = newSettings
	state.State.Mu.Unlock()

	// Update agents and workflows after settings are updated.
	if err := state.State.UpdateAgents(); err != nil {
		return err
	}
	if err := state.State.UpdateWorkflows(); err != nil {
		return err
	}

	configPath, err := config.GetHomeConfigPath()
	if err != nil {
		return fmt.Errorf("error getting config path: %w", err)
	}
	if err := config.SaveConfig(configPath); err != nil {
		return fmt.Errorf("error saving config: %w", err)
	}
	return nil
}

func HandleTemplateValues(w http.ResponseWriter, r *http.Request) {
	values := agent.GetPromptTemplateValues()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(values); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// HandleWorkflowOutputFields returns the available output fields for workflow steps
func HandleWorkflowOutputFields(w http.ResponseWriter, r *http.Request) {
	// These are the fields that can be used as outputs from one agent to another
	outputFields := []string{
		"generatedText",     // The raw generated text from an agent
		"extractedCode",     // Code extracted from the generated text
		"summary",           // A summary of the generated content
		"actionItems",       // Action items extracted from the content
		"suggestedChanges",  // Suggested code changes
		"reviewComments",    // Code review comments
		"testCases",         // Generated test cases
		"documentationText", // Generated documentation
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(outputFields); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// HandleWorkflowInputMappings returns the available input mappings for workflow steps
func HandleWorkflowInputMappings(w http.ResponseWriter, r *http.Request) {
	// These are the ways to map outputs from previous steps to inputs for the next step
	inputMappings := []string{
		"useAsPrompt",       // Use the output directly as the prompt
		"appendToPrompt",    // Append the output to the existing prompt
		"useAsContext",      // Use the output as context information
		"useAsInstructions", // Use the output as instructions for the agent
		"useAsCodeInput",    // Use the output as code to be processed
		"useAsReviewTarget", // Use the output as the target for a review
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(inputMappings); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
