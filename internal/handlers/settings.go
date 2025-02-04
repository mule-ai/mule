package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jbutlerdev/dev-team/internal/config"
	"github.com/jbutlerdev/dev-team/internal/settings"
	"github.com/jbutlerdev/dev-team/internal/state"
	"github.com/jbutlerdev/genai"
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
	state.State.Mu.RLock()
	oldSettings := state.State.Settings
	state.State.Mu.RUnlock()

	refreshAiProvider := false
	if oldSettings.Provider != newSettings.Provider ||
		oldSettings.APIKey != newSettings.APIKey ||
		oldSettings.Server != newSettings.Server {
		refreshAiProvider = true
	}

	state.State.Mu.Lock()
	if refreshAiProvider {
		genaiProvider, err := genai.NewProvider(newSettings.Provider, newSettings.APIKey)
		if err != nil {
			return fmt.Errorf("error initializing GenAI provider: %v", err)
		}
		state.State.GenAI = genaiProvider
	}
	state.State.Settings = newSettings
	state.State.Mu.Unlock()
	if err := config.SaveConfig(config.ConfigPath); err != nil {
		return fmt.Errorf("error saving config: %v", err)
	}
	return nil
}
