package handlers

import (
	"dev-team/internal/state"
	"encoding/json"
	"net/http"
)

func HandleGeminiModels(w http.ResponseWriter, r *http.Request) {
	if state.State.GenAI == nil {
		http.Error(w, "AI provider not initialized", http.StatusInternalServerError)
		return
	}
	models := state.State.GenAI.Models()

	json.NewEncoder(w).Encode(models)
}
