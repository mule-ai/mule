package job

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq"
)

// PGStore implements JobStore backed by PostgreSQL
type PGStore struct {
	db *sql.DB
}

// NewPGStore creates a new PGStore instance
func NewPGStore(db *sql.DB) *PGStore {
	return &PGStore{db: db}
}

// CreateJob creates a new job
func (s *PGStore) CreateJob(job *Job) error {
	inputDataJSON, err := json.Marshal(job.InputData)
	if err != nil {
		return fmt.Errorf("failed to marshal input data: %w", err)
	}

	outputDataJSON, err := json.Marshal(job.OutputData)
	if err != nil {
		return fmt.Errorf("failed to marshal output data: %w", err)
	}

	// Handle NULL values for workflow_id and wasm_module_id
	var workflowID interface{} = job.WorkflowID
	if job.WorkflowID == "" {
		workflowID = nil
	}

	query := `INSERT INTO jobs (id, workflow_id, wasm_module_id, status, input_data, output_data, working_directory, created_at)
			  VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`

	_, err = s.db.Exec(query, job.ID, workflowID, job.WasmModuleID, job.Status, inputDataJSON, outputDataJSON, job.WorkingDirectory)
	return err
}

// GetJob retrieves a job by ID
func (s *PGStore) GetJob(id string) (*Job, error) {
	job := &Job{}
	var inputDataJSON, outputDataJSON []byte
	var workflowID sql.NullString
	var workingDirectory sql.NullString

	query := `SELECT id, workflow_id, wasm_module_id, status, input_data, output_data, working_directory, created_at, started_at, completed_at
			  FROM jobs WHERE id = $1`

	err := s.db.QueryRow(query, id).Scan(
		&job.ID, &workflowID, &job.WasmModuleID, &job.Status, &inputDataJSON, &outputDataJSON, &workingDirectory,
		&job.CreatedAt, &job.StartedAt, &job.CompletedAt)

	// Convert NULL workflow_id to empty string
	if workflowID.Valid {
		job.WorkflowID = workflowID.String
	} else {
		job.WorkflowID = ""
	}

	// Convert NULL working_directory to empty string
	if workingDirectory.Valid {
		job.WorkingDirectory = workingDirectory.String
	} else {
		job.WorkingDirectory = ""
	}

	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("job not found")
	}
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(inputDataJSON, &job.InputData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal input data: %w", err)
	}

	if err = json.Unmarshal(outputDataJSON, &job.OutputData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal output data: %w", err)
	}

	return job, nil
}

// ListJobs retrieves jobs with pagination and filtering support
func (s *PGStore) ListJobs(opts ListJobsOptions) ([]*Job, int, error) {
	// Set default values if not provided
	if opts.Page <= 0 {
		opts.Page = 1
	}
	if opts.PageSize <= 0 {
		opts.PageSize = 20
	}

	// Base query
	baseQuery := `SELECT j.id, j.workflow_id, j.wasm_module_id, j.status, j.input_data, j.output_data, j.working_directory, j.created_at, j.started_at, j.completed_at
				  FROM jobs j`
	countQuery := `SELECT COUNT(*) FROM jobs j`

	// Build WHERE clause
	whereClause := ""
	args := []interface{}{}
	argIndex := 1

	// Status filter
	if opts.Status != nil {
		if whereClause == "" {
			whereClause = " WHERE"
		} else {
			whereClause += " AND"
		}
		whereClause += fmt.Sprintf(" j.status = $%d", argIndex)
		args = append(args, string(*opts.Status))
		argIndex++
	}

	// Search filter (searches in workflow_id and working_directory)
	if opts.Search != "" {
		if whereClause == "" {
			whereClause = " WHERE"
		} else {
			whereClause += " AND"
		}
		whereClause += fmt.Sprintf(" (j.workflow_id::text ILIKE $%d OR j.working_directory ILIKE $%d)", argIndex, argIndex)
		searchTerm := "%" + opts.Search + "%"
		args = append(args, searchTerm)
		argIndex++
	}

	// Workflow name filter
	if opts.WorkflowName != "" {
		if whereClause == "" {
			whereClause = " WHERE"
		} else {
			whereClause += " AND"
		}
		// Join with workflows table to filter by workflow name
		baseQuery += " INNER JOIN workflows w ON j.workflow_id = w.id"
		countQuery += " INNER JOIN workflows w ON j.workflow_id = w.id"
		whereClause += fmt.Sprintf(" w.name ILIKE $%d", argIndex)
		searchTerm := "%" + opts.WorkflowName + "%"
		args = append(args, searchTerm)
		argIndex++
	}

	// Complete queries
	query := baseQuery + whereClause + " ORDER BY j.created_at DESC LIMIT $%d OFFSET $%d"
	query = fmt.Sprintf(query, argIndex, argIndex+1)
	args = append(args, opts.PageSize, (opts.Page-1)*opts.PageSize)

	countQuery += whereClause
	if whereClause != "" {
		countQuery += ";"
	}

	// Get total count
	var totalCount int
	err := s.db.QueryRow(countQuery, args[:argIndex-1]...).Scan(&totalCount)
	if err != nil {
		return nil, 0, err
	}

	// Get jobs
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Error closing rows: %v", closeErr)
		}
	}()

	var jobs []*Job
	for rows.Next() {
		job := &Job{}
		var inputDataJSON, outputDataJSON []byte
		var workflowID sql.NullString
		var workingDirectory sql.NullString

		err := rows.Scan(&job.ID, &workflowID, &job.WasmModuleID, &job.Status, &inputDataJSON, &outputDataJSON, &workingDirectory,
			&job.CreatedAt, &job.StartedAt, &job.CompletedAt)

		// Convert NULL workflow_id to empty string
		if workflowID.Valid {
			job.WorkflowID = workflowID.String
		} else {
			job.WorkflowID = ""
		}

		// Convert NULL working_directory to empty string
		if workingDirectory.Valid {
			job.WorkingDirectory = workingDirectory.String
		} else {
			job.WorkingDirectory = ""
		}

		if err != nil {
			return nil, 0, err
		}

		if err = json.Unmarshal(inputDataJSON, &job.InputData); err != nil {
			return nil, 0, fmt.Errorf("failed to unmarshal input data: %w", err)
		}

		if err = json.Unmarshal(outputDataJSON, &job.OutputData); err != nil {
			return nil, 0, fmt.Errorf("failed to unmarshal output data: %w", err)
		}

		jobs = append(jobs, job)
	}

	return jobs, totalCount, rows.Err()
}

// UpdateJob updates an existing job
func (s *PGStore) UpdateJob(job *Job) error {
	inputDataJSON, err := json.Marshal(job.InputData)
	if err != nil {
		return fmt.Errorf("failed to marshal input data: %w", err)
	}

	outputDataJSON, err := json.Marshal(job.OutputData)
	if err != nil {
		return fmt.Errorf("failed to marshal output data: %w", err)
	}

	// Handle NULL values for workflow_id
	var workflowID interface{} = job.WorkflowID
	if job.WorkflowID == "" {
		workflowID = nil
	}

	query := `UPDATE jobs SET workflow_id = $1, wasm_module_id = $2, status = $3, input_data = $4, output_data = $5,
			  working_directory = $6, started_at = $7, completed_at = $8 WHERE id = $9`

	result, err := s.db.Exec(query, workflowID, job.WasmModuleID, job.Status, inputDataJSON, outputDataJSON,
		job.WorkingDirectory, job.StartedAt, job.CompletedAt, job.ID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return errors.New("job not found")
	}

	return nil
}

// DeleteJob deletes a job
func (s *PGStore) DeleteJob(id string) error {
	query := `DELETE FROM jobs WHERE id = $1`
	result, err := s.db.Exec(query, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return errors.New("job not found")
	}

	return nil
}

// CreateJobStep creates a new job step
func (s *PGStore) CreateJobStep(step *JobStep) error {
	inputDataJSON, err := json.Marshal(step.InputData)
	if err != nil {
		return fmt.Errorf("failed to marshal input data: %w", err)
	}

	outputDataJSON, err := json.Marshal(step.OutputData)
	if err != nil {
		return fmt.Errorf("failed to marshal output data: %w", err)
	}

	query := `INSERT INTO job_steps (id, job_id, workflow_step_id, step_order, status, input_data, output_data)
			  VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err = s.db.Exec(query, step.ID, step.JobID, step.WorkflowStepID, step.StepOrder, step.Status,
		inputDataJSON, outputDataJSON)
	return err
}

// GetJobStep retrieves a job step by ID
func (s *PGStore) GetJobStep(id string) (*JobStep, error) {
	step := &JobStep{}
	var inputDataJSON, outputDataJSON []byte

	query := `SELECT id, job_id, workflow_step_id, status, input_data, output_data, started_at, completed_at 
			  FROM job_steps WHERE id = $1`

	err := s.db.QueryRow(query, id).Scan(
		&step.ID, &step.JobID, &step.WorkflowStepID, &step.Status, &inputDataJSON, &outputDataJSON,
		&step.StartedAt, &step.CompletedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("job step not found")
	}
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(inputDataJSON, &step.InputData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal input data: %w", err)
	}

	if err = json.Unmarshal(outputDataJSON, &step.OutputData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal output data: %w", err)
	}

	return step, nil
}

// ListJobSteps retrieves all steps for a job
func (s *PGStore) ListJobSteps(jobID string) ([]*JobStep, error) {
	query := `SELECT id, job_id, workflow_step_id, step_order, status, input_data, output_data, started_at, completed_at, error_message
			  FROM job_steps WHERE job_id = $1 ORDER BY created_at`

	rows, err := s.db.Query(query, jobID)
	if err != nil {
		return nil, err
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Printf("Error closing rows: %v", closeErr)
		}
	}()

	var steps []*JobStep
	for rows.Next() {
		step := &JobStep{}
		var inputDataJSON, outputDataJSON []byte

		err := rows.Scan(&step.ID, &step.JobID, &step.WorkflowStepID, &step.StepOrder, &step.Status, &inputDataJSON, &outputDataJSON,
			&step.StartedAt, &step.CompletedAt, &step.ErrorMessage)
		if err != nil {
			return nil, err
		}

		if err = json.Unmarshal(inputDataJSON, &step.InputData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal input data: %w", err)
		}

		if err = json.Unmarshal(outputDataJSON, &step.OutputData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal output data: %w", err)
		}

		steps = append(steps, step)
	}

	return steps, rows.Err()
}

// UpdateJobStep updates an existing job step
func (s *PGStore) UpdateJobStep(step *JobStep) error {
	inputDataJSON, err := json.Marshal(step.InputData)
	if err != nil {
		return fmt.Errorf("failed to marshal input data: %w", err)
	}

	outputDataJSON, err := json.Marshal(step.OutputData)
	if err != nil {
		return fmt.Errorf("failed to marshal output data: %w", err)
	}

	query := `UPDATE job_steps SET status = $1, input_data = $2, output_data = $3,
			  started_at = $4, completed_at = $5, step_order = $6, error_message = $7 WHERE id = $8`

	result, err := s.db.Exec(query, step.Status, inputDataJSON, outputDataJSON,
		step.StartedAt, step.CompletedAt, step.StepOrder, step.ErrorMessage, step.ID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return errors.New("job step not found")
	}

	return nil
}

// DeleteJobStep deletes a job step
func (s *PGStore) DeleteJobStep(id string) error {
	query := `DELETE FROM job_steps WHERE id = $1`
	result, err := s.db.Exec(query, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return errors.New("job step not found")
	}

	return nil
}

// GetNextQueuedJob retrieves the next queued job for processing
func (s *PGStore) GetNextQueuedJob() (*Job, error) {
	query := `SELECT id, workflow_id, status, input_data, output_data, working_directory, created_at, started_at, completed_at
			  FROM jobs WHERE status = 'queued' ORDER BY created_at ASC LIMIT 1`

	job := &Job{}
	var inputDataJSON, outputDataJSON []byte
	var workflowID sql.NullString
	var workingDirectory sql.NullString

	err := s.db.QueryRow(query).Scan(
		&job.ID, &workflowID, &job.Status, &inputDataJSON, &outputDataJSON, &workingDirectory,
		&job.CreatedAt, &job.StartedAt, &job.CompletedAt)

	// Convert NULL workflow_id to empty string
	if workflowID.Valid {
		job.WorkflowID = workflowID.String
	} else {
		job.WorkflowID = ""
	}

	// Convert NULL working_directory to empty string
	if workingDirectory.Valid {
		job.WorkingDirectory = workingDirectory.String
	} else {
		job.WorkingDirectory = ""
	}

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil // No queued jobs
	}
	if err != nil {
		return nil, err
	}

	if err = json.Unmarshal(inputDataJSON, &job.InputData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal input data: %w", err)
	}

	if err = json.Unmarshal(outputDataJSON, &job.OutputData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal output data: %w", err)
	}

	return job, nil
}

// MarkJobRunning marks a job as running
func (s *PGStore) MarkJobRunning(jobID string) error {
	now := time.Now()
	query := `UPDATE jobs SET status = 'running', started_at = $1 WHERE id = $2`

	result, err := s.db.Exec(query, now, jobID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return errors.New("job not found")
	}

	return nil
}

// MarkJobCompleted marks a job as completed
func (s *PGStore) MarkJobCompleted(jobID string, outputData map[string]interface{}) error {
	now := time.Now()
	outputDataJSON, err := json.Marshal(outputData)
	if err != nil {
		return fmt.Errorf("failed to marshal output data: %w", err)
	}

	query := `UPDATE jobs SET status = 'completed', output_data = $1, completed_at = $2 WHERE id = $3`

	result, err := s.db.Exec(query, outputDataJSON, now, jobID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return errors.New("job not found")
	}

	return nil
}

// MarkJobFailed marks a job as failed
func (s *PGStore) MarkJobFailed(jobID string, err error) error {
	now := time.Now()
	// Store error message in output_data
	outputData := map[string]interface{}{"error": err.Error()}
	outputDataJSON, marshalErr := json.Marshal(outputData)
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal error data: %w", marshalErr)
	}

	query := `UPDATE jobs SET status = 'failed', output_data = $1, completed_at = $2 WHERE id = $3`

	result, execErr := s.db.Exec(query, outputDataJSON, now, jobID)
	if execErr != nil {
		return execErr
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return errors.New("job not found")
	}

	return nil
}

// CancelJob marks a job as cancelled
func (s *PGStore) CancelJob(jobID string) error {
	now := time.Now()
	query := `UPDATE jobs SET status = 'cancelled', completed_at = $1 WHERE id = $2 AND status IN ('queued', 'running')`

	result, err := s.db.Exec(query, now, jobID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return errors.New("job not found or cannot be cancelled")
	}

	return nil
}
