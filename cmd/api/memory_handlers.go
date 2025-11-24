package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/internal/api"
)

// Memory configuration handlers
func (h *apiHandler) getMemoryConfigHandler(w http.ResponseWriter, r *http.Request) {
	config, err := h.store.GetMemoryConfig(r.Context(), "default")
	if err != nil {
		api.HandleError(w, err, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

func (h *apiHandler) updateMemoryConfigHandler(w http.ResponseWriter, r *http.Request) {
	var config primitive.MemoryConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}

	if err := h.store.UpdateMemoryConfig(r.Context(), &config); err != nil {
		api.HandleError(w, err, http.StatusInternalServerError)
		return
	}

	// Reinitialize the memory tool with the new configuration
	if err := h.runtime.ReinitializeMemoryTool(); err != nil {
		// Log the error but don't fail the request - the config was saved successfully
		log.Printf("Warning: Failed to reinitialize memory tool: %v", err)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(config)
}