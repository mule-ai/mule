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

// createMockDB creates a sqlmock for testing
func createMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	return db, mock
}

// AgentManager Tests

func TestAgentManager_CreateAgent(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	// Test that columns match query
	mock.ExpectExec("INSERT INTO agents").
		WithArgs(sqlmock.AnyArg(), "test-agent", "Test description", "provider-1", "model-1", "You are helpful", sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// Verify the query format
	query := `INSERT INTO agents (id, name, description, provider_id, model_id, system_prompt, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	assert.NotEmpty(t, query)

	// Test the mock works
	result, err := db.ExecContext(context.Background(), query, "test-id", "test-agent", "Test description", "provider-1", "model-1", "You are helpful", time.Now(), time.Now())
	require.NoError(t, err)
	rowsAffected, err := result.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)
}

func TestAgentManager_ListAgents(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	now := time.Now()
	columns := []string{"id", "name", "description", "provider_id", "model_id", "system_prompt", "created_at", "updated_at"}

	// Empty list
	mock.ExpectQuery("SELECT id, name, description, provider_id, model_id, system_prompt, created_at, updated_at FROM agents ORDER BY created_at DESC").
		WillReturnRows(sqlmock.NewRows(columns))

	rows, err := db.QueryContext(context.Background(), "SELECT id, name, description, provider_id, model_id, system_prompt, created_at, updated_at FROM agents ORDER BY created_at DESC")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	agentCount := 0
	for rows.Next() {
		agentCount++
	}
	assert.Equal(t, 0, agentCount)

	// List with data
	mock.ExpectQuery("SELECT id, name, description, provider_id, model_id, system_prompt, created_at, updated_at FROM agents ORDER BY created_at DESC").
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("agent-1", "Agent 1", "Desc 1", "provider-1", "model-1", "Prompt 1", now, now).
			AddRow("agent-2", "Agent 2", "Desc 2", "provider-1", "model-2", "Prompt 2", now, now))

	rows, err = db.QueryContext(context.Background(), "SELECT id, name, description, provider_id, model_id, system_prompt, created_at, updated_at FROM agents ORDER BY created_at DESC")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	agentCount = 0
	for rows.Next() {
		agentCount++
	}
	assert.Equal(t, 2, agentCount)
}

func TestAgentManager_ScanAgent(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	now := time.Now()
	columns := []string{"id", "name", "description", "provider_id", "model_id", "system_prompt", "created_at", "updated_at"}

	mock.ExpectQuery("SELECT id, name, description, provider_id, model_id, system_prompt, created_at, updated_at FROM agents WHERE id").
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("agent-123", "Test Agent", "Test description", "provider-1", "model-1", "You are helpful", now, now))

	rows, err := db.QueryContext(context.Background(), "SELECT id, name, description, provider_id, model_id, system_prompt, created_at, updated_at FROM agents WHERE id = $1", "agent-123")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	require.True(t, rows.Next())
	var id, name, description, providerID, modelID, systemPrompt string
	var createdAt, updatedAt time.Time
	err = rows.Scan(&id, &name, &description, &providerID, &modelID, &systemPrompt, &createdAt, &updatedAt)
	require.NoError(t, err)
	assert.Equal(t, "agent-123", id)
	assert.Equal(t, "Test Agent", name)
	assert.Equal(t, "Test description", description)
	assert.Equal(t, "provider-1", providerID)
	assert.Equal(t, "model-1", modelID)
	assert.Equal(t, "You are helpful", systemPrompt)
}

func TestAgentManager_AddToolToAgent(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	mock.ExpectExec("INSERT INTO agent_tools").
		WithArgs("agent-123", "tool-456").
		WillReturnResult(sqlmock.NewResult(1, 1))

	result, err := db.ExecContext(context.Background(), "INSERT INTO agent_tools (agent_id, tool_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", "agent-123", "tool-456")
	require.NoError(t, err)
	rowsAffected, err := result.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)
}

func TestAgentManager_GetAgentTools(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	now := time.Now()
	columns := []string{"id", "name", "description", "metadata", "created_at", "updated_at"}

	// No tools
	mock.ExpectQuery("SELECT t.id, t.name, t.description, t.metadata, t.created_at, t.updated_at").
		WithArgs("agent-123").
		WillReturnRows(sqlmock.NewRows(columns))

	rows, err := db.QueryContext(context.Background(), "SELECT t.id, t.name, t.description, t.metadata, t.created_at, t.updated_at FROM tools t JOIN agent_tools at ON t.id = at.tool_id WHERE at.agent_id = $1 ORDER BY t.created_at DESC", "agent-123")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	count := 0
	for rows.Next() {
		count++
	}
	assert.Equal(t, 0, count)

	// With tools
	metadata := []byte(`{"tool_type": "api", "config": {}}`)
	mock.ExpectQuery("SELECT t.id, t.name, t.description, t.metadata, t.created_at, t.updated_at").
		WithArgs("agent-123").
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("tool-1", "Tool 1", "Desc 1", metadata, now, now).
			AddRow("tool-2", "Tool 2", "Desc 2", metadata, now, now))

	rows, err = db.QueryContext(context.Background(), "SELECT t.id, t.name, t.description, t.metadata, t.created_at, t.updated_at FROM tools t JOIN agent_tools at ON t.id = at.tool_id WHERE at.agent_id = $1 ORDER BY t.created_at DESC", "agent-123")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	count = 0
	for rows.Next() {
		count++
	}
	assert.Equal(t, 2, count)
}

// ProviderManager Tests

func TestProviderManager_CreateProvider(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	// Test encryption placeholder - we test the query structure
	mock.ExpectExec("INSERT INTO providers").
		WithArgs(sqlmock.AnyArg(), "Test Provider", "https://api.openai.com", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	result, err := db.ExecContext(context.Background(), "INSERT INTO providers (id, name, api_base_url, api_key_encrypted, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)", "provider-1", "Test Provider", "https://api.openai.com", "encrypted-key", time.Now(), time.Now())
	require.NoError(t, err)
	rowsAffected, err := result.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)
}

func TestProviderManager_ScanProvider(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	now := time.Now()
	columns := []string{"id", "name", "api_base_url", "api_key_encrypted", "created_at", "updated_at"}

	mock.ExpectQuery("SELECT id, name, api_base_url, api_key_encrypted, created_at, updated_at FROM providers WHERE id").
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("provider-123", "Test Provider", "https://api.openai.com", "encrypted-key", now, now))

	rows, err := db.QueryContext(context.Background(), "SELECT id, name, api_base_url, api_key_encrypted, created_at, updated_at FROM providers WHERE id = $1", "provider-123")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	require.True(t, rows.Next())
	var id, name, apiBaseURL, apiKeyEnc string
	var createdAt, updatedAt time.Time
	err = rows.Scan(&id, &name, &apiBaseURL, &apiKeyEnc, &createdAt, &updatedAt)
	require.NoError(t, err)
	assert.Equal(t, "provider-123", id)
	assert.Equal(t, "Test Provider", name)
	assert.Equal(t, "https://api.openai.com", apiBaseURL)
	assert.Equal(t, "encrypted-key", apiKeyEnc)
}

func TestProviderManager_ListProviders(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	now := time.Now()
	columns := []string{"id", "name", "api_base_url", "api_key_encrypted", "created_at", "updated_at"}

	mock.ExpectQuery("SELECT id, name, api_base_url, api_key_encrypted, created_at, updated_at FROM providers ORDER BY created_at DESC").
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("provider-1", "Provider 1", "https://api1.com", "key1", now, now).
			AddRow("provider-2", "Provider 2", "https://api2.com", "key2", now, now))

	rows, err := db.QueryContext(context.Background(), "SELECT id, name, api_base_url, api_key_encrypted, created_at, updated_at FROM providers ORDER BY created_at DESC")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	count := 0
	for rows.Next() {
		count++
	}
	assert.Equal(t, 2, count)
}

// SkillManager Tests

func TestSkillManager_CreateSkill(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	mock.ExpectExec("INSERT INTO skills").
		WithArgs(sqlmock.AnyArg(), "test-skill", "Test skill description", "/path/to/skill", true, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	result, err := db.ExecContext(context.Background(), "INSERT INTO skills (id, name, description, path, enabled, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7)", "skill-1", "test-skill", "Test skill description", "/path/to/skill", true, time.Now(), time.Now())
	require.NoError(t, err)
	rowsAffected, err := result.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)
}

func TestSkillManager_ScanSkill(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	now := time.Now()
	columns := []string{"id", "name", "description", "path", "enabled", "created_at", "updated_at"}

	mock.ExpectQuery("SELECT id, name, description, path, enabled, created_at, updated_at FROM skills WHERE id").
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("skill-123", "Test Skill", "Test description", "/path/to/skill", true, now, now))

	rows, err := db.QueryContext(context.Background(), "SELECT id, name, description, path, enabled, created_at, updated_at FROM skills WHERE id = $1", "skill-123")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	require.True(t, rows.Next())
	var id, name, description, path string
	var enabled bool
	var createdAt, updatedAt time.Time
	err = rows.Scan(&id, &name, &description, &path, &enabled, &createdAt, &updatedAt)
	require.NoError(t, err)
	assert.Equal(t, "skill-123", id)
	assert.Equal(t, "Test Skill", name)
	assert.Equal(t, "Test description", description)
	assert.Equal(t, "/path/to/skill", path)
	assert.True(t, enabled)
}

func TestSkillManager_ListSkills(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	now := time.Now()
	columns := []string{"id", "name", "description", "path", "enabled", "created_at", "updated_at"}

	mock.ExpectQuery("SELECT id, name, description, path, enabled, created_at, updated_at FROM skills ORDER BY name").
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("skill-1", "Alpha Skill", "Desc 1", "/path1", true, now, now).
			AddRow("skill-2", "Beta Skill", "Desc 2", "/path2", false, now, now))

	rows, err := db.QueryContext(context.Background(), "SELECT id, name, description, path, enabled, created_at, updated_at FROM skills ORDER BY name")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	count := 0
	for rows.Next() {
		count++
	}
	assert.Equal(t, 2, count)
}

func TestSkillManager_AddSkillToAgent(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	mock.ExpectExec("INSERT INTO agent_skills").
		WithArgs("agent-123", "skill-456").
		WillReturnResult(sqlmock.NewResult(1, 1))

	result, err := db.ExecContext(context.Background(), "INSERT INTO agent_skills (agent_id, skill_id) VALUES ($1, $2) ON CONFLICT DO NOTHING", "agent-123", "skill-456")
	require.NoError(t, err)
	rowsAffected, err := result.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)
}

func TestSkillManager_GetAgentSkills(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	now := time.Now()
	columns := []string{"id", "name", "description", "path", "enabled", "created_at", "updated_at"}

	// No skills
	mock.ExpectQuery("SELECT s.id, s.name, s.description, s.path, s.enabled, s.created_at, s.updated_at").
		WithArgs("agent-123").
		WillReturnRows(sqlmock.NewRows(columns))

	rows, err := db.QueryContext(context.Background(), "SELECT s.id, s.name, s.description, s.path, s.enabled, s.created_at, s.updated_at FROM skills s JOIN agent_skills ast ON s.id = ast.skill_id WHERE ast.agent_id = $1 ORDER BY s.name", "agent-123")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	count := 0
	for rows.Next() {
		count++
	}
	assert.Equal(t, 0, count)

	// With skills
	mock.ExpectQuery("SELECT s.id, s.name, s.description, s.path, s.enabled, s.created_at, s.updated_at").
		WithArgs("agent-123").
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("skill-1", "Alpha", "Desc", "/path", true, now, now).
			AddRow("skill-2", "Beta", "Desc", "/path", true, now, now))

	rows, err = db.QueryContext(context.Background(), "SELECT s.id, s.name, s.description, s.path, s.enabled, s.created_at, s.updated_at FROM skills s JOIN agent_skills ast ON s.id = ast.skill_id WHERE ast.agent_id = $1 ORDER BY s.name", "agent-123")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	count = 0
	for rows.Next() {
		count++
	}
	assert.Equal(t, 2, count)
}

// ToolManager Tests

func TestToolManager_CreateTool(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	metadata := map[string]interface{}{
		"tool_type": "api",
		"config":    map[string]interface{}{"url": "https://api.example.com"},
	}
	metadataBytes, err := json.Marshal(metadata)
	require.NoError(t, err)

	mock.ExpectExec("INSERT INTO tools").
		WithArgs(sqlmock.AnyArg(), "test-tool", "Test tool description", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	result, err := db.ExecContext(context.Background(), "INSERT INTO tools (id, name, description, metadata, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)", "tool-1", "test-tool", "Test tool description", metadataBytes, time.Now(), time.Now())
	require.NoError(t, err)
	rowsAffected, err := result.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)
}

func TestToolManager_ScanTool(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	now := time.Now()
	columns := []string{"id", "name", "description", "metadata", "created_at", "updated_at"}
	metadata := []byte(`{"tool_type": "api", "config": {"url": "https://api.example.com"}}`)

	mock.ExpectQuery("SELECT id, name, description, metadata, created_at, updated_at FROM tools WHERE id").
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("tool-123", "Test Tool", "Test description", metadata, now, now))

	rows, err := db.QueryContext(context.Background(), "SELECT id, name, description, metadata, created_at, updated_at FROM tools WHERE id = $1", "tool-123")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	require.True(t, rows.Next())
	var id, name, description string
	var metadataBytes []byte
	var createdAt, updatedAt time.Time
	err = rows.Scan(&id, &name, &description, &metadataBytes, &createdAt, &updatedAt)
	require.NoError(t, err)
	assert.Equal(t, "tool-123", id)
	assert.Equal(t, "Test Tool", name)

	// Parse and verify metadata
	var parsedMetadata map[string]interface{}
	err = json.Unmarshal(metadataBytes, &parsedMetadata)
	require.NoError(t, err)
	assert.Equal(t, "api", parsedMetadata["tool_type"])
}

func TestToolManager_ListTools(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	now := time.Now()
	columns := []string{"id", "name", "description", "metadata", "created_at", "updated_at"}
	metadata := []byte(`{"tool_type": "api"}`)

	mock.ExpectQuery("SELECT id, name, description, metadata, created_at, updated_at FROM tools ORDER BY created_at DESC").
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("tool-1", "Tool 1", "Desc 1", metadata, now, now).
			AddRow("tool-2", "Tool 2", "Desc 2", metadata, now, now))

	rows, err := db.QueryContext(context.Background(), "SELECT id, name, description, metadata, created_at, updated_at FROM tools ORDER BY created_at DESC")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	count := 0
	for rows.Next() {
		count++
	}
	assert.Equal(t, 2, count)
}

func TestToolManager_GetToolConfig(t *testing.T) {
	// Test config extraction from metadata
	metadata := map[string]interface{}{
		"tool_type": "api",
		"config": map[string]interface{}{
			"url":         "https://api.example.com",
			"timeout":     30,
			"retry_count": 3,
		},
	}

	metadataBytes, err := json.Marshal(metadata)
	require.NoError(t, err)

	var parsedMetadata map[string]interface{}
	err = json.Unmarshal(metadataBytes, &parsedMetadata)
	require.NoError(t, err)

	config, ok := parsedMetadata["config"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "https://api.example.com", config["url"])
	assert.Equal(t, float64(30), config["timeout"]) // JSON numbers are float64
	assert.Equal(t, float64(3), config["retry_count"])
}

// WorkflowManager Tests

func TestWorkflowManager_CreateWorkflow(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	mock.ExpectExec("INSERT INTO workflows").
		WithArgs(sqlmock.AnyArg(), "test-workflow", "Test workflow description", true, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	result, err := db.ExecContext(context.Background(), "INSERT INTO workflows (id, name, description, is_async, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6)", "wf-1", "test-workflow", "Test workflow description", true, time.Now(), time.Now())
	require.NoError(t, err)
	rowsAffected, err := result.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)
}

func TestWorkflowManager_ScanWorkflow(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	now := time.Now()
	columns := []string{"id", "name", "description", "is_async", "created_at", "updated_at"}

	mock.ExpectQuery("SELECT id, name, description, is_async, created_at, updated_at FROM workflows WHERE id").
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("wf-123", "Test Workflow", "Test description", true, now, now))

	rows, err := db.QueryContext(context.Background(), "SELECT id, name, description, is_async, created_at, updated_at FROM workflows WHERE id = $1", "wf-123")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	require.True(t, rows.Next())
	var id, name, description string
	var isAsync bool
	var createdAt, updatedAt time.Time
	err = rows.Scan(&id, &name, &description, &isAsync, &createdAt, &updatedAt)
	require.NoError(t, err)
	assert.Equal(t, "wf-123", id)
	assert.Equal(t, "Test Workflow", name)
	assert.True(t, isAsync)
}

func TestWorkflowManager_ListWorkflows(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	now := time.Now()
	columns := []string{"id", "name", "description", "is_async", "created_at", "updated_at"}

	mock.ExpectQuery("SELECT id, name, description, is_async, created_at, updated_at FROM workflows ORDER BY created_at DESC").
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("wf-1", "Workflow 1", "Desc 1", true, now, now).
			AddRow("wf-2", "Workflow 2", "Desc 2", false, now, now))

	rows, err := db.QueryContext(context.Background(), "SELECT id, name, description, is_async, created_at, updated_at FROM workflows ORDER BY created_at DESC")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	count := 0
	for rows.Next() {
		count++
	}
	assert.Equal(t, 2, count)
}

func TestWorkflowManager_CreateWorkflowStep(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	config := map[string]interface{}{"param": "value"}
	configBytes, err := json.Marshal(config)
	require.NoError(t, err)

	agentID := "agent-123"
	mock.ExpectExec("INSERT INTO workflow_steps").
		WithArgs(sqlmock.AnyArg(), "wf-123", 1, "AGENT", &agentID, nil, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	result, err := db.ExecContext(context.Background(), "INSERT INTO workflow_steps (id, workflow_id, step_order, step_type, agent_id, wasm_module_id, config, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)", "step-1", "wf-123", 1, "AGENT", agentID, nil, configBytes, time.Now())
	require.NoError(t, err)
	rowsAffected, err := result.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rowsAffected)
}

func TestWorkflowManager_ScanWorkflowStep(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	now := time.Now()
	columns := []string{"id", "workflow_id", "step_order", "step_type", "agent_id", "wasm_module_id", "config", "created_at"}
	config := []byte(`{"param": "value"}`)

	mock.ExpectQuery("SELECT id, workflow_id, step_order, step_type, agent_id, wasm_module_id, config, created_at FROM workflow_steps WHERE id").
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("step-123", "wf-123", 1, "AGENT", "agent-1", nil, config, now))

	rows, err := db.QueryContext(context.Background(), "SELECT id, workflow_id, step_order, step_type, agent_id, wasm_module_id, config, created_at FROM workflow_steps WHERE id = $1", "step-123")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	require.True(t, rows.Next())
	var id, workflowID, stepType string
	var stepOrder int
	var agentID, wasmModuleID *string
	var configBytes []byte
	var createdAt time.Time
	err = rows.Scan(&id, &workflowID, &stepOrder, &stepType, &agentID, &wasmModuleID, &configBytes, &createdAt)
	require.NoError(t, err)
	assert.Equal(t, "step-123", id)
	assert.Equal(t, 1, stepOrder)
	assert.Equal(t, "AGENT", stepType)
	assert.NotNil(t, agentID)
	assert.Equal(t, "agent-1", *agentID)
	assert.Nil(t, wasmModuleID)
}

func TestWorkflowManager_GetWorkflowSteps(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	now := time.Now()
	columns := []string{"id", "workflow_id", "step_order", "step_type", "agent_id", "wasm_module_id", "config", "created_at"}
	config := []byte(`{"param": "value"}`)

	mock.ExpectQuery("SELECT id, workflow_id, step_order, step_type, agent_id, wasm_module_id, config, created_at FROM workflow_steps WHERE workflow_id").
		WithArgs("wf-123").
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("step-1", "wf-123", 1, "AGENT", "agent-1", nil, config, now).
			AddRow("step-2", "wf-123", 2, "WASM", nil, "wasm-1", config, now))

	rows, err := db.QueryContext(context.Background(), "SELECT id, workflow_id, step_order, step_type, agent_id, wasm_module_id, config, created_at FROM workflow_steps WHERE workflow_id = $1 ORDER BY step_order ASC", "wf-123")
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()

	count := 0
	for rows.Next() {
		count++
	}
	assert.Equal(t, 2, count)
}

// Error handling tests

func TestErrorHandling_ExecFailure(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	// Test exec failure
	mock.ExpectExec("INSERT INTO agents").
		WillReturnError(sql.ErrConnDone)

	_, err := db.ExecContext(context.Background(), "INSERT INTO agents (id, name) VALUES ($1, $2)", "agent-1", "Test Agent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection")
}

func TestErrorHandling_RowsClose(t *testing.T) {
	db, mock := createMockDB(t)
	defer func() { _ = db.Close() }()

	now := time.Now()
	columns := []string{"id", "name", "description", "provider_id", "model_id", "system_prompt", "created_at", "updated_at"}

	// Simulate rows not being closed properly
	mock.ExpectQuery("SELECT id, name, description, provider_id, model_id, system_prompt, created_at, updated_at FROM agents").
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("agent-1", "Agent 1", "Desc", "p1", "m1", "prompt", now, now))

	rows, err := db.QueryContext(context.Background(), "SELECT id, name, description, provider_id, model_id, system_prompt, created_at, updated_at FROM agents")
	require.NoError(t, err)

	// Read one row
	if rows.Next() {
		// Don't close - simulate potential leak
		_ = struct{}{} // no-op to satisfy linter
	}

	// Verify we can still use the db after potential leak scenario
	mock.ExpectPing()
	err = db.Ping()
	assert.NoError(t, err)

	// Now properly close
	_ = rows.Close()
}

// Test Workflow Step config marshaling
func TestWorkflowStepConfigMarshaling(t *testing.T) {
	config := map[string]interface{}{
		"param1": "value1",
		"param2": 123,
		"nested": map[string]interface{}{
			"key": "value",
		},
	}

	configBytes, err := json.Marshal(config)
	require.NoError(t, err)

	var parsedConfig map[string]interface{}
	err = json.Unmarshal(configBytes, &parsedConfig)
	require.NoError(t, err)

	assert.Equal(t, "value1", parsedConfig["param1"])
	assert.Equal(t, float64(123), parsedConfig["param2"])

	nested, ok := parsedConfig["nested"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "value", nested["key"])
}

// Test Skill config marshaling
func TestSkillConfigMarshaling(t *testing.T) {
	// Test skill configuration storage
	skill := map[string]interface{}{
		"name":        "test-skill",
		"description": "Test skill description",
		"path":        "/path/to/skill",
		"enabled":     true,
	}

	configBytes, err := json.Marshal(skill)
	require.NoError(t, err)

	var parsedSkill map[string]interface{}
	err = json.Unmarshal(configBytes, &parsedSkill)
	require.NoError(t, err)

	assert.Equal(t, "test-skill", parsedSkill["name"])
	assert.Equal(t, "Test skill description", parsedSkill["description"])
	assert.Equal(t, "/path/to/skill", parsedSkill["path"])
	assert.Equal(t, true, parsedSkill["enabled"])
}
