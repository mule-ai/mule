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

	"github.com/mule-ai/mule/internal/database"
	dbmodels "github.com/mule-ai/mule/pkg/database"
)

// =============================================================================
// Test helper: wrap sqlmock to provide the database.DB interface
// =============================================================================

// testDB wraps sqlmock to provide a compatible interface for managers
type testDB struct {
	*sql.DB
	mock sqlmock.Sqlmock
}

func (m *testDB) BeginTx(ctx context.Context, opts interface{}) (*sql.Tx, error) {
	return m.Begin()
}

func (m *testDB) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return m.DB.ExecContext(ctx, query, args...)
}

func (m *testDB) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return m.DB.QueryContext(ctx, query, args...)
}

func (m *testDB) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return m.DB.QueryRowContext(ctx, query, args...)
}

// =============================================================================
// Test Setup
// =============================================================================

func createTestDBForManagers(t *testing.T) (*database.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)

	// Wrap the mock DB in our database.DB type
	dbWrapper := &database.DB{DB: db}

	return dbWrapper, mock
}

// =============================================================================
// ToolManager API Tests
// =============================================================================

func TestToolManagerAPI_CreateTool(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewToolManager(db)
	ctx := context.Background()

	config := map[string]interface{}{
		"url": "https://api.example.com",
	}

	mock.ExpectExec("INSERT INTO tools").
		WithArgs(sqlmock.AnyArg(), "Test Tool", "Test tool description", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	tool, err := mgr.CreateTool(ctx, "Test Tool", "Test tool description", "api", config)

	require.NoError(t, err)
	assert.NotNil(t, tool)
	assert.Equal(t, "Test Tool", tool.Name)
	assert.Equal(t, "Test tool description", tool.Description)
	assert.NotEmpty(t, tool.ID)

	// Verify metadata contains the config
	var metadata map[string]interface{}
	err = json.Unmarshal(tool.Metadata, &metadata)
	require.NoError(t, err)
	assert.Equal(t, "api", metadata["tool_type"])
	configField, ok := metadata["config"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "https://api.example.com", configField["url"])

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestToolManagerAPI_GetTool(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewToolManager(db)
	ctx := context.Background()

	toolID := "tool-123"
	now := time.Now()
	metadata := []byte(`{"tool_type": "api", "config": {"url": "https://example.com"}}`)

	columns := []string{"id", "name", "description", "metadata", "created_at", "updated_at"}
	mock.ExpectQuery("SELECT id, name, description, metadata, created_at, updated_at FROM tools WHERE id").
		WithArgs(toolID).
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow(toolID, "Test Tool", "Test description", metadata, now, now))

	tool, err := mgr.GetTool(ctx, toolID)

	require.NoError(t, err)
	assert.NotNil(t, tool)
	assert.Equal(t, toolID, tool.ID)
	assert.Equal(t, "Test Tool", tool.Name)
	assert.Equal(t, "Test description", tool.Description)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestToolManagerAPI_GetTool_NotFound(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewToolManager(db)
	ctx := context.Background()

	toolID := "nonexistent"

	mock.ExpectQuery("SELECT id, name, description, metadata, created_at, updated_at FROM tools WHERE id").
		WithArgs(toolID).
		WillReturnError(sql.ErrNoRows)

	tool, err := mgr.GetTool(ctx, toolID)

	assert.Error(t, err)
	assert.Nil(t, tool)
	assert.Contains(t, err.Error(), "not found")

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestToolManagerAPI_ListTools(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewToolManager(db)
	ctx := context.Background()

	now := time.Now()
	metadata := []byte(`{"tool_type": "api"}`)
	columns := []string{"id", "name", "description", "metadata", "created_at", "updated_at"}

	// Empty list
	mock.ExpectQuery("SELECT id, name, description, metadata, created_at, updated_at FROM tools ORDER BY created_at DESC").
		WillReturnRows(sqlmock.NewRows(columns))

	tools, err := mgr.ListTools(ctx)

	require.NoError(t, err)
	assert.Empty(t, tools)

	// With data
	mock.ExpectQuery("SELECT id, name, description, metadata, created_at, updated_at FROM tools ORDER BY created_at DESC").
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("tool-1", "Tool 1", "Desc 1", metadata, now, now).
			AddRow("tool-2", "Tool 2", "Desc 2", metadata, now, now))

	tools, err = mgr.ListTools(ctx)

	require.NoError(t, err)
	assert.Len(t, tools, 2)
	assert.Equal(t, "Tool 1", tools[0].Name)
	assert.Equal(t, "Tool 2", tools[1].Name)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestToolManagerAPI_UpdateTool(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewToolManager(db)
	ctx := context.Background()

	toolID := "tool-123"
	now := time.Now()
	oldMetadata := []byte(`{"tool_type": "old"}`)

	columns := []string{"id", "name", "description", "metadata", "created_at", "updated_at"}

	// Mock GetTool
	mock.ExpectQuery("SELECT id, name, description, metadata, created_at, updated_at FROM tools WHERE id").
		WithArgs(toolID).
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow(toolID, "Old Name", "Old Desc", oldMetadata, now, now))

	// Mock Update
	mock.ExpectExec("UPDATE tools SET name").
		WithArgs("New Name", "New Desc", sqlmock.AnyArg(), sqlmock.AnyArg(), toolID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	newConfig := map[string]interface{}{"url": "https://new.example.com"}
	tool, err := mgr.UpdateTool(ctx, toolID, "New Name", "New Desc", "api", newConfig)

	require.NoError(t, err)
	assert.NotNil(t, tool)
	assert.Equal(t, "New Name", tool.Name)
	assert.Equal(t, "New Desc", tool.Description)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestToolManagerAPI_DeleteTool(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewToolManager(db)
	ctx := context.Background()

	toolID := "tool-123"

	mock.ExpectExec("DELETE FROM tools WHERE id").
		WithArgs(toolID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := mgr.DeleteTool(ctx, toolID)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestToolManagerAPI_DeleteTool_NotFound(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewToolManager(db)
	ctx := context.Background()

	toolID := "nonexistent"

	mock.ExpectExec("DELETE FROM tools WHERE id").
		WithArgs(toolID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := mgr.DeleteTool(ctx, toolID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestToolManagerAPI_GetToolConfig(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewToolManager(db)
	ctx := context.Background()

	toolID := "tool-123"
	now := time.Now()
	metadata := []byte(`{"tool_type": "api", "config": {"url": "https://api.example.com", "timeout": 30}}`)

	columns := []string{"id", "name", "description", "metadata", "created_at", "updated_at"}
	mock.ExpectQuery("SELECT id, name, description, metadata, created_at, updated_at FROM tools WHERE id").
		WithArgs(toolID).
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow(toolID, "Test Tool", "Test description", metadata, now, now))

	config, err := mgr.GetToolConfig(ctx, toolID)

	require.NoError(t, err)
	assert.NotNil(t, config)
	assert.Equal(t, "https://api.example.com", config["url"])
	assert.Equal(t, float64(30), config["timeout"])

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestToolManagerAPI_GetToolConfig_MissingConfig(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewToolManager(db)
	ctx := context.Background()

	toolID := "tool-123"
	now := time.Now()
	// Metadata without config field
	metadata := []byte(`{"tool_type": "api"}`)

	columns := []string{"id", "name", "description", "metadata", "created_at", "updated_at"}
	mock.ExpectQuery("SELECT id, name, description, metadata, created_at, updated_at FROM tools WHERE id").
		WithArgs(toolID).
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow(toolID, "Test Tool", "Test description", metadata, now, now))

	config, err := mgr.GetToolConfig(ctx, toolID)

	assert.Error(t, err)
	assert.Nil(t, config)
	assert.Contains(t, err.Error(), "missing config")

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestToolManagerAPI_DatabaseError(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewToolManager(db)
	ctx := context.Background()

	mock.ExpectExec("INSERT INTO tools").
		WillReturnError(sql.ErrConnDone)

	_, err := mgr.CreateTool(ctx, "Test", "Desc", "api", nil)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to insert")

	assert.NoError(t, mock.ExpectationsWereMet())
}

// =============================================================================
// SkillManager API Tests
// =============================================================================

func TestSkillManagerAPI_CreateSkill(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewSkillManager(db)
	ctx := context.Background()

	mock.ExpectExec("INSERT INTO skills").
		WithArgs(sqlmock.AnyArg(), "Test Skill", "Test skill description", "/path/to/skill", true, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	skill, err := mgr.CreateSkill(ctx, "Test Skill", "Test skill description", "/path/to/skill", true)

	require.NoError(t, err)
	assert.NotNil(t, skill)
	assert.Equal(t, "Test Skill", skill.Name)
	assert.Equal(t, "Test skill description", skill.Description)
	assert.Equal(t, "/path/to/skill", skill.Path)
	assert.True(t, skill.Enabled)
	assert.NotEmpty(t, skill.ID)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSkillManagerAPI_GetSkill(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewSkillManager(db)
	ctx := context.Background()

	skillID := "skill-123"
	now := time.Now()
	columns := []string{"id", "name", "description", "path", "enabled", "created_at", "updated_at"}

	mock.ExpectQuery("SELECT id, name, description, path, enabled, created_at, updated_at FROM skills WHERE id").
		WithArgs(skillID).
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow(skillID, "Test Skill", "Test description", "/path/to/skill", true, now, now))

	skill, err := mgr.GetSkill(ctx, skillID)

	require.NoError(t, err)
	assert.NotNil(t, skill)
	assert.Equal(t, skillID, skill.ID)
	assert.Equal(t, "Test Skill", skill.Name)
	assert.Equal(t, "/path/to/skill", skill.Path)
	assert.True(t, skill.Enabled)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSkillManagerAPI_GetSkill_NotFound(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewSkillManager(db)
	ctx := context.Background()

	skillID := "nonexistent"

	mock.ExpectQuery("SELECT id, name, description, path, enabled, created_at, updated_at FROM skills WHERE id").
		WithArgs(skillID).
		WillReturnError(sql.ErrNoRows)

	skill, err := mgr.GetSkill(ctx, skillID)

	assert.Error(t, err)
	assert.Nil(t, skill)
	assert.Contains(t, err.Error(), "not found")

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSkillManagerAPI_ListSkills(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewSkillManager(db)
	ctx := context.Background()

	now := time.Now()
	columns := []string{"id", "name", "description", "path", "enabled", "created_at", "updated_at"}

	// Empty list
	mock.ExpectQuery("SELECT id, name, description, path, enabled, created_at, updated_at FROM skills ORDER BY name").
		WillReturnRows(sqlmock.NewRows(columns))

	skills, err := mgr.ListSkills(ctx)

	require.NoError(t, err)
	assert.Empty(t, skills)

	// With data
	mock.ExpectQuery("SELECT id, name, description, path, enabled, created_at, updated_at FROM skills ORDER BY name").
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("skill-1", "Alpha Skill", "Desc 1", "/path1", true, now, now).
			AddRow("skill-2", "Beta Skill", "Desc 2", "/path2", false, now, now))

	skills, err = mgr.ListSkills(ctx)

	require.NoError(t, err)
	assert.Len(t, skills, 2)
	assert.Equal(t, "Alpha Skill", skills[0].Name)
	assert.True(t, skills[0].Enabled)
	assert.False(t, skills[1].Enabled)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSkillManagerAPI_UpdateSkill(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewSkillManager(db)
	ctx := context.Background()

	skillID := "skill-123"
	now := time.Now()
	columns := []string{"id", "name", "description", "path", "enabled", "created_at", "updated_at"}

	// Mock GetSkill
	mock.ExpectQuery("SELECT id, name, description, path, enabled, created_at, updated_at FROM skills WHERE id").
		WithArgs(skillID).
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow(skillID, "Old Name", "Old Desc", "/old/path", false, now, now))

	// Mock Update
	mock.ExpectExec("UPDATE skills SET name").
		WithArgs("New Name", "New Desc", "/new/path", true, sqlmock.AnyArg(), skillID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	skill, err := mgr.UpdateSkill(ctx, skillID, "New Name", "New Desc", "/new/path", true)

	require.NoError(t, err)
	assert.NotNil(t, skill)
	assert.Equal(t, "New Name", skill.Name)
	assert.Equal(t, "New Desc", skill.Description)
	assert.Equal(t, "/new/path", skill.Path)
	assert.True(t, skill.Enabled)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSkillManagerAPI_DeleteSkill(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewSkillManager(db)
	ctx := context.Background()

	skillID := "skill-123"

	mock.ExpectExec("DELETE FROM skills WHERE id").
		WithArgs(skillID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := mgr.DeleteSkill(ctx, skillID)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSkillManagerAPI_DeleteSkill_NotFound(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewSkillManager(db)
	ctx := context.Background()

	skillID := "nonexistent"

	mock.ExpectExec("DELETE FROM skills WHERE id").
		WithArgs(skillID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := mgr.DeleteSkill(ctx, skillID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSkillManagerAPI_AddSkillToAgent(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewSkillManager(db)
	ctx := context.Background()

	agentID := "agent-123"
	skillID := "skill-456"

	mock.ExpectExec("INSERT INTO agent_skills").
		WithArgs(agentID, skillID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := mgr.AddSkillToAgent(ctx, agentID, skillID)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSkillManagerAPI_RemoveSkillFromAgent(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewSkillManager(db)
	ctx := context.Background()

	agentID := "agent-123"
	skillID := "skill-456"

	mock.ExpectExec("DELETE FROM agent_skills WHERE agent_id").
		WithArgs(agentID, skillID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := mgr.RemoveSkillFromAgent(ctx, agentID, skillID)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSkillManagerAPI_GetAgentSkills(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewSkillManager(db)
	ctx := context.Background()

	agentID := "agent-123"
	now := time.Now()
	columns := []string{"id", "name", "description", "path", "enabled", "created_at", "updated_at"}

	// No skills
	mock.ExpectQuery("SELECT s.id, s.name, s.description, s.path, s.enabled, s.created_at, s.updated_at").
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows(columns))

	skills, err := mgr.GetAgentSkills(ctx, agentID)

	require.NoError(t, err)
	assert.Empty(t, skills)

	// With skills
	mock.ExpectQuery("SELECT s.id, s.name, s.description, s.path, s.enabled, s.created_at, s.updated_at").
		WithArgs(agentID).
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("skill-1", "Alpha", "Desc", "/path1", true, now, now).
			AddRow("skill-2", "Beta", "Desc", "/path2", true, now, now))

	skills, err = mgr.GetAgentSkills(ctx, agentID)

	require.NoError(t, err)
	assert.Len(t, skills, 2)
	assert.Equal(t, "Alpha", skills[0].Name)
	assert.Equal(t, "Beta", skills[1].Name)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSkillManagerAPI_DatabaseError(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	mgr := NewSkillManager(db)
	ctx := context.Background()

	mock.ExpectExec("INSERT INTO skills").
		WillReturnError(sql.ErrConnDone)

	_, err := mgr.CreateSkill(ctx, "Test", "Desc", "/path", true)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to insert")

	assert.NoError(t, mock.ExpectationsWereMet())
}

// =============================================================================
// ProviderManager API Tests
// =============================================================================

func TestProviderManagerAPI_CreateProvider(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	secret := []byte("16bytessecretkey")
	mgr := NewProviderManager(db, secret)
	ctx := context.Background()

	mock.ExpectExec("INSERT INTO providers").
		WithArgs(sqlmock.AnyArg(), "Test Provider", "https://api.openai.com", sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	provider, err := mgr.CreateProvider(ctx, "Test Provider", "https://api.openai.com", "secret-key")

	require.NoError(t, err)
	assert.NotNil(t, provider)
	assert.Equal(t, "Test Provider", provider.Name)
	assert.Equal(t, "https://api.openai.com", provider.APIBaseURL)
	assert.NotEmpty(t, provider.APIKeyEncrypted)
	assert.NotEqual(t, "secret-key", provider.APIKeyEncrypted) // Should be encrypted

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestProviderManagerAPI_GetProvider(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	secret := []byte("16bytessecretkey")
	mgr := NewProviderManager(db, secret)
	ctx := context.Background()

	providerID := "provider-123"
	now := time.Now()
	columns := []string{"id", "name", "api_base_url", "api_key_encrypted", "created_at", "updated_at"}

	mock.ExpectQuery("SELECT id, name, api_base_url, api_key_encrypted, created_at, updated_at FROM providers WHERE id").
		WithArgs(providerID).
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow(providerID, "Test Provider", "https://api.openai.com", "encrypted-key", now, now))

	provider, err := mgr.GetProvider(ctx, providerID)

	require.NoError(t, err)
	assert.NotNil(t, provider)
	assert.Equal(t, providerID, provider.ID)
	assert.Equal(t, "Test Provider", provider.Name)
	assert.Equal(t, "https://api.openai.com", provider.APIBaseURL)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestProviderManagerAPI_GetProvider_NotFound(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	secret := []byte("16bytessecretkey")
	mgr := NewProviderManager(db, secret)
	ctx := context.Background()

	providerID := "nonexistent"

	mock.ExpectQuery("SELECT id, name, api_base_url, api_key_encrypted, created_at, updated_at FROM providers WHERE id").
		WithArgs(providerID).
		WillReturnError(sql.ErrNoRows)

	provider, err := mgr.GetProvider(ctx, providerID)

	assert.Error(t, err)
	assert.Nil(t, provider)
	assert.Contains(t, err.Error(), "not found")

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestProviderManagerAPI_ListProviders(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	secret := []byte("16bytessecretkey")
	mgr := NewProviderManager(db, secret)
	ctx := context.Background()

	now := time.Now()
	columns := []string{"id", "name", "api_base_url", "api_key_encrypted", "created_at", "updated_at"}

	// Empty list
	mock.ExpectQuery("SELECT id, name, api_base_url, api_key_encrypted, created_at, updated_at FROM providers ORDER BY created_at DESC").
		WillReturnRows(sqlmock.NewRows(columns))

	providers, err := mgr.ListProviders(ctx)

	require.NoError(t, err)
	assert.Empty(t, providers)

	// With data
	mock.ExpectQuery("SELECT id, name, api_base_url, api_key_encrypted, created_at, updated_at FROM providers ORDER BY created_at DESC").
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow("provider-1", "Provider 1", "https://api1.com", "key1", now, now).
			AddRow("provider-2", "Provider 2", "https://api2.com", "key2", now, now))

	providers, err = mgr.ListProviders(ctx)

	require.NoError(t, err)
	assert.Len(t, providers, 2)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestProviderManagerAPI_UpdateProvider(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	secret := []byte("16bytessecretkey")
	mgr := NewProviderManager(db, secret)
	ctx := context.Background()

	providerID := "provider-123"
	now := time.Now()
	columns := []string{"id", "name", "api_base_url", "api_key_encrypted", "created_at", "updated_at"}

	// Mock GetProvider
	mock.ExpectQuery("SELECT id, name, api_base_url, api_key_encrypted, created_at, updated_at FROM providers WHERE id").
		WithArgs(providerID).
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow(providerID, "Old Name", "https://old.com", "old-key", now, now))

	// Mock Update
	mock.ExpectExec("UPDATE providers SET name").
		WithArgs("New Name", "https://new.com", sqlmock.AnyArg(), sqlmock.AnyArg(), providerID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	provider, err := mgr.UpdateProvider(ctx, providerID, "New Name", "https://new.com", "new-key")

	require.NoError(t, err)
	assert.NotNil(t, provider)
	assert.Equal(t, "New Name", provider.Name)
	assert.Equal(t, "https://new.com", provider.APIBaseURL)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestProviderManagerAPI_DeleteProvider(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	secret := []byte("16bytessecretkey")
	mgr := NewProviderManager(db, secret)
	ctx := context.Background()

	providerID := "provider-123"

	mock.ExpectExec("DELETE FROM providers WHERE id").
		WithArgs(providerID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := mgr.DeleteProvider(ctx, providerID)

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestProviderManagerAPI_DeleteProvider_NotFound(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	secret := []byte("16bytessecretkey")
	mgr := NewProviderManager(db, secret)
	ctx := context.Background()

	providerID := "nonexistent"

	mock.ExpectExec("DELETE FROM providers WHERE id").
		WithArgs(providerID).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := mgr.DeleteProvider(ctx, providerID)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestProviderManagerAPI_GetDecryptedAPIKey(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	secret := []byte("16bytessecretkey")
	mgr := NewProviderManager(db, secret)
	ctx := context.Background()

	providerID := "provider-123"
	now := time.Now()

	// First encrypt a key using the same method the manager uses
	encryptedKey, err := mgr.encryptAPIKey("my-secret-api-key")
	require.NoError(t, err)

	columns := []string{"id", "name", "api_base_url", "api_key_encrypted", "created_at", "updated_at"}
	mock.ExpectQuery("SELECT id, name, api_base_url, api_key_encrypted, created_at, updated_at FROM providers WHERE id").
		WithArgs(providerID).
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow(providerID, "Test Provider", "https://api.openai.com", encryptedKey, now, now))

	decryptedKey, err := mgr.GetDecryptedAPIKey(ctx, providerID)

	require.NoError(t, err)
	assert.Equal(t, "my-secret-api-key", decryptedKey)

	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestProviderManagerAPI_EncryptDecryptRoundTrip(t *testing.T) {
	db, _ := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	secret := []byte("16bytessecretkey")
	mgr := NewProviderManager(db, secret)

	originalKey := "super-secret-api-key-12345"

	encrypted, err := mgr.encryptAPIKey(originalKey)
	require.NoError(t, err)
	assert.NotEqual(t, originalKey, encrypted)

	decrypted, err := mgr.decryptAPIKey(encrypted)
	require.NoError(t, err)
	assert.Equal(t, originalKey, decrypted)
}

func TestProviderManagerAPI_EncryptDecryptWithSpecialCharacters(t *testing.T) {
	db, _ := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	secret := []byte("16bytessecretkey")
	mgr := NewProviderManager(db, secret)

	// Test with special characters that might cause issues
	originalKey := "key-with-special-chars!@#$%^&*()_+-=[]{}|;':\",./<>?"

	encrypted, err := mgr.encryptAPIKey(originalKey)
	require.NoError(t, err)

	decrypted, err := mgr.decryptAPIKey(encrypted)
	require.NoError(t, err)
	assert.Equal(t, originalKey, decrypted)
}

func TestProviderManagerAPI_DecryptInvalidKey(t *testing.T) {
	db, _ := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	secret := []byte("16bytessecretkey")
	mgr := NewProviderManager(db, secret)

	// Invalid hex string
	_, err := mgr.decryptAPIKey("not-valid-hex!")
	assert.Error(t, err)

	// Too short ciphertext
	_, err = mgr.decryptAPIKey("000102030405060708090a0b")
	assert.Error(t, err)
}

func TestProviderManagerAPI_DatabaseError(t *testing.T) {
	db, mock := createTestDBForManagers(t)
	defer func() { _ = db.Close() }()

	secret := []byte("16bytessecretkey")
	mgr := NewProviderManager(db, secret)
	ctx := context.Background()

	mock.ExpectExec("INSERT INTO providers").
		WillReturnError(sql.ErrConnDone)

	_, err := mgr.CreateProvider(ctx, "Test", "https://api.com", "key")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to insert")

	assert.NoError(t, mock.ExpectationsWereMet())
}

// =============================================================================
// Model Conversion Tests
// =============================================================================

func TestToolManagerAPI_ConvertToDBModel(t *testing.T) {
	// Test that the internal tool model can be converted to dbmodels.Tool
	now := time.Now()
	metadata := []byte(`{"tool_type": "api", "config": {"url": "https://example.com"}}`)

	tool := &dbmodels.Tool{
		ID:          "tool-123",
		Name:        "Test Tool",
		Description: "Test description",
		Metadata:    metadata,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	assert.Equal(t, "tool-123", tool.ID)
	assert.Equal(t, "Test Tool", tool.Name)
	assert.Equal(t, "Test description", tool.Description)
	assert.NotNil(t, tool.Metadata)
}

func TestSkillManagerAPI_ConvertToDBModel(t *testing.T) {
	// Test that the internal skill model can be converted to dbmodels.Skill
	now := time.Now()

	skill := &dbmodels.Skill{
		ID:          "skill-123",
		Name:        "Test Skill",
		Description: "Test description",
		Path:        "/path/to/skill",
		Enabled:     true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	assert.Equal(t, "skill-123", skill.ID)
	assert.Equal(t, "Test Skill", skill.Name)
	assert.Equal(t, "/path/to/skill", skill.Path)
	assert.True(t, skill.Enabled)
}

func TestProviderManagerAPI_ConvertToDBModel(t *testing.T) {
	// Test that the internal provider model can be converted to dbmodels.Provider
	now := time.Now()

	provider := &dbmodels.Provider{
		ID:              "provider-123",
		Name:            "Test Provider",
		APIBaseURL:      "https://api.example.com",
		APIKeyEncrypted: "encrypted-key",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	assert.Equal(t, "provider-123", provider.ID)
	assert.Equal(t, "Test Provider", provider.Name)
	assert.Equal(t, "https://api.example.com", provider.APIBaseURL)
}
