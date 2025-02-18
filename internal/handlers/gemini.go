package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/jbutlerdev/dev-team/internal/state"
)

func HandleModels(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider == "" {
		http.Error(w, "provider parameter is required", http.StatusBadRequest)
		return
	}

	if state.State.GenAI == nil {
		http.Error(w, "AI provider not initialized", http.StatusInternalServerError)
		return
	}

	var models []string
	switch provider {
	case "gemini":
		if state.State.GenAI.Gemini == nil {
			http.Error(w, "Gemini provider not initialized", http.StatusInternalServerError)
			return
		}
		models = state.State.GenAI.Gemini.Models()
	case "ollama":
		if state.State.GenAI.Ollama == nil {
			http.Error(w, "Ollama provider not initialized", http.StatusInternalServerError)
			return
		}
		models = state.State.GenAI.Ollama.Models()
	default:
		http.Error(w, "unsupported provider", http.StatusBadRequest)
		return
	}

	err := json.NewEncoder(w).Encode(models)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
