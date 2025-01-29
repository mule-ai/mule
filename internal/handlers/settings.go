package handlers

import (
	"dev-team/internal/config"
	"dev-team/internal/settings"
	"dev-team/internal/state"
	"encoding/json"
	"fmt"
	"net/http"
)

func HandleGetSettings(w http.ResponseWriter, r *http.Request) {
	state.State.Mu.RLock()
	defer state.State.Mu.RUnlock()

	json.NewEncoder(w).Encode(state.State.Settings)
}

func HandleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var settings settings.Settings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	state.State.Mu.Lock()
	state.State.Settings = settings
	state.State.Mu.Unlock()

	if err := config.SaveConfig(); err != nil {
		http.Error(w, fmt.Sprintf("Error saving config: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
