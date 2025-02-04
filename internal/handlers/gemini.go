package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jbutlerdev/dev-team/internal/state"
)

func HandleGeminiModels(w http.ResponseWriter, r *http.Request) {
	if state.State.GenAI == nil {
		http.Error(w, "AI provider not initialized", http.StatusInternalServerError)
		return
	}
	models := state.State.GenAI.Models()

	err := json.NewEncoder(w).Encode(models)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
