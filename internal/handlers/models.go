package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/mule-ai/mule/internal/state"
)

func HandleModels(w http.ResponseWriter, r *http.Request) {
	providerName := r.URL.Query().Get("provider")
	if providerName == "" {
		http.Error(w, "provider name parameter is required", http.StatusBadRequest)
		return
	}

	if state.State.GenAI == nil {
		http.Error(w, "AI providers not initialized", http.StatusInternalServerError)
		return
	}

	providerInstance, ok := state.State.GenAI[providerName]
	if !ok || providerInstance == nil {
		http.Error(w, fmt.Sprintf("Provider '%s' not found or not initialized", providerName), http.StatusInternalServerError)
		return
	}

	models := providerInstance.Models()

	err := json.NewEncoder(w).Encode(models)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
