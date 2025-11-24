package manager

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mule-ai/mule/pkg/database"
)

// WasmModuleSourceManager handles WASM module source code operations
type WasmModuleSourceManager struct {
	db *sql.DB
}

// NewWasmModuleSourceManager creates a new WASM module source manager
func NewWasmModuleSourceManager(db *sql.DB) *WasmModuleSourceManager {
	return &WasmModuleSourceManager{db: db}
}

// CreateSource creates a new WASM module source record
func (wmsm *WasmModuleSourceManager) CreateSource(ctx context.Context, source *database.WasmModuleSource) error {
	if source.ID == "" {
		source.ID = uuid.New().String()
	}

	now := time.Now()
	source.CreatedAt = now
	source.UpdatedAt = now

	query := `INSERT INTO wasm_module_sources (id, wasm_module_id, language, source_code, version, compilation_status, compilation_error, compiled_at, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`
	_, err := wmsm.db.ExecContext(ctx, query,
		source.ID,
		source.WasmModuleID,
		source.Language,
		source.SourceCode,
		source.Version,
		source.CompilationStatus,
		source.CompilationError,
		source.CompiledAt,
		source.CreatedAt,
		source.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert WASM module source: %w", err)
	}

	return nil
}

// GetSource retrieves a WASM module source by ID
func (wmsm *WasmModuleSourceManager) GetSource(ctx context.Context, id string) (*database.WasmModuleSource, error) {
	query := `SELECT id, wasm_module_id, language, source_code, version, compilation_status, compilation_error, compiled_at, created_at, updated_at FROM wasm_module_sources WHERE id = $1`
	source := &database.WasmModuleSource{}
	err := wmsm.db.QueryRowContext(ctx, query, id).Scan(
		&source.ID,
		&source.WasmModuleID,
		&source.Language,
		&source.SourceCode,
		&source.Version,
		&source.CompilationStatus,
		&source.CompilationError,
		&source.CompiledAt,
		&source.CreatedAt,
		&source.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("WASM module source not found: %s", id)
		}
		return nil, fmt.Errorf("failed to query WASM module source: %w", err)
	}

	return source, nil
}

// GetLatestSourceByModuleID retrieves the latest source code for a WASM module
func (wmsm *WasmModuleSourceManager) GetLatestSourceByModuleID(ctx context.Context, wasmModuleID string) (*database.WasmModuleSource, error) {
	query := `SELECT id, wasm_module_id, language, source_code, version, compilation_status, compilation_error, compiled_at, created_at, updated_at FROM wasm_module_sources WHERE wasm_module_id = $1 ORDER BY version DESC, updated_at DESC LIMIT 1`
	source := &database.WasmModuleSource{}
	err := wmsm.db.QueryRowContext(ctx, query, wasmModuleID).Scan(
		&source.ID,
		&source.WasmModuleID,
		&source.Language,
		&source.SourceCode,
		&source.Version,
		&source.CompilationStatus,
		&source.CompilationError,
		&source.CompiledAt,
		&source.CreatedAt,
		&source.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no source code found for WASM module: %s", wasmModuleID)
		}
		return nil, fmt.Errorf("failed to query WASM module source: %w", err)
	}

	return source, nil
}

// ListSourcesByModuleID lists all source code versions for a WASM module
func (wmsm *WasmModuleSourceManager) ListSourcesByModuleID(ctx context.Context, wasmModuleID string) ([]*database.WasmModuleSource, error) {
	query := `SELECT id, wasm_module_id, language, source_code, version, compilation_status, compilation_error, compiled_at, created_at, updated_at FROM wasm_module_sources WHERE wasm_module_id = $1 ORDER BY version DESC, updated_at DESC`
	rows, err := wmsm.db.QueryContext(ctx, query, wasmModuleID)
	if err != nil {
		return nil, fmt.Errorf("failed to query WASM module sources: %w", err)
	}
	defer rows.Close()

	var sources []*database.WasmModuleSource
	for rows.Next() {
		source := &database.WasmModuleSource{}
		err := rows.Scan(
			&source.ID,
			&source.WasmModuleID,
			&source.Language,
			&source.SourceCode,
			&source.Version,
			&source.CompilationStatus,
			&source.CompilationError,
			&source.CompiledAt,
			&source.CreatedAt,
			&source.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan WASM module source: %w", err)
		}
		sources = append(sources, source)
	}

	return sources, nil
}

// UpdateSource updates a WASM module source record
func (wmsm *WasmModuleSourceManager) UpdateSource(ctx context.Context, source *database.WasmModuleSource) error {
	source.UpdatedAt = time.Now()

	query := `UPDATE wasm_module_sources SET language = $1, source_code = $2, version = $3, compilation_status = $4, compilation_error = $5, compiled_at = $6, updated_at = $7 WHERE id = $8`
	_, err := wmsm.db.ExecContext(ctx, query,
		source.Language,
		source.SourceCode,
		source.Version,
		source.CompilationStatus,
		source.CompilationError,
		source.CompiledAt,
		source.UpdatedAt,
		source.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update WASM module source: %w", err)
	}

	return nil
}

// DeleteSource deletes a WASM module source record
func (wmsm *WasmModuleSourceManager) DeleteSource(ctx context.Context, id string) error {
	query := `DELETE FROM wasm_module_sources WHERE id = $1`
	result, err := wmsm.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete WASM module source: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("WASM module source not found: %s", id)
	}

	return nil
}

// IncrementVersion increments the version number for a new source record
func (wmsm *WasmModuleSourceManager) GetNextVersion(ctx context.Context, wasmModuleID string) (int, error) {
	query := `SELECT COALESCE(MAX(version), 0) + 1 FROM wasm_module_sources WHERE wasm_module_id = $1`
	var nextVersion int
	err := wmsm.db.QueryRowContext(ctx, query, wasmModuleID).Scan(&nextVersion)
	if err != nil {
		return 0, fmt.Errorf("failed to get next version: %w", err)
	}

	return nextVersion, nil
}
