package handlers

import (
	"dev-team/internal/state"
	"dev-team/pkg/genai"
	"encoding/json"
	"fmt"
	"net/http"
)

func HandleGeminiModels(w http.ResponseWriter, r *http.Request) {
	state.State.Mu.RLock()
	settings := state.State.Settings
	state.State.Mu.RUnlock()

	if settings.GeminiAPIKey == "" {
		http.Error(w, "Gemini API key not configured", http.StatusBadRequest)
		return
	}

	models, err := genai.GetGeminiModels(settings.GeminiAPIKey)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching Gemini models: %v", err), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(models)
}
