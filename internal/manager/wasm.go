package manager

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mule-ai/mule/internal/primitive"
)

// WasmModuleManager handles WASM module operations
type WasmModuleManager struct {
	store primitive.PrimitiveStore
}

// NewWasmModuleManager creates a new WASM module manager
func NewWasmModuleManager(store primitive.PrimitiveStore) *WasmModuleManager {
	return &WasmModuleManager{store: store}
}

// CreateWasmModule creates a new WASM module
func (wmm *WasmModuleManager) CreateWasmModule(ctx context.Context, name, description string, moduleData, config []byte) (*primitive.WasmModule, error) {
	id := uuid.New().String()

	now := time.Now()
	module := &primitive.WasmModule{
		ID:          id,
		Name:        name,
		Description: description,
		ModuleData:  moduleData,
		Config:      config,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	err := wmm.store.CreateWasmModule(ctx, module)
	if err != nil {
		return nil, fmt.Errorf("failed to insert WASM module: %w", err)
	}

	return module, nil
}

// GetWasmModule retrieves a WASM module by ID
func (wmm *WasmModuleManager) GetWasmModule(ctx context.Context, id string) (*primitive.WasmModule, error) {
	module, err := wmm.store.GetWasmModule(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to query WASM module: %w", err)
	}

	return module, nil
}

// ListWasmModules lists all WASM modules (without module data for performance)
func (wmm *WasmModuleManager) ListWasmModules(ctx context.Context) ([]*primitive.WasmModuleListItem, error) {
	modules, err := wmm.store.ListWasmModules(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query WASM modules: %w", err)
	}

	return modules, nil
}

// UpdateWasmModule updates a WASM module
func (wmm *WasmModuleManager) UpdateWasmModule(ctx context.Context, id, name, description string, moduleData, config []byte) (*primitive.WasmModule, error) {
	module, err := wmm.GetWasmModule(ctx, id)
	if err != nil {
		return nil, err
	}

	module.Name = name
	module.Description = description
	if moduleData != nil {
		module.ModuleData = moduleData
	}
	if config != nil {
		module.Config = config
	}
	module.UpdatedAt = time.Now()

	err = wmm.store.UpdateWasmModule(ctx, module)
	if err != nil {
		return nil, fmt.Errorf("failed to update WASM module: %w", err)
	}

	return module, nil
}

// DeleteWasmModule deletes a WASM module
func (wmm *WasmModuleManager) DeleteWasmModule(ctx context.Context, id string) error {
	err := wmm.store.DeleteWasmModule(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to delete WASM module: %w", err)
	}

	return nil
}
