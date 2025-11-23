package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/mule-ai/mule/internal/api"
	"github.com/mule-ai/mule/internal/manager"
	"github.com/mule-ai/mule/pkg/database"
	"github.com/mule-ai/mule/internal/wasmcompiler"
)

// CompileWasmModuleRequest represents a request to compile WASM source code
type CompileWasmModuleRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Language    string `json:"language"`
	SourceCode  string `json:"source_code"`
}

// CompileWasmModuleResponse represents the response from a compilation request
type CompileWasmModuleResponse struct {
	ModuleID          string    `json:"module_id"`
	SourceID          string    `json:"source_id"`
	CompilationStatus string    `json:"compilation_status"`
	CompilationError  *string   `json:"compilation_error,omitempty"`
	CompiledAt        *time.Time `json:"compiled_at,omitempty"`
	SourceChecksum    string    `json:"source_checksum"`
}

// TestWasmModuleRequest represents a request to test a WASM module
type TestWasmModuleRequest struct {
	ModuleID string                 `json:"module_id"`
	Input    map[string]interface{} `json:"input"`
}

// TestWasmModuleResponse represents the response from a WASM module test
type TestWasmModuleResponse struct {
	Output  interface{} `json:"output"`
	Success bool        `json:"success"`
	Error   string      `json:"error,omitempty"`
}

// GetWasmModuleSourceResponse represents the response for getting WASM source code
type GetWasmModuleSourceResponse struct {
	ID                string    `json:"id"`
	WasmModuleID      string    `json:"wasm_module_id"`
	Language          string    `json:"language"`
	SourceCode        string    `json:"source_code"`
	Version           int       `json:"version"`
	CompilationStatus string    `json:"compilation_status"`
	CompilationError  *string   `json:"compilation_error,omitempty"`
	CompiledAt        *time.Time `json:"compiled_at,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (h *apiHandler) compileWasmModuleHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CompileWasmModuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Name == "" {
		api.HandleError(w, fmt.Errorf("name is required"), http.StatusBadRequest)
		return
	}
	if req.Language == "" {
		req.Language = "go" // Default to Go
	}
	if req.SourceCode == "" {
		api.HandleError(w, fmt.Errorf("source_code is required"), http.StatusBadRequest)
		return
	}

	// Validate Go source code if applicable
	if req.Language == "go" {
		if err := wasmcompiler.ValidateGoSource(req.SourceCode); err != nil {
			api.HandleError(w, fmt.Errorf("source code validation failed: %w", err), http.StatusBadRequest)
			return
		}
	}

	// Create compiler
	compiler := wasmcompiler.NewCompiler("/tmp/wasm-compile")

	// Create source manager
	sourceMgr := manager.NewWasmModuleSourceManager(h.db.DB)

	// Compile and create module with source
	wasmModule, source, err := wasmcompiler.CreateWasmModuleWithSource(
		ctx,
		compiler,
		h.wasmModuleMgr,
		sourceMgr,
		req.Name,
		req.Description,
		req.Language,
		req.SourceCode,
	)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to compile WASM module: %w", err), http.StatusInternalServerError)
		return
	}

	response := CompileWasmModuleResponse{
		ModuleID:          wasmModule.ID,
		SourceID:          source.ID,
		CompilationStatus: source.CompilationStatus,
		CompilationError:  source.CompilationError,
		CompiledAt:        source.CompiledAt,
		SourceChecksum:    source.SourceCode, // This would be calculated during compilation
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(response)
}

func (h *apiHandler) getWasmModuleSourceHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	moduleID := vars["id"]

	sourceMgr := manager.NewWasmModuleSourceManager(h.db.DB)
	source, err := sourceMgr.GetLatestSourceByModuleID(ctx, moduleID)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to get source code: %w", err), http.StatusNotFound)
		return
	}

	response := GetWasmModuleSourceResponse{
		ID:                source.ID,
		WasmModuleID:      source.WasmModuleID,
		Language:          source.Language,
		SourceCode:        source.SourceCode,
		Version:           source.Version,
		CompilationStatus: source.CompilationStatus,
		CompilationError:  source.CompilationError,
		CompiledAt:        source.CompiledAt,
		CreatedAt:         source.CreatedAt,
		UpdatedAt:         source.UpdatedAt,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (h *apiHandler) updateWasmModuleSourceHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	moduleID := vars["id"]

	var req CompileWasmModuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.SourceCode == "" {
		api.HandleError(w, fmt.Errorf("source_code is required"), http.StatusBadRequest)
		return
	}

	// Validate Go source code if applicable
	if req.Language == "go" {
		if err := wasmcompiler.ValidateGoSource(req.SourceCode); err != nil {
			api.HandleError(w, fmt.Errorf("source code validation failed: %w", err), http.StatusBadRequest)
			return
		}
	}

	// Get existing module
	wasmModule, err := h.wasmModuleMgr.GetWasmModule(ctx, moduleID)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to get WASM module: %w", err), http.StatusNotFound)
		return
	}

	// Create compiler
	compiler := wasmcompiler.NewCompiler("/tmp/wasm-compile")

	// Compile the new source code
	compileReq := wasmcompiler.CompileRequest{
		SourceCode: req.SourceCode,
		Language:   req.Language,
		ModuleName: wasmModule.Name,
	}

	compileResult, err := compiler.Compile(ctx, compileReq)
	if err != nil {
		api.HandleError(w, fmt.Errorf("compilation failed: %w", err), http.StatusInternalServerError)
		return
	}

	// Update the WASM module with new compiled data if successful
	if compileResult.Success {
		_, err = h.wasmModuleMgr.UpdateWasmModule(ctx, moduleID, wasmModule.Name, wasmModule.Description, compileResult.ModuleData)
		if err != nil {
			api.HandleError(w, fmt.Errorf("failed to update WASM module: %w", err), http.StatusInternalServerError)
			return
		}
	}

	// Create new source record
	sourceMgr := manager.NewWasmModuleSourceManager(h.db.DB)
	nextVersion, err := sourceMgr.GetNextVersion(ctx, moduleID)
	if err != nil {
		api.HandleError(w, fmt.Errorf("failed to get next version: %w", err), http.StatusInternalServerError)
		return
	}

	source := &database.WasmModuleSource{
		ID:                uuid.New().String(),
		WasmModuleID:      moduleID,
		Language:          req.Language,
		SourceCode:        req.SourceCode,
		Version:           nextVersion,
		CompilationStatus: getCompilationStatus(compileResult),
	}

	if !compileResult.Success {
		source.CompilationError = &compileResult.Error
	}

	if compileResult.Success {
		source.CompiledAt = &compileResult.CompiledAt
	}

	if err := sourceMgr.CreateSource(ctx, source); err != nil {
		api.HandleError(w, fmt.Errorf("failed to store source code: %w", err), http.StatusInternalServerError)
		return
	}

	response := CompileWasmModuleResponse{
		ModuleID:          moduleID,
		SourceID:          source.ID,
		CompilationStatus: source.CompilationStatus,
		CompilationError:  source.CompilationError,
		CompiledAt:        source.CompiledAt,
		SourceChecksum:    compileResult.SourceChecksum,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (h *apiHandler) testWasmModuleHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req TestWasmModuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		api.HandleError(w, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		return
	}

	if req.ModuleID == "" {
		api.HandleError(w, fmt.Errorf("module_id is required"), http.StatusBadRequest)
		return
	}

	// Execute the WASM module
	result, err := h.wasmExecutor.Execute(ctx, req.ModuleID, req.Input)
	if err != nil {
		api.HandleError(w, fmt.Errorf("WASM execution failed: %w", err), http.StatusInternalServerError)
		return
	}

	response := TestWasmModuleResponse{
		Output:  result["output"],
		Success: result["success"].(bool),
	}

	if !response.Success {
		if stderr, ok := result["stderr"].(string); ok && stderr != "" {
			response.Error = stderr
		} else if errMsg, ok := result["error"].(string); ok {
			response.Error = errMsg
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func (h *apiHandler) getWasmModuleExampleHandler(w http.ResponseWriter, r *http.Request) {
	language := r.URL.Query().Get("language")
	if language == "" {
		language = "go"
	}

	var exampleCode string
	if language == "go" {
		exampleCode = wasmcompiler.GenerateExampleGoCode()
	} else {
		api.HandleError(w, fmt.Errorf("example code not available for language: %s", language), http.StatusBadRequest)
		return
	}

	response := map[string]string{
		"language":    language,
		"example_code": exampleCode,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(response)
}

func getCompilationStatus(result *wasmcompiler.CompileResult) string {
	if result.Success {
		return "success"
	}
	return "failed"
}