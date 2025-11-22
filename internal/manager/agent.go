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

// AgentManager handles agent operations
type AgentManager struct {
	db *database.DB
}

// NewAgentManager creates a new agent manager
func NewAgentManager(db *database.DB) *AgentManager {
	return &AgentManager{db: db}
}

// CreateAgent creates a new agent
func (am *AgentManager) CreateAgent(ctx context.Context, name, description, providerID, modelID, systemPrompt string) (*dbmodels.Agent, error) {
	id := uuid.New().String()

	now := time.Now()
	agent := &dbmodels.Agent{
		ID:           id,
		Name:         name,
		Description:  description,
		ProviderID:   providerID,
		ModelID:      modelID,
		SystemPrompt: systemPrompt,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	query := `INSERT INTO agents (id, name, description, provider_id, model_id, system_prompt, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := am.db.ExecContext(ctx, query, agent.ID, agent.Name, agent.Description, agent.ProviderID, agent.ModelID, agent.SystemPrompt, agent.CreatedAt, agent.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert agent: %w", err)
	}

	return agent, nil
}

// GetAgent retrieves an agent by ID
func (am *AgentManager) GetAgent(ctx context.Context, id string) (*dbmodels.Agent, error) {
	query := `SELECT id, name, description, provider_id, model_id, system_prompt, created_at, updated_at FROM agents WHERE id = $1`
	agent := &dbmodels.Agent{}
	err := am.db.QueryRowContext(ctx, query, id).Scan(
		&agent.ID,
		&agent.Name,
		&agent.Description,
		&agent.ProviderID,
		&agent.ModelID,
		&agent.SystemPrompt,
		&agent.CreatedAt,
		&agent.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("agent not found: %s", id)
		}
		return nil, fmt.Errorf("failed to query agent: %w", err)
	}

	return agent, nil
}

// ListAgents lists all agents
func (am *AgentManager) ListAgents(ctx context.Context) ([]*dbmodels.Agent, error) {
	query := `SELECT id, name, description, provider_id, model_id, system_prompt, created_at, updated_at FROM agents ORDER BY created_at DESC`
	rows, err := am.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query agents: %w", err)
	}
	defer rows.Close()

	var agents []*dbmodels.Agent
	for rows.Next() {
		agent := &dbmodels.Agent{}
		err := rows.Scan(
			&agent.ID,
			&agent.Name,
			&agent.Description,
			&agent.ProviderID,
			&agent.ModelID,
			&agent.SystemPrompt,
			&agent.CreatedAt,
			&agent.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan agent: %w", err)
		}
		agents = append(agents, agent)
	}

	return agents, nil
}

// UpdateAgent updates an agent
func (am *AgentManager) UpdateAgent(ctx context.Context, id, name, description, providerID, modelID, systemPrompt string) (*dbmodels.Agent, error) {
	agent, err := am.GetAgent(ctx, id)
	if err != nil {
		return nil, err
	}

	agent.Name = name
	agent.Description = description
	agent.ProviderID = providerID
	agent.ModelID = modelID
	agent.SystemPrompt = systemPrompt
	agent.UpdatedAt = time.Now()

	query := `UPDATE agents SET name = $1, description = $2, provider_id = $3, model_id = $4, system_prompt = $5, updated_at = $6 WHERE id = $7`
	_, err = am.db.ExecContext(ctx, query, agent.Name, agent.Description, agent.ProviderID, agent.ModelID, agent.SystemPrompt, agent.UpdatedAt, agent.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update agent: %w", err)
	}

	return agent, nil
}

// DeleteAgent deletes an agent
func (am *AgentManager) DeleteAgent(ctx context.Context, id string) error {
	query := `DELETE FROM agents WHERE id = $1`
	result, err := am.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete agent: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("agent not found: %s", id)
	}

	return nil
}

// AddToolToAgent adds a tool to an agent
func (am *AgentManager) AddToolToAgent(ctx context.Context, agentID, toolID string) error {
	query := `INSERT INTO agent_tools (agent_id, tool_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	_, err := am.db.ExecContext(ctx, query, agentID, toolID)
	if err != nil {
		return fmt.Errorf("failed to add tool to agent: %w", err)
	}

	return nil
}

// RemoveToolFromAgent removes a tool from an agent
func (am *AgentManager) RemoveToolFromAgent(ctx context.Context, agentID, toolID string) error {
	query := `DELETE FROM agent_tools WHERE agent_id = $1 AND tool_id = $2`
	_, err := am.db.ExecContext(ctx, query, agentID, toolID)
	if err != nil {
		return fmt.Errorf("failed to remove tool from agent: %w", err)
	}

	return nil
}

// GetAgentTools gets all tools for an agent
func (am *AgentManager) GetAgentTools(ctx context.Context, agentID string) ([]*dbmodels.Tool, error) {
	query := `
		SELECT t.id, t.name, t.description, t.metadata, t.created_at, t.updated_at
		FROM tools t
		JOIN agent_tools at ON t.id = at.tool_id
		WHERE at.agent_id = $1
		ORDER BY t.created_at DESC
	`

	rows, err := am.db.QueryContext(ctx, query, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query agent tools: %w", err)
	}
	defer rows.Close()

	var tools []*dbmodels.Tool
	for rows.Next() {
		tool := &dbmodels.Tool{}
		err := rows.Scan(
			&tool.ID,
			&tool.Name,
			&tool.Description,
			&tool.Metadata,
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
