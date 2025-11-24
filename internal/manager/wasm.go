package manager

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mule-ai/mule/internal/database"
	dbmodels "github.com/mule-ai/mule/pkg/database"
)

// WasmModuleManager handles WASM module operations
type WasmModuleManager struct {
	db *database.DB
}

// NewWasmModuleManager creates a new WASM module manager
func NewWasmModuleManager(db *database.DB) *WasmModuleManager {
	return &WasmModuleManager{db: db}
}

// CreateWasmModule creates a new WASM module
func (wmm *WasmModuleManager) CreateWasmModule(ctx context.Context, name, description string, moduleData []byte) (*dbmodels.WasmModule, error) {
	id := uuid.New().String()

	now := time.Now()
	module := &dbmodels.WasmModule{
		ID:          id,
		Name:        name,
		Description: description,
		ModuleData:  moduleData,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	query := `INSERT INTO wasm_modules (id, name, description, module_data, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := wmm.db.ExecContext(ctx, query, module.ID, module.Name, module.Description, module.ModuleData, module.CreatedAt, module.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert WASM module: %w", err)
	}

	return module, nil
}

// GetWasmModule retrieves a WASM module by ID
func (wmm *WasmModuleManager) GetWasmModule(ctx context.Context, id string) (*dbmodels.WasmModule, error) {
	query := `SELECT id, name, description, module_data, created_at, updated_at FROM wasm_modules WHERE id = $1`
	module := &dbmodels.WasmModule{}
	err := wmm.db.QueryRowContext(ctx, query, id).Scan(
		&module.ID,
		&module.Name,
		&module.Description,
		&module.ModuleData,
		&module.CreatedAt,
		&module.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("WASM module not found: %s", id)
		}
		return nil, fmt.Errorf("failed to query WASM module: %w", err)
	}

	return module, nil
}

// ListWasmModules lists all WASM modules
func (wmm *WasmModuleManager) ListWasmModules(ctx context.Context) ([]*dbmodels.WasmModule, error) {
	query := `SELECT id, name, description, module_data, created_at, updated_at FROM wasm_modules ORDER BY created_at DESC`
	rows, err := wmm.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query WASM modules: %w", err)
	}
	defer rows.Close()

	var modules []*dbmodels.WasmModule
	for rows.Next() {
		module := &dbmodels.WasmModule{}
		err := rows.Scan(
			&module.ID,
			&module.Name,
			&module.Description,
			&module.ModuleData,
			&module.CreatedAt,
			&module.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan WASM module: %w", err)
		}
		modules = append(modules, module)
	}

	return modules, nil
}

// UpdateWasmModule updates a WASM module
func (wmm *WasmModuleManager) UpdateWasmModule(ctx context.Context, id, name, description string, moduleData []byte) (*dbmodels.WasmModule, error) {
	module, err := wmm.GetWasmModule(ctx, id)
	if err != nil {
		return nil, err
	}

	module.Name = name
	module.Description = description
	if moduleData != nil {
		module.ModuleData = moduleData
	}
	module.UpdatedAt = time.Now()

	query := `UPDATE wasm_modules SET name = $1, description = $2, module_data = $3, updated_at = $4 WHERE id = $5`
	_, err = wmm.db.ExecContext(ctx, query, module.Name, module.Description, module.ModuleData, module.UpdatedAt, module.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update WASM module: %w", err)
	}

	return module, nil
}

// DeleteWasmModule deletes a WASM module
func (wmm *WasmModuleManager) DeleteWasmModule(ctx context.Context, id string) error {
	query := `DELETE FROM wasm_modules WHERE id = $1`
	result, err := wmm.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete WASM module: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("WASM module not found: %s", id)
	}

	return nil
}
