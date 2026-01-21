package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) CreateTool(ctx context.Context, tool *Tool) error {
	query := `
		INSERT INTO tools (id, name, description, type, config, created_at, updated_at)
		VALUES (:id, :name, :description, :type, :config, :created_at, :updated_at)
	`

	if tool.ID == "" {
		tool.ID = uuid.New().String()
	}

	_, err := r.db.NamedExecContext(ctx, query, tool)
	return err
}

func (r *Repository) GetToolByID(ctx context.Context, id string) (*Tool, error) {
	var tool Tool
	query := `SELECT id, name, description, type, config, created_at, updated_at FROM tools WHERE id = $1`
	err := r.db.GetContext(ctx, &tool, query, id)
	if err != nil {
		return nil, err
	}
	return &tool, nil
}

func (r *Repository) UpdateTool(ctx context.Context, tool *Tool) error {
	query := `
		UPDATE tools 
		SET name = :name, description = :description, type = :type, config = :config, updated_at = :updated_at
		WHERE id = :id
	`

	_, err := r.db.NamedExecContext(ctx, query, tool)
	return err
}

func (r *Repository) DeleteTool(ctx context.Context, id string) error {
	query := `DELETE FROM tools WHERE id = $1`
	_, err := r.db.ExecContext(ctx, query, id)
	return err
}

func (r *Repository) ListTools(ctx context.Context) ([]*Tool, error) {
	var tools []*Tool
	query := `SELECT id, name, description, type, config, created_at, updated_at FROM tools`
	err := r.db.SelectContext(ctx, &tools, query)
	if err != nil {
		return nil, err
	}
	return tools, nil
}

func (r *Repository) ListToolsByType(ctx context.Context, toolType ToolType) ([]*Tool, error) {
	var tools []*Tool
	query := `SELECT id, name, description, type, config, created_at, updated_at FROM tools WHERE type = $1`
	err := r.db.SelectContext(ctx, &tools, query, toolType)
	if err != nil {
		return nil, err
	}
	return tools, nil
}