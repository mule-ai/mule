package manager

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// Test helper: wrap sqlmock to match the database interface used by managers
// =============================================================================

// mockDB wraps sqlmock to provide the database interface expected by managers
type mockDB struct {
	*sql.DB
	mock sqlmock.Sqlmock
}

func (m *mockDB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	return m.Begin()
}

func (m *mockDB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return m.DB.ExecContext(ctx, query, args...)
}

func (m *mockDB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return m.DB.QueryContext(ctx, query, args...)
}

func (m *mockDB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return m.DB.QueryRowContext(ctx, query, args...)
}

// =============================================================================
// Testable Manager implementations that use the mockDB interface
// =============================================================================

// TestableAgentManager is a test version of AgentManager using mockDB
type TestableAgentManager struct {
	db *mockDB
}

func NewTestableAgentManager(db *mockDB) *TestableAgentManager {
	return &TestableAgentManager{db: db}
}

func (am *TestableAgentManager) GetAgent(ctx context.Context, id string) (*testAgent, error) {
	query := `SELECT id, name, description, provider_id, model_id, system_prompt, created_at, updated_at FROM agents WHERE id = $1`
	agent := &testAgent{}
	err := am.db.QueryRowContext(ctx, query, id).Scan(
		&agent.ID, &agent.Name, &agent.Description, &agent.ProviderID, &agent.ModelID, &agent.SystemPrompt, &agent.CreatedAt, &agent.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, err
	}
	return agent, nil
}

func (am *TestableAgentManager) UpdateAgent(ctx context.Context, id, name, description, providerID, modelID, systemPrompt string) (*testAgent, error) {
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
		return nil, err
	}
	return agent, nil
}

func (am *TestableAgentManager) DeleteAgent(ctx context.Context, id string) error {
	query := `DELETE FROM agents WHERE id = $1`
	result, err := am.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (am *TestableAgentManager) RemoveToolFromAgent(ctx context.Context, agentID, toolID string) error {
	query := `DELETE FROM agent_tools WHERE agent_id = $1 AND tool_id = $2`
	_, err := am.db.ExecContext(ctx, query, agentID, toolID)
	return err
}

// TestableSkillManager is a test version of SkillManager using mockDB
type TestableSkillManager struct {
	db *mockDB
}

func NewTestableSkillManager(db *mockDB) *TestableSkillManager {
	return &TestableSkillManager{db: db}
}

func (sm *TestableSkillManager) GetSkill(ctx context.Context, id string) (*testSkill, error) {
	query := `SELECT id, name, description, path, enabled, created_at, updated_at FROM skills WHERE id = $1`
	skill := &testSkill{}
	err := sm.db.QueryRowContext(ctx, query, id).Scan(
		&skill.ID, &skill.Name, &skill.Description, &skill.Path, &skill.Enabled, &skill.CreatedAt, &skill.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, err
	}
	return skill, nil
}

func (sm *TestableSkillManager) UpdateSkill(ctx context.Context, id, name, description, path string, enabled bool) (*testSkill, error) {
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
		return nil, err
	}
	return skill, nil
}

func (sm *TestableSkillManager) DeleteSkill(ctx context.Context, id string) error {
	query := `DELETE FROM skills WHERE id = $1`
	result, err := sm.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (sm *TestableSkillManager) RemoveSkillFromAgent(ctx context.Context, agentID, skillID string) error {
	query := `DELETE FROM agent_skills WHERE agent_id = $1 AND skill_id = $2`
	_, err := sm.db.ExecContext(ctx, query, agentID, skillID)
	return err
}

// TestableWorkflowManager is a test version of WorkflowManager using mockDB
type TestableWorkflowManager struct {
	db *mockDB
}

func NewTestableWorkflowManager(db *mockDB) *TestableWorkflowManager {
	return &TestableWorkflowManager{db: db}
}

func (wm *TestableWorkflowManager) GetWorkflow(ctx context.Context, id string) (*testWorkflow, error) {
	query := `SELECT id, name, description, is_async, created_at, updated_at FROM workflows WHERE id = $1`
	workflow := &testWorkflow{}
	err := wm.db.QueryRowContext(ctx, query, id).Scan(
		&workflow.ID, &workflow.Name, &workflow.Description, &workflow.IsAsync, &workflow.CreatedAt, &workflow.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, err
	}
	return workflow, nil
}

func (wm *TestableWorkflowManager) UpdateWorkflow(ctx context.Context, id, name, description string, isAsync bool) (*testWorkflow, error) {
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
		return nil, err
	}
	return workflow, nil
}

func (wm *TestableWorkflowManager) DeleteWorkflow(ctx context.Context, id string) error {
	query := `DELETE FROM workflows WHERE id = $1`
	result, err := wm.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (wm *TestableWorkflowManager) GetWorkflowStep(ctx context.Context, id string) (*testWorkflowStep, error) {
	query := `SELECT id, workflow_id, step_order, step_type, agent_id, wasm_module_id, config, created_at FROM workflow_steps WHERE id = $1`
	step := &testWorkflowStep{}
	err := wm.db.QueryRowContext(ctx, query, id).Scan(
		&step.ID, &step.WorkflowID, &step.StepOrder, &step.Type, &step.AgentID, &step.WasmModuleID, &step.Config, &step.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, err
	}
	return step, nil
}

func (wm *TestableWorkflowManager) UpdateWorkflowStep(ctx context.Context, id string, stepOrder int, stepType string, agentID, wasmModuleID *string, config map[string]interface{}) (*testWorkflowStep, error) {
	step, err := wm.GetWorkflowStep(ctx, id)
	if err != nil {
		return nil, err
	}

	configBytes, err := json.Marshal(config)
	if err != nil {
		return nil, err
	}

	step.StepOrder = stepOrder
	step.Type = stepType
	step.AgentID = agentID
	step.WasmModuleID = wasmModuleID
	step.Config = configBytes

	query := `UPDATE workflow_steps SET step_order = $1, step_type = $2, agent_id = $3, wasm_module_id = $4, config = $5 WHERE id = $6`
	_, err = wm.db.ExecContext(ctx, query, step.StepOrder, step.Type, step.AgentID, step.WasmModuleID, step.Config, step.ID)
	if err != nil {
		return nil, err
	}
	return step, nil
}

func (wm *TestableWorkflowManager) GetWorkflowStepConfig(ctx context.Context, id string) (map[string]interface{}, error) {
	step, err := wm.GetWorkflowStep(ctx, id)
	if err != nil {
		return nil, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal(step.Config, &config); err != nil {
		return nil, err
	}
	return config, nil
}

// =============================================================================
// Test data structures (simplified versions of dbmodels)
// =============================================================================

type testAgent struct {
	ID           string
	Name         string
	Description  string
	ProviderID   string
	ModelID      string
	SystemPrompt string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type testSkill struct {
	ID          string
	Name        string
	Description string
	Path        string
	Enabled     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type testWorkflow struct {
	ID          string
	Name        string
	Description string
	IsAsync     bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type testWorkflowStep struct {
	ID           string
	WorkflowID   string
	StepOrder    int
	Type         string
	AgentID      *string
	WasmModuleID *string
	Config       []byte
	CreatedAt    time.Time
}

// =============================================================================
// Test setup helper
// =============================================================================

func createMockDBWithWrapper(t *testing.T) (*mockDB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	return &mockDB{DB: db, mock: mock}, mock
}

// =============================================================================
// AgentManager Tests
// =============================================================================

func TestAgentManager_UpdateAgent(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableAgentManager(db)
	ctx := context.Background()

	now := time.Now()
	agentID := "agent-123"

	// Mock GetAgent (called first in UpdateAgent)
	mock.ExpectQuery("SELECT id, name, description, provider_id, model_id, system_prompt, created_at, updated_at FROM agents WHERE id").
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "provider_id", "model_id", "system_prompt", "created_at", "updated_at"}).
			AddRow(agentID, "Old Name", "Old Desc", "provider-1", "model-1", "Old prompt", now, now))

	// Mock Update query
	mock.ExpectExec("UPDATE agents SET name").
		WithArgs("New Name", "New Desc", "provider-2", "model-2", "New prompt", sqlmock.AnyArg(), agentID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Execute update
	agent, err := mgr.UpdateAgent(ctx, agentID, "New Name", "New Desc", "provider-2", "model-2", "New prompt")

	require.NoError(t, err)
	assert.NotNil(t, agent)
	assert.Equal(t, agentID, agent.ID)
	assert.Equal(t, "New Name", agent.Name)
	assert.Equal(t, "New Desc", agent.Description)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentManager_UpdateAgent_NotFound(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableAgentManager(db)
	ctx := context.Background()

	// Mock GetAgent returning no rows
	mock.ExpectQuery("SELECT id, name, description, provider_id, model_id, system_prompt, created_at, updated_at FROM agents WHERE id").
		WithArgs("nonexistent").
		WillReturnError(sql.ErrNoRows)

	// Execute update - should fail
	agent, err := mgr.UpdateAgent(ctx, "nonexistent", "Name", "Desc", "provider", "model", "prompt")

	assert.Error(t, err)
	assert.Nil(t, agent)
	assert.Equal(t, sql.ErrNoRows, err)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentManager_DeleteAgent(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableAgentManager(db)
	ctx := context.Background()

	agentID := "agent-123"

	// Mock delete query with 1 row affected
	mock.ExpectExec("DELETE FROM agents WHERE id").
		WithArgs(agentID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Execute delete
	err := mgr.DeleteAgent(ctx, agentID)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentManager_DeleteAgent_NotFound(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableAgentManager(db)
	ctx := context.Background()

	// Mock delete query with 0 rows affected
	mock.ExpectExec("DELETE FROM agents WHERE id").
		WithArgs("nonexistent").
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Execute delete - should fail
	err := mgr.DeleteAgent(ctx, "nonexistent")

	assert.Error(t, err)
	assert.Equal(t, sql.ErrNoRows, err)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentManager_RemoveToolFromAgent(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableAgentManager(db)
	ctx := context.Background()

	agentID := "agent-123"
	toolID := "tool-456"

	// Mock delete query
	mock.ExpectExec("DELETE FROM agent_tools WHERE agent_id").
		WithArgs(agentID, toolID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Execute
	err := mgr.RemoveToolFromAgent(ctx, agentID, toolID)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentManager_GetAgent(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableAgentManager(db)
	ctx := context.Background()

	agentID := "agent-123"
	now := time.Now()

	// Mock query
	mock.ExpectQuery("SELECT id, name, description, provider_id, model_id, system_prompt, created_at, updated_at FROM agents WHERE id").
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "provider_id", "model_id", "system_prompt", "created_at", "updated_at"}).
			AddRow(agentID, "Test Agent", "Test Description", "provider-1", "model-1", "You are helpful", now, now))

	// Execute
	agent, err := mgr.GetAgent(ctx, agentID)

	require.NoError(t, err)
	assert.NotNil(t, agent)
	assert.Equal(t, agentID, agent.ID)
	assert.Equal(t, "Test Agent", agent.Name)
	assert.Equal(t, "Test Description", agent.Description)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestAgentManager_GetAgent_NotFound(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableAgentManager(db)
	ctx := context.Background()

	// Mock query returning no rows
	mock.ExpectQuery("SELECT id, name, description, provider_id, model_id, system_prompt, created_at, updated_at FROM agents WHERE id").
		WithArgs("nonexistent").
		WillReturnError(sql.ErrNoRows)

	// Execute - should fail
	agent, err := mgr.GetAgent(ctx, "nonexistent")

	assert.Error(t, err)
	assert.Nil(t, agent)
	assert.Equal(t, sql.ErrNoRows, err)

	assert.NoError(t, mock.ExpectationsWereMet())
}

// =============================================================================
// SkillManager Tests
// =============================================================================

func TestSkillManager_GetSkill(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableSkillManager(db)
	ctx := context.Background()

	skillID := "skill-123"
	now := time.Now()

	// Mock query
	mock.ExpectQuery("SELECT id, name, description, path, enabled, created_at, updated_at FROM skills WHERE id").
		WithArgs(skillID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "path", "enabled", "created_at", "updated_at"}).
			AddRow(skillID, "Test Skill", "Test Description", "/path/to/skill", true, now, now))

	// Execute
	skill, err := mgr.GetSkill(ctx, skillID)

	require.NoError(t, err)
	assert.NotNil(t, skill)
	assert.Equal(t, skillID, skill.ID)
	assert.Equal(t, "Test Skill", skill.Name)
	assert.Equal(t, "/path/to/skill", skill.Path)
	assert.True(t, skill.Enabled)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSkillManager_GetSkill_NotFound(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableSkillManager(db)
	ctx := context.Background()

	// Mock query returning no rows
	mock.ExpectQuery("SELECT id, name, description, path, enabled, created_at, updated_at FROM skills WHERE id").
		WithArgs("nonexistent").
		WillReturnError(sql.ErrNoRows)

	// Execute - should fail
	skill, err := mgr.GetSkill(ctx, "nonexistent")

	assert.Error(t, err)
	assert.Nil(t, skill)
	assert.Equal(t, sql.ErrNoRows, err)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSkillManager_UpdateSkill(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableSkillManager(db)
	ctx := context.Background()

	skillID := "skill-123"
	now := time.Now()

	// Mock GetSkill (called first)
	mock.ExpectQuery("SELECT id, name, description, path, enabled, created_at, updated_at FROM skills WHERE id").
		WithArgs(skillID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "path", "enabled", "created_at", "updated_at"}).
			AddRow(skillID, "Old Name", "Old Desc", "/old/path", false, now, now))

	// Mock Update query
	mock.ExpectExec("UPDATE skills SET name").
		WithArgs("New Name", "New Desc", "/new/path", true, sqlmock.AnyArg(), skillID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Execute update
	skill, err := mgr.UpdateSkill(ctx, skillID, "New Name", "New Desc", "/new/path", true)

	require.NoError(t, err)
	assert.NotNil(t, skill)
	assert.Equal(t, skillID, skill.ID)
	assert.Equal(t, "New Name", skill.Name)
	assert.Equal(t, "New Desc", skill.Description)
	assert.Equal(t, "/new/path", skill.Path)
	assert.True(t, skill.Enabled)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSkillManager_UpdateSkill_NotFound(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableSkillManager(db)
	ctx := context.Background()

	// Mock GetSkill returning no rows
	mock.ExpectQuery("SELECT id, name, description, path, enabled, created_at, updated_at FROM skills WHERE id").
		WithArgs("nonexistent").
		WillReturnError(sql.ErrNoRows)

	// Execute update - should fail
	skill, err := mgr.UpdateSkill(ctx, "nonexistent", "Name", "Desc", "/path", true)

	assert.Error(t, err)
	assert.Nil(t, skill)
	assert.Equal(t, sql.ErrNoRows, err)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSkillManager_DeleteSkill(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableSkillManager(db)
	ctx := context.Background()

	skillID := "skill-123"

	// Mock delete query with 1 row affected
	mock.ExpectExec("DELETE FROM skills WHERE id").
		WithArgs(skillID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Execute delete
	err := mgr.DeleteSkill(ctx, skillID)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSkillManager_DeleteSkill_NotFound(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableSkillManager(db)
	ctx := context.Background()

	// Mock delete query with 0 rows affected
	mock.ExpectExec("DELETE FROM skills WHERE id").
		WithArgs("nonexistent").
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Execute delete - should fail
	err := mgr.DeleteSkill(ctx, "nonexistent")

	assert.Error(t, err)
	assert.Equal(t, sql.ErrNoRows, err)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSkillManager_RemoveSkillFromAgent(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableSkillManager(db)
	ctx := context.Background()

	agentID := "agent-123"
	skillID := "skill-456"

	// Mock delete query
	mock.ExpectExec("DELETE FROM agent_skills WHERE agent_id").
		WithArgs(agentID, skillID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Execute
	err := mgr.RemoveSkillFromAgent(ctx, agentID, skillID)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

// =============================================================================
// WorkflowManager Tests
// =============================================================================

func TestWorkflowManager_GetWorkflow(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableWorkflowManager(db)
	ctx := context.Background()

	workflowID := "wf-123"
	now := time.Now()

	// Mock query
	mock.ExpectQuery("SELECT id, name, description, is_async, created_at, updated_at FROM workflows WHERE id").
		WithArgs(workflowID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "is_async", "created_at", "updated_at"}).
			AddRow(workflowID, "Test Workflow", "Test Description", true, now, now))

	// Execute
	workflow, err := mgr.GetWorkflow(ctx, workflowID)

	require.NoError(t, err)
	assert.NotNil(t, workflow)
	assert.Equal(t, workflowID, workflow.ID)
	assert.Equal(t, "Test Workflow", workflow.Name)
	assert.True(t, workflow.IsAsync)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkflowManager_GetWorkflow_NotFound(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableWorkflowManager(db)
	ctx := context.Background()

	// Mock query returning no rows
	mock.ExpectQuery("SELECT id, name, description, is_async, created_at, updated_at FROM workflows WHERE id").
		WithArgs("nonexistent").
		WillReturnError(sql.ErrNoRows)

	// Execute - should fail
	workflow, err := mgr.GetWorkflow(ctx, "nonexistent")

	assert.Error(t, err)
	assert.Nil(t, workflow)
	assert.Equal(t, sql.ErrNoRows, err)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkflowManager_UpdateWorkflow(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableWorkflowManager(db)
	ctx := context.Background()

	workflowID := "wf-123"
	now := time.Now()

	// Mock GetWorkflow (called first)
	mock.ExpectQuery("SELECT id, name, description, is_async, created_at, updated_at FROM workflows WHERE id").
		WithArgs(workflowID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "description", "is_async", "created_at", "updated_at"}).
			AddRow(workflowID, "Old Name", "Old Desc", false, now, now))

	// Mock Update query
	mock.ExpectExec("UPDATE workflows SET name").
		WithArgs("New Name", "New Desc", true, sqlmock.AnyArg(), workflowID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Execute update
	workflow, err := mgr.UpdateWorkflow(ctx, workflowID, "New Name", "New Desc", true)

	require.NoError(t, err)
	assert.NotNil(t, workflow)
	assert.Equal(t, workflowID, workflow.ID)
	assert.Equal(t, "New Name", workflow.Name)
	assert.Equal(t, "New Desc", workflow.Description)
	assert.True(t, workflow.IsAsync)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkflowManager_UpdateWorkflow_NotFound(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableWorkflowManager(db)
	ctx := context.Background()

	// Mock GetWorkflow returning no rows
	mock.ExpectQuery("SELECT id, name, description, is_async, created_at, updated_at FROM workflows WHERE id").
		WithArgs("nonexistent").
		WillReturnError(sql.ErrNoRows)

	// Execute update - should fail
	workflow, err := mgr.UpdateWorkflow(ctx, "nonexistent", "Name", "Desc", true)

	assert.Error(t, err)
	assert.Nil(t, workflow)
	assert.Equal(t, sql.ErrNoRows, err)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkflowManager_DeleteWorkflow(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableWorkflowManager(db)
	ctx := context.Background()

	workflowID := "wf-123"

	// Mock delete query with 1 row affected
	mock.ExpectExec("DELETE FROM workflows WHERE id").
		WithArgs(workflowID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// Execute delete
	err := mgr.DeleteWorkflow(ctx, workflowID)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkflowManager_DeleteWorkflow_NotFound(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableWorkflowManager(db)
	ctx := context.Background()

	// Mock delete query with 0 rows affected
	mock.ExpectExec("DELETE FROM workflows WHERE id").
		WithArgs("nonexistent").
		WillReturnResult(sqlmock.NewResult(0, 0))

	// Execute delete - should fail
	err := mgr.DeleteWorkflow(ctx, "nonexistent")

	assert.Error(t, err)
	assert.Equal(t, sql.ErrNoRows, err)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkflowManager_GetWorkflowStep(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableWorkflowManager(db)
	ctx := context.Background()

	stepID := "step-123"
	now := time.Now()
	config := []byte(`{"param": "value"}`)

	// Mock query
	mock.ExpectQuery("SELECT id, workflow_id, step_order, step_type, agent_id, wasm_module_id, config, created_at FROM workflow_steps WHERE id").
		WithArgs(stepID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "workflow_id", "step_order", "step_type", "agent_id", "wasm_module_id", "config", "created_at"}).
			AddRow(stepID, "wf-123", 1, "AGENT", "agent-1", nil, config, now))

	// Execute
	step, err := mgr.GetWorkflowStep(ctx, stepID)

	require.NoError(t, err)
	assert.NotNil(t, step)
	assert.Equal(t, stepID, step.ID)
	assert.Equal(t, "wf-123", step.WorkflowID)
	assert.Equal(t, 1, step.StepOrder)
	assert.Equal(t, "AGENT", step.Type)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkflowManager_GetWorkflowStep_NotFound(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableWorkflowManager(db)
	ctx := context.Background()

	// Mock query returning no rows
	mock.ExpectQuery("SELECT id, workflow_id, step_order, step_type, agent_id, wasm_module_id, config, created_at FROM workflow_steps WHERE id").
		WithArgs("nonexistent").
		WillReturnError(sql.ErrNoRows)

	// Execute - should fail
	step, err := mgr.GetWorkflowStep(ctx, "nonexistent")

	assert.Error(t, err)
	assert.Nil(t, step)
	assert.Equal(t, sql.ErrNoRows, err)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkflowManager_UpdateWorkflowStep(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableWorkflowManager(db)
	ctx := context.Background()

	stepID := "step-123"
	now := time.Now()
	oldConfig := []byte(`{"old": "config"}`)
	newConfig := map[string]interface{}{"new": "config"}

	// Mock GetWorkflowStep (called first)
	mock.ExpectQuery("SELECT id, workflow_id, step_order, step_type, agent_id, wasm_module_id, config, created_at FROM workflow_steps WHERE id").
		WithArgs(stepID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "workflow_id", "step_order", "step_type", "agent_id", "wasm_module_id", "config", "created_at"}).
			AddRow(stepID, "wf-123", 1, "AGENT", "agent-1", nil, oldConfig, now))

	// Mock Update query
	mock.ExpectExec("UPDATE workflow_steps SET step_order").
		WithArgs(2, "WASM", nil, "wasm-1", sqlmock.AnyArg(), stepID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	wasmID := "wasm-1"
	// Execute update
	step, err := mgr.UpdateWorkflowStep(ctx, stepID, 2, "WASM", nil, &wasmID, newConfig)

	require.NoError(t, err)
	assert.NotNil(t, step)
	assert.Equal(t, stepID, step.ID)
	assert.Equal(t, 2, step.StepOrder)
	assert.Equal(t, "WASM", step.Type)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkflowManager_UpdateWorkflowStep_NotFound(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableWorkflowManager(db)
	ctx := context.Background()

	// Mock GetWorkflowStep returning no rows
	mock.ExpectQuery("SELECT id, workflow_id, step_order, step_type, agent_id, wasm_module_id, config, created_at FROM workflow_steps WHERE id").
		WithArgs("nonexistent").
		WillReturnError(sql.ErrNoRows)

	// Execute update - should fail
	step, err := mgr.UpdateWorkflowStep(ctx, "nonexistent", 1, "AGENT", nil, nil, nil)

	assert.Error(t, err)
	assert.Nil(t, step)
	assert.Equal(t, sql.ErrNoRows, err)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestWorkflowManager_GetWorkflowStepConfig(t *testing.T) {
	db, mock := createMockDBWithWrapper(t)
	defer func() { _ = db.Close() }()

	mgr := NewTestableWorkflowManager(db)
	ctx := context.Background()

	stepID := "step-123"
	now := time.Now()
	configBytes, _ := json.Marshal(map[string]interface{}{"param1": "value1", "param2": 42})

	// Mock GetWorkflowStep (called first)
	mock.ExpectQuery("SELECT id, workflow_id, step_order, step_type, agent_id, wasm_module_id, config, created_at FROM workflow_steps WHERE id").
		WithArgs(stepID).
		WillReturnRows(sqlmock.NewRows([]string{"id", "workflow_id", "step_order", "step_type", "agent_id", "wasm_module_id", "config", "created_at"}).
			AddRow(stepID, "wf-123", 1, "AGENT", "agent-1", nil, configBytes, now))

	// Execute
	config, err := mgr.GetWorkflowStepConfig(ctx, stepID)

	require.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, "value1", config["param1"])
	assert.Equal(t, float64(42), config["param2"]) // JSON unmarshal converts numbers to float64

	assert.NoError(t, mock.ExpectationsWereMet())
}
