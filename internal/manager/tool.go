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

// ToolManager handles tool operations
type ToolManager struct {
	db *database.DB
}

// NewToolManager creates a new tool manager
func NewToolManager(db *database.DB) *ToolManager {
	return &ToolManager{db: db}
}

// CreateTool creates a new tool
func (tm *ToolManager) CreateTool(ctx context.Context, name, description, toolType string, config map[string]interface{}) (*dbmodels.Tool, error) {
	id := uuid.New().String()

	configBytes, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	now := time.Now()
	tool := &dbmodels.Tool{
		ID:          id,
		Name:        name,
		Description: description,
		Type:        toolType,
		Config:      configBytes,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	query := `INSERT INTO tools (id, name, description, type, config, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err = tm.db.ExecContext(ctx, query, tool.ID, tool.Name, tool.Description, tool.Type, tool.Config, tool.CreatedAt, tool.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert tool: %w", err)
	}

	return tool, nil
}

// GetTool retrieves a tool by ID
func (tm *ToolManager) GetTool(ctx context.Context, id string) (*dbmodels.Tool, error) {
	query := `SELECT id, name, description, type, config, created_at, updated_at FROM tools WHERE id = $1`
	tool := &dbmodels.Tool{}
	err := tm.db.QueryRowContext(ctx, query, id).Scan(
		&tool.ID,
		&tool.Name,
		&tool.Description,
		&tool.Type,
		&tool.Config,
		&tool.CreatedAt,
		&tool.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("tool not found: %s", id)
		}
		return nil, fmt.Errorf("failed to query tool: %w", err)
	}

	return tool, nil
}

// ListTools lists all tools
func (tm *ToolManager) ListTools(ctx context.Context) ([]*dbmodels.Tool, error) {
	query := `SELECT id, name, description, type, config, created_at, updated_at FROM tools ORDER BY created_at DESC`
	rows, err := tm.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query tools: %w", err)
	}
	defer rows.Close()

	var tools []*dbmodels.Tool
	for rows.Next() {
		tool := &dbmodels.Tool{}
		err := rows.Scan(
			&tool.ID,
			&tool.Name,
			&tool.Description,
			&tool.Type,
			&tool.Config,
			&tool.CreatedAt,
			&tool.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tool: %w", err)
		}
		tools = append(tools, tool)
	}

	return tools, nil
}

// UpdateTool updates a tool
func (tm *ToolManager) UpdateTool(ctx context.Context, id, name, description, toolType string, config map[string]interface{}) (*dbmodels.Tool, error) {
	tool, err := tm.GetTool(ctx, id)
	if err != nil {
		return nil, err
	}

	configBytes, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	tool.Name = name
	tool.Description = description
	tool.Type = toolType
	tool.Config = configBytes
	tool.UpdatedAt = time.Now()

	query := `UPDATE tools SET name = $1, description = $2, type = $3, config = $4, updated_at = $5 WHERE id = $6`
	_, err = tm.db.ExecContext(ctx, query, tool.Name, tool.Description, tool.Type, tool.Config, tool.UpdatedAt, tool.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update tool: %w", err)
	}

	return tool, nil
}

// DeleteTool deletes a tool
func (tm *ToolManager) DeleteTool(ctx context.Context, id string) error {
	query := `DELETE FROM tools WHERE id = $1`
	result, err := tm.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete tool: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("tool not found: %s", id)
	}

	return nil
}

// GetToolConfig gets the configuration for a tool as a map
func (tm *ToolManager) GetToolConfig(ctx context.Context, id string) (map[string]interface{}, error) {
	tool, err := tm.GetTool(ctx, id)
	if err != nil {
		return nil, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal(tool.Config, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tool config: %w", err)
	}

	return config, nil
}
