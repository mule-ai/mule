package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/jbutlerdev/dev-team/internal/config"
	"github.com/jbutlerdev/dev-team/internal/settings"
	"github.com/jbutlerdev/dev-team/internal/state"
	"github.com/jbutlerdev/dev-team/pkg/agent"
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
	// TODO:
	// This needs to get updated to properly recreate the appstate
	// if the AI providers get recreated, the agents will need to be recreated
	// therefor the agents referenced by the repositories will need to be recreated
	/*
		state.State.Mu.RLock()
		oldSettings := state.State.Settings
		state.State.Mu.RUnlock()

		refreshAiProvider := oldSettings.Provider != newSettings.Provider ||
			oldSettings.APIKey != newSettings.APIKey ||
			oldSettings.Server != newSettings.Server

		state.State.Mu.Lock()
		if refreshAiProvider {
			// TODO: provide the logger
			genaiProvider, err := genai.NewProvider(newSettings.Provider, genai.ProviderOptions{
				APIKey:  newSettings.APIKey,
				BaseURL: newSettings.Server,
			})
			if err != nil {
				return fmt.Errorf("error initializing GenAI provider: %v", err)
			}
			state.State.GenAI = genaiProvider
		}
	*/
	state.State.Mu.Lock()
	state.State.Settings = newSettings
	state.State.Mu.Unlock()
	configPath, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configPath = filepath.Join(configPath, config.ConfigPath)
	if err := config.SaveConfig(configPath); err != nil {
		return fmt.Errorf("error saving config: %v", err)
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
