package manager

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/mule-ai/mule/internal/database"
	dbmodels "github.com/mule-ai/mule/pkg/database"
)

// SkillManager handles skill operations
type SkillManager struct {
	db *database.DB
}

// NewSkillManager creates a new skill manager
func NewSkillManager(db *database.DB) *SkillManager {
	return &SkillManager{db: db}
}

// CreateSkill creates a new skill
func (sm *SkillManager) CreateSkill(ctx context.Context, name, description, path string, enabled bool) (*dbmodels.Skill, error) {
	id := uuid.New().String()

	now := time.Now()
	skill := &dbmodels.Skill{
		ID:          id,
		Name:        name,
		Description: description,
		Path:        path,
		Enabled:     enabled,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	query := `INSERT INTO skills (id, name, description, path, enabled, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := sm.db.ExecContext(ctx, query, skill.ID, skill.Name, skill.Description, skill.Path, skill.Enabled, skill.CreatedAt, skill.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to insert skill: %w", err)
	}

	return skill, nil
}

// GetSkill retrieves a skill by ID
func (sm *SkillManager) GetSkill(ctx context.Context, id string) (*dbmodels.Skill, error) {
	query := `SELECT id, name, description, path, enabled, created_at, updated_at FROM skills WHERE id = $1`
	skill := &dbmodels.Skill{}
	err := sm.db.QueryRowContext(ctx, query, id).Scan(
		&skill.ID,
		&skill.Name,
		&skill.Description,
		&skill.Path,
		&skill.Enabled,
		&skill.CreatedAt,
		&skill.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("skill not found: %s", id)
		}
		return nil, fmt.Errorf("failed to query skill: %w", err)
	}

	return skill, nil
}

// ListSkills lists all skills
func (sm *SkillManager) ListSkills(ctx context.Context) ([]*dbmodels.Skill, error) {
	query := `SELECT id, name, description, path, enabled, created_at, updated_at FROM skills ORDER BY name`
	rows, err := sm.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query skills: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Error closing rows: %v", closeErr)
		}
	}()

	var skills []*dbmodels.Skill
	for rows.Next() {
		skill := &dbmodels.Skill{}
		err := rows.Scan(
			&skill.ID,
			&skill.Name,
			&skill.Description,
			&skill.Path,
			&skill.Enabled,
			&skill.CreatedAt,
			&skill.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan skill: %w", err)
		}
		skills = append(skills, skill)
	}

	return skills, nil
}

// UpdateSkill updates a skill
func (sm *SkillManager) UpdateSkill(ctx context.Context, id, name, description, path string, enabled bool) (*dbmodels.Skill, error) {
	skill, err := sm.GetSkill(ctx, id)
	if err != nil {
		return nil, err
	}

	skill.Name = name
	skill.Description = description
	skill.Path = path
	skill.Enabled = enabled
	skill.UpdatedAt = time.Now()

	query := `UPDATE skills SET name = $1, description = $2, path = $3, enabled = $4, updated_at = $5 WHERE id = $6`
	_, err = sm.db.ExecContext(ctx, query, skill.Name, skill.Description, skill.Path, skill.Enabled, skill.UpdatedAt, skill.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update skill: %w", err)
	}

	return skill, nil
}

// DeleteSkill deletes a skill
func (sm *SkillManager) DeleteSkill(ctx context.Context, id string) error {
	query := `DELETE FROM skills WHERE id = $1`
	result, err := sm.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete skill: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("skill not found: %s", id)
	}

	return nil
}

// AddSkillToAgent adds a skill to an agent
func (sm *SkillManager) AddSkillToAgent(ctx context.Context, agentID, skillID string) error {
	query := `INSERT INTO agent_skills (agent_id, skill_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	_, err := sm.db.ExecContext(ctx, query, agentID, skillID)
	if err != nil {
		return fmt.Errorf("failed to add skill to agent: %w", err)
	}

	return nil
}

// RemoveSkillFromAgent removes a skill from an agent
func (sm *SkillManager) RemoveSkillFromAgent(ctx context.Context, agentID, skillID string) error {
	query := `DELETE FROM agent_skills WHERE agent_id = $1 AND skill_id = $2`
	_, err := sm.db.ExecContext(ctx, query, agentID, skillID)
	if err != nil {
		return fmt.Errorf("failed to remove skill from agent: %w", err)
	}

	return nil
}

// GetAgentSkills gets all skills for an agent
func (sm *SkillManager) GetAgentSkills(ctx context.Context, agentID string) ([]*dbmodels.Skill, error) {
	query := `
		SELECT s.id, s.name, s.description, s.path, s.enabled, s.created_at, s.updated_at
		FROM skills s
		JOIN agent_skills ast ON s.id = ast.skill_id
		WHERE ast.agent_id = $1
		ORDER BY s.name
	`

	rows, err := sm.db.QueryContext(ctx, query, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query agent skills: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Error closing rows: %v", closeErr)
		}
	}()

	var skills []*dbmodels.Skill
	for rows.Next() {
		skill := &dbmodels.Skill{}
		err := rows.Scan(
			&skill.ID,
			&skill.Name,
			&skill.Description,
			&skill.Path,
			&skill.Enabled,
			&skill.CreatedAt,
			&skill.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan skill: %w", err)
		}
		skills = append(skills, skill)
	}

	return skills, nil
}
