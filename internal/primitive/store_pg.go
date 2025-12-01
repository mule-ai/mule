package primitive

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/google/uuid"
)

// PGStore implements PrimitiveStore backed by PostgreSQL.
type PGStore struct {
	db *sql.DB
}

// NewPGStore creates a new PGStore instance.
func NewPGStore(db *sql.DB) *PGStore {
	return &PGStore{db: db}
}

func (s *PGStore) CreateProvider(ctx context.Context, p *Provider) error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}
	query := `INSERT INTO providers (id, name, api_base_url, api_key_encrypted, created_at, updated_at) VALUES ($1, $2, $3, $4, NOW(), NOW())`
	_, err := s.db.ExecContext(ctx, query, p.ID, p.Name, p.APIBaseURL, []byte(p.APIKeyEnc))
	return err
}

func (s *PGStore) GetProvider(ctx context.Context, id string) (*Provider, error) {
	p := &Provider{}
	var apiKeyEncrypted []byte
	query := `SELECT id, name, api_base_url, api_key_encrypted, created_at, updated_at FROM providers WHERE id = $1`
	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&p.ID, &p.Name, &p.APIBaseURL, &apiKeyEncrypted, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.APIKeyEnc = string(apiKeyEncrypted)
	return p, nil
}

func (s *PGStore) ListProviders(ctx context.Context) ([]*Provider, error) {
	query := `SELECT id, name, api_base_url, api_key_encrypted, created_at, updated_at FROM providers ORDER BY name`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Error closing rows: %v", closeErr)
		}
	}()

	providers := []*Provider{}
	for rows.Next() {
		p := &Provider{}
		var apiKeyEncrypted []byte
		err := rows.Scan(&p.ID, &p.Name, &p.APIBaseURL, &apiKeyEncrypted, &p.CreatedAt, &p.UpdatedAt)
		if err != nil {
			return nil, err
		}
		p.APIKeyEnc = string(apiKeyEncrypted)
		providers = append(providers, p)
	}
	return providers, rows.Err()
}

func (s *PGStore) UpdateProvider(ctx context.Context, p *Provider) error {
	query := `UPDATE providers SET name = $1, api_base_url = $2, api_key_encrypted = $3, updated_at = NOW() WHERE id = $4`
	res, err := s.db.ExecContext(ctx, query, p.Name, p.APIBaseURL, []byte(p.APIKeyEnc), p.ID)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PGStore) DeleteProvider(ctx context.Context, id string) error {
	query := `DELETE FROM providers WHERE id = $1`
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PGStore) CreateTool(ctx context.Context, t *Tool) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	metadataJSON, err := json.Marshal(t.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal tool metadata: %w", err)
	}
	query := `INSERT INTO tools (id, name, description, metadata, created_at, updated_at) VALUES ($1, $2, $3, $4, NOW(), NOW())`
	_, err = s.db.ExecContext(ctx, query, t.ID, t.Name, t.Description, metadataJSON)
	return err
}

func (s *PGStore) GetTool(ctx context.Context, id string) (*Tool, error) {
	t := &Tool{}
	var metadataJSON []byte
	query := `SELECT id, name, description, metadata, created_at, updated_at FROM tools WHERE id = $1`
	err := s.db.QueryRowContext(ctx, query, id).Scan(&t.ID, &t.Name, &t.Description, &metadataJSON, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(metadataJSON, &t.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tool metadata: %w", err)
	}
	return t, nil
}

func (s *PGStore) ListTools(ctx context.Context) ([]*Tool, error) {
	query := `SELECT id, name, description, metadata, created_at, updated_at FROM tools ORDER BY name`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Error closing rows: %v", closeErr)
		}
	}()

	var tools []*Tool
	for rows.Next() {
		t := &Tool{}
		var metadataJSON []byte
		err := rows.Scan(&t.ID, &t.Name, &t.Description, &metadataJSON, &t.CreatedAt, &t.UpdatedAt)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(metadataJSON, &t.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tool metadata: %w", err)
		}
		tools = append(tools, t)
	}
	return tools, rows.Err()
}

func (s *PGStore) UpdateTool(ctx context.Context, t *Tool) error {
	metadataJSON, err := json.Marshal(t.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal tool metadata: %w", err)
	}
	query := `UPDATE tools SET name = $1, description = $2, metadata = $3, updated_at = NOW() WHERE id = $4`
	res, err := s.db.ExecContext(ctx, query, t.Name, t.Description, metadataJSON, t.ID)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PGStore) DeleteTool(ctx context.Context, id string) error {
	query := `DELETE FROM tools WHERE id = $1`
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PGStore) CreateAgent(ctx context.Context, a *Agent) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	query := `INSERT INTO agents (id, name, description, provider_id, model_id, system_prompt, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`
	_, err := s.db.ExecContext(ctx, query, a.ID, a.Name, a.Description, a.ProviderID, a.ModelID, a.SystemPrompt)
	return err
}

func (s *PGStore) GetAgent(ctx context.Context, id string) (*Agent, error) {
	a := &Agent{}
	query := `SELECT id, name, description, provider_id, model_id, system_prompt, created_at, updated_at FROM agents WHERE id = $1`
	err := s.db.QueryRowContext(ctx, query, id).Scan(&a.ID, &a.Name, &a.Description, &a.ProviderID, &a.ModelID, &a.SystemPrompt, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

func (s *PGStore) ListAgents(ctx context.Context) ([]*Agent, error) {
	query := `SELECT id, name, description, provider_id, model_id, system_prompt, created_at, updated_at FROM agents ORDER BY name`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Error closing rows: %v", closeErr)
		}
	}()

	var agents []*Agent
	for rows.Next() {
		a := &Agent{}
		err := rows.Scan(&a.ID, &a.Name, &a.Description, &a.ProviderID, &a.ModelID, &a.SystemPrompt, &a.CreatedAt, &a.UpdatedAt)
		if err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	return agents, rows.Err()
}

func (s *PGStore) UpdateAgent(ctx context.Context, a *Agent) error {
	query := `UPDATE agents SET name = $1, description = $2, provider_id = $3, model_id = $4, system_prompt = $5, updated_at = NOW() WHERE id = $6`
	res, err := s.db.ExecContext(ctx, query, a.Name, a.Description, a.ProviderID, a.ModelID, a.SystemPrompt, a.ID)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PGStore) DeleteAgent(ctx context.Context, id string) error {
	query := `DELETE FROM agents WHERE id = $1`
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PGStore) CreateWorkflow(ctx context.Context, w *Workflow) error {
	if w.ID == "" {
		w.ID = uuid.New().String()
	}
	query := `INSERT INTO workflows (id, name, description, is_async, created_at, updated_at) VALUES ($1, $2, $3, $4, NOW(), NOW())`
	_, err := s.db.ExecContext(ctx, query, w.ID, w.Name, w.Description, w.IsAsync)
	return err
}

func (s *PGStore) GetWorkflow(ctx context.Context, id string) (*Workflow, error) {
	w := &Workflow{}
	query := `SELECT id, name, description, is_async, created_at, updated_at FROM workflows WHERE id = $1`
	err := s.db.QueryRowContext(ctx, query, id).Scan(&w.ID, &w.Name, &w.Description, &w.IsAsync, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return w, err
}

func (s *PGStore) ListWorkflows(ctx context.Context) ([]*Workflow, error) {
	query := `SELECT id, name, description, is_async, created_at, updated_at FROM workflows ORDER BY name`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Error closing rows: %v", closeErr)
		}
	}()

	var workflows []*Workflow
	for rows.Next() {
		w := &Workflow{}
		err := rows.Scan(&w.ID, &w.Name, &w.Description, &w.IsAsync, &w.CreatedAt, &w.UpdatedAt)
		if err != nil {
			return nil, err
		}
		workflows = append(workflows, w)
	}
	return workflows, rows.Err()
}

func (s *PGStore) UpdateWorkflow(ctx context.Context, w *Workflow) error {
	query := `UPDATE workflows SET name = $1, description = $2, is_async = $3, updated_at = NOW() WHERE id = $4`
	res, err := s.db.ExecContext(ctx, query, w.Name, w.Description, w.IsAsync, w.ID)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PGStore) DeleteWorkflow(ctx context.Context, id string) error {
	query := `DELETE FROM workflows WHERE id = $1`
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PGStore) CreateWorkflowStep(ctx context.Context, step *WorkflowStep) error {
	if step.ID == "" {
		step.ID = uuid.New().String()
	}
	configJSON, err := json.Marshal(step.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal workflow step config: %w", err)
	}
	query := `INSERT INTO workflow_steps (id, workflow_id, step_order, step_type, agent_id, wasm_module_id, config, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`
	_, err = s.db.ExecContext(ctx, query, step.ID, step.WorkflowID, step.StepOrder, step.StepType, step.AgentID, step.WasmModuleID, configJSON)
	return err
}

func (s *PGStore) ListWorkflowSteps(ctx context.Context, workflowID string) ([]*WorkflowStep, error) {
	query := `SELECT id, workflow_id, step_order, step_type, agent_id, wasm_module_id, config, created_at FROM workflow_steps WHERE workflow_id = $1 ORDER BY step_order`
	rows, err := s.db.QueryContext(ctx, query, workflowID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Error closing rows: %v", closeErr)
		}
	}()

	var steps []*WorkflowStep
	for rows.Next() {
		step := &WorkflowStep{}
		var configJSON []byte
		err := rows.Scan(&step.ID, &step.WorkflowID, &step.StepOrder, &step.StepType, &step.AgentID, &step.WasmModuleID, &configJSON, &step.CreatedAt)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(configJSON, &step.Config); err != nil {
			return nil, fmt.Errorf("failed to unmarshal workflow step config: %w", err)
		}
		steps = append(steps, step)
	}
	return steps, rows.Err()
}

// GetAgentTools retrieves tools associated with an agent
func (s *PGStore) GetAgentTools(ctx context.Context, agentID string) ([]*Tool, error) {
	query := `
		SELECT t.id, t.name, t.description, t.metadata, t.created_at, t.updated_at
		FROM tools t
		JOIN agent_tools at ON t.id = at.tool_id
		WHERE at.agent_id = $1
		ORDER BY t.created_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to query agent tools: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Error closing rows: %v", closeErr)
		}
	}()

	var tools []*Tool
	for rows.Next() {
		tool := &Tool{}
		var metadataJSON []byte
		err := rows.Scan(
			&tool.ID,
			&tool.Name,
			&tool.Description,
			&metadataJSON,
			&tool.CreatedAt,
			&tool.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan tool: %w", err)
		}
		if err = json.Unmarshal(metadataJSON, &tool.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal tool metadata: %w", err)
		}
		tools = append(tools, tool)
	}

	return tools, rows.Err()
}

// AssignToolToAgent assigns a tool to an agent
func (s *PGStore) AssignToolToAgent(ctx context.Context, agentID, toolID string) error {
	query := `INSERT INTO agent_tools (agent_id, tool_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
	_, err := s.db.ExecContext(ctx, query, agentID, toolID)
	if err != nil {
		return fmt.Errorf("failed to assign tool to agent: %w", err)
	}
	return nil
}

// RemoveToolFromAgent removes a tool from an agent
func (s *PGStore) RemoveToolFromAgent(ctx context.Context, agentID, toolID string) error {
	query := `DELETE FROM agent_tools WHERE agent_id = $1 AND tool_id = $2`
	result, err := s.db.ExecContext(ctx, query, agentID, toolID)
	if err != nil {
		return fmt.Errorf("failed to remove tool from agent: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

// DB returns the underlying database connection
func (s *PGStore) DB() *sql.DB {
	return s.db
}

// Memory configuration methods
func (s *PGStore) GetMemoryConfig(ctx context.Context, id string) (*MemoryConfig, error) {
	if id == "" {
		id = "default"
	}

	config := &MemoryConfig{}
	query := `SELECT id, database_url, embedding_provider, embedding_model, embedding_dims,
			  default_ttl_seconds, default_top_k, created_at, updated_at
			  FROM memory_config WHERE id = $1`

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&config.ID, &config.DatabaseURL, &config.EmbeddingProvider, &config.EmbeddingModel,
		&config.EmbeddingDims, &config.DefaultTTLSeconds, &config.DefaultTopK,
		&config.CreatedAt, &config.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get memory config: %w", err)
	}

	return config, nil
}

func (s *PGStore) UpdateMemoryConfig(ctx context.Context, config *MemoryConfig) error {
	if config.ID == "" {
		config.ID = "default"
	}

	query := `UPDATE memory_config
			  SET database_url = $1, embedding_provider = $2, embedding_model = $3,
				  embedding_dims = $4, default_ttl_seconds = $5, default_top_k = $6,
				  updated_at = NOW()
			  WHERE id = $7`

	res, err := s.db.ExecContext(ctx, query,
		config.DatabaseURL, config.EmbeddingProvider, config.EmbeddingModel,
		config.EmbeddingDims, config.DefaultTTLSeconds, config.DefaultTopK,
		config.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update memory config: %w", err)
	}

	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}

	return nil
}

// Settings methods
func (s *PGStore) GetSetting(ctx context.Context, key string) (*Setting, error) {
	setting := &Setting{}
	query := `SELECT id, key, value, description, category, created_at, updated_at FROM settings WHERE key = $1`

	err := s.db.QueryRowContext(ctx, query, key).Scan(
		&setting.ID, &setting.Key, &setting.Value, &setting.Description,
		&setting.Category, &setting.CreatedAt, &setting.UpdatedAt,
	)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get setting: %w", err)
	}

	return setting, nil
}

func (s *PGStore) ListSettings(ctx context.Context) ([]*Setting, error) {
	query := `SELECT id, key, value, description, category, created_at, updated_at FROM settings ORDER BY category, key`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list settings: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Error closing rows: %v", closeErr)
		}
	}()

	var settings []*Setting
	for rows.Next() {
		setting := &Setting{}
		err := rows.Scan(
			&setting.ID, &setting.Key, &setting.Value, &setting.Description,
			&setting.Category, &setting.CreatedAt, &setting.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan setting: %w", err)
		}
		settings = append(settings, setting)
	}

	return settings, rows.Err()
}

func (s *PGStore) UpdateSetting(ctx context.Context, setting *Setting) error {
	query := `UPDATE settings SET value = $1, description = $2, category = $3, updated_at = NOW() WHERE key = $4`
	res, err := s.db.ExecContext(ctx, query, setting.Value, setting.Description, setting.Category, setting.Key)
	if err != nil {
		return fmt.Errorf("failed to update setting: %w", err)
	}

	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}

	return nil
}

// WASM module methods
func (s *PGStore) CreateWasmModule(ctx context.Context, w *WasmModule) error {
	if w.ID == "" {
		w.ID = uuid.New().String()
	}

	// Convert config bytes to JSON string for PostgreSQL JSONB column
	var configJSON interface{}
	if len(w.Config) > 0 {
		configJSON = string(w.Config)
	} else {
		configJSON = nil
	}

	query := `INSERT INTO wasm_modules (id, name, description, module_data, config, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, NOW(), NOW())`
	_, err := s.db.ExecContext(ctx, query, w.ID, w.Name, w.Description, w.ModuleData, configJSON)
	return err
}

func (s *PGStore) GetWasmModule(ctx context.Context, id string) (*WasmModule, error) {
	w := &WasmModule{}
	query := `SELECT id, name, description, module_data, config, created_at, updated_at FROM wasm_modules WHERE id = $1`
	err := s.db.QueryRowContext(ctx, query, id).Scan(&w.ID, &w.Name, &w.Description, &w.ModuleData, &w.Config, &w.CreatedAt, &w.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return w, err
}

func (s *PGStore) ListWasmModules(ctx context.Context) ([]*WasmModuleListItem, error) {
	query := `SELECT id, name, description, config, created_at, updated_at FROM wasm_modules ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Error closing rows: %v", closeErr)
		}
	}()

	var modules []*WasmModuleListItem
	for rows.Next() {
		w := &WasmModuleListItem{}
		err := rows.Scan(&w.ID, &w.Name, &w.Description, &w.Config, &w.CreatedAt, &w.UpdatedAt)
		if err != nil {
			return nil, err
		}
		modules = append(modules, w)
	}
	return modules, rows.Err()
}

func (s *PGStore) UpdateWasmModule(ctx context.Context, w *WasmModule) error {
	// Convert config bytes to JSON string for PostgreSQL JSONB column
	var configJSON interface{}
	if len(w.Config) > 0 {
		configJSON = string(w.Config)
	} else {
		configJSON = nil
	}

	query := `UPDATE wasm_modules SET name = $1, description = $2, module_data = $3, config = $4, updated_at = NOW() WHERE id = $5`
	res, err := s.db.ExecContext(ctx, query, w.Name, w.Description, w.ModuleData, configJSON, w.ID)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PGStore) DeleteWasmModule(ctx context.Context, id string) error {
	query := `DELETE FROM wasm_modules WHERE id = $1`
	res, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	count, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}
