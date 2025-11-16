package manager

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/mule-ai/mule/internal/database"
	dbmodels "github.com/mule-ai/mule/pkg/database"
)

// WorkflowManager handles workflow operations
type WorkflowManager struct {
	db *database.DB
}

// NewWorkflowManager creates a new workflow manager
func NewWorkflowManager(db *database.DB) *WorkflowManager {
	return &WorkflowManager{db: db}
}

// CreateWorkflow creates a new workflow
func (wm *WorkflowManager) CreateWorkflow(ctx context.Context, name, description string, isAsync bool) (*dbmodels.Workflow, error) {
	id := uuid.New().String()

	now := time.Now()
	workflow := &dbmodels.Workflow{
		ID:          id,
		Name:        name,
		Description: description,
		IsAsync:     isAsync,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	query := `INSERT INTO workflows (id, name, description, is_async, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := wm.db.ExecContext(ctx, query, workflow.ID, workflow.Name, workflow.Description, workflow.IsAsync, workflow.CreatedAt, workflow.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert workflow: %w", err)
	}

	return workflow, nil
}

// GetWorkflow retrieves a workflow by ID
func (wm *WorkflowManager) GetWorkflow(ctx context.Context, id string) (*dbmodels.Workflow, error) {
	query := `SELECT id, name, description, is_async, created_at, updated_at FROM workflows WHERE id = $1`
	workflow := &dbmodels.Workflow{}
	err := wm.db.QueryRowContext(ctx, query, id).Scan(
		&workflow.ID,
		&workflow.Name,
		&workflow.Description,
		&workflow.IsAsync,
		&workflow.CreatedAt,
		&workflow.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("workflow not found: %s", id)
		}
		return nil, fmt.Errorf("failed to query workflow: %w", err)
	}

	return workflow, nil
}

// ListWorkflows lists all workflows
func (wm *WorkflowManager) ListWorkflows(ctx context.Context) ([]*dbmodels.Workflow, error) {
	query := `SELECT id, name, description, is_async, created_at, updated_at FROM workflows ORDER BY created_at DESC`
	rows, err := wm.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query workflows: %w", err)
	}
	defer rows.Close()

	var workflows []*dbmodels.Workflow
	for rows.Next() {
		workflow := &dbmodels.Workflow{}
		err := rows.Scan(
			&workflow.ID,
			&workflow.Name,
			&workflow.Description,
			&workflow.IsAsync,
			&workflow.CreatedAt,
			&workflow.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan workflow: %w", err)
		}
		workflows = append(workflows, workflow)
	}

	return workflows, nil
}

// UpdateWorkflow updates a workflow
func (wm *WorkflowManager) UpdateWorkflow(ctx context.Context, id, name, description string, isAsync bool) (*dbmodels.Workflow, error) {
	workflow, err := wm.GetWorkflow(ctx, id)
	if err != nil {
		return nil, err
	}

	workflow.Name = name
	workflow.Description = description
	workflow.IsAsync = isAsync
	workflow.UpdatedAt = time.Now()

	query := `UPDATE workflows SET name = $1, description = $2, is_async = $3, updated_at = $4 WHERE id = $5`
	_, err = wm.db.ExecContext(ctx, query, workflow.Name, workflow.Description, workflow.IsAsync, workflow.UpdatedAt, workflow.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update workflow: %w", err)
	}

	return workflow, nil
}

// DeleteWorkflow deletes a workflow
func (wm *WorkflowManager) DeleteWorkflow(ctx context.Context, id string) error {
	query := `DELETE FROM workflows WHERE id = $1`
	result, err := wm.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete workflow: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("workflow not found: %s", id)
	}

	return nil
}

// CreateWorkflowStep creates a new workflow step
func (wm *WorkflowManager) CreateWorkflowStep(ctx context.Context, workflowID string, stepOrder int, stepType string, agentID, wasmModuleID *string, config map[string]interface{}) (*dbmodels.WorkflowStep, error) {
	id := uuid.New().String()

	configBytes, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	now := time.Now()
	step := &dbmodels.WorkflowStep{
		ID:           id,
		WorkflowID:   workflowID,
		StepOrder:    stepOrder,
		Type:         stepType,
		AgentID:      agentID,
		WasmModuleID: wasmModuleID,
		Config:       configBytes,
		CreatedAt:    now,
	}

	query := `INSERT INTO workflow_steps (id, workflow_id, step_order, type, agent_id, wasm_module_id, config, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err = wm.db.ExecContext(ctx, query, step.ID, step.WorkflowID, step.StepOrder, step.Type, step.AgentID, step.WasmModuleID, step.Config, step.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert workflow step: %w", err)
	}

	return step, nil
}

// GetWorkflowSteps gets all steps for a workflow
func (wm *WorkflowManager) GetWorkflowSteps(ctx context.Context, workflowID string) ([]*dbmodels.WorkflowStep, error) {
	query := `SELECT id, workflow_id, step_order, type, agent_id, wasm_module_id, config, created_at FROM workflow_steps WHERE workflow_id = $1 ORDER BY step_order ASC`
	rows, err := wm.db.QueryContext(ctx, query, workflowID)
	if err != nil {
		return nil, fmt.Errorf("failed to query workflow steps: %w", err)
	}
	defer rows.Close()

	var steps []*dbmodels.WorkflowStep
	for rows.Next() {
		step := &dbmodels.WorkflowStep{}
		err := rows.Scan(
			&step.ID,
			&step.WorkflowID,
			&step.StepOrder,
			&step.Type,
			&step.AgentID,
			&step.WasmModuleID,
			&step.Config,
			&step.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan workflow step: %w", err)
		}
		steps = append(steps, step)
	}

	return steps, nil
}

// UpdateWorkflowStep updates a workflow step
func (wm *WorkflowManager) UpdateWorkflowStep(ctx context.Context, id string, stepOrder int, stepType string, agentID, wasmModuleID *string, config map[string]interface{}) (*dbmodels.WorkflowStep, error) {
	step, err := wm.GetWorkflowStep(ctx, id)
	if err != nil {
		return nil, err
	}

	configBytes, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	step.StepOrder = stepOrder
	step.Type = stepType
	step.AgentID = agentID
	step.WasmModuleID = wasmModuleID
	step.Config = configBytes

	query := `UPDATE workflow_steps SET step_order = $1, type = $2, agent_id = $3, wasm_module_id = $4, config = $5 WHERE id = $6`
	_, err = wm.db.ExecContext(ctx, query, step.StepOrder, step.Type, step.AgentID, step.WasmModuleID, step.Config, step.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update workflow step: %w", err)
	}

	return step, nil
}

// GetWorkflowStep retrieves a workflow step by ID
func (wm *WorkflowManager) GetWorkflowStep(ctx context.Context, id string) (*dbmodels.WorkflowStep, error) {
	query := `SELECT id, workflow_id, step_order, type, agent_id, wasm_module_id, config, created_at FROM workflow_steps WHERE id = $1`
	step := &dbmodels.WorkflowStep{}
	err := wm.db.QueryRowContext(ctx, query, id).Scan(
		&step.ID,
		&step.WorkflowID,
		&step.StepOrder,
		&step.Type,
		&step.AgentID,
		&step.WasmModuleID,
		&step.Config,
		&step.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("workflow step not found: %s", id)
		}
		return nil, fmt.Errorf("failed to query workflow step: %w", err)
	}

	return step, nil
}

// DeleteWorkflowStep deletes a workflow step
func (wm *WorkflowManager) DeleteWorkflowStep(ctx context.Context, id string) error {
	query := `DELETE FROM workflow_steps WHERE id = $1`
	result, err := wm.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete workflow step: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("workflow step not found: %s", id)
	}

	return nil
}

// GetWorkflowStepConfig gets the configuration for a workflow step as a map
func (wm *WorkflowManager) GetWorkflowStepConfig(ctx context.Context, id string) (map[string]interface{}, error) {
	step, err := wm.GetWorkflowStep(ctx, id)
	if err != nil {
		return nil, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal(step.Config, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal workflow step config: %w", err)
	}

	return config, nil
}
