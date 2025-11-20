package job

import (
	"errors"
	"time"
)

// Errors
var (
	ErrJobNotFound     = errors.New("job not found")
	ErrJobStepNotFound = errors.New("job step not found")
)

// Status represents job status
type Status string

const (
	StatusQueued    Status = "queued"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

// String returns string representation of status
func (s Status) String() string {
	return string(s)
}

// CanTransitionTo checks if status can transition to target status
func (s Status) CanTransitionTo(target Status) bool {
	switch s {
	case StatusQueued:
		return target == StatusRunning || target == StatusFailed
	case StatusRunning:
		return target == StatusCompleted || target == StatusFailed
	case StatusCompleted, StatusFailed:
		return false // Terminal states
	default:
		return false
	}
}

// Job represents a workflow execution job
type Job struct {
	ID          string                 `json:"id" db:"id"`
	WorkflowID  string                 `json:"workflow_id" db:"workflow_id"`
	Status      Status                 `json:"status" db:"status"`
	InputData   map[string]interface{} `json:"input_data" db:"input_data"`
	OutputData  map[string]interface{} `json:"output_data" db:"output_data"`
	CreatedAt   time.Time              `json:"created_at" db:"created_at"`
	StartedAt   *time.Time             `json:"started_at,omitempty" db:"started_at"`
	CompletedAt *time.Time             `json:"completed_at,omitempty" db:"completed_at"`
}

// JobStep represents the execution of a single step within a job
type JobStep struct {
	ID             string                 `json:"id" db:"id"`
	JobID          string                 `json:"job_id" db:"job_id"`
	WorkflowStepID string                 `json:"workflow_step_id" db:"workflow_step_id"`
	Status         Status                 `json:"status" db:"status"`
	InputData      map[string]interface{} `json:"input_data" db:"input_data"`
	OutputData     map[string]interface{} `json:"output_data" db:"output_data"`
	StartedAt      *time.Time             `json:"started_at,omitempty" db:"started_at"`
	CompletedAt    *time.Time             `json:"completed_at,omitempty" db:"completed_at"`
}

// JobStore defines interface for job persistence
type JobStore interface {
	CreateJob(job *Job) error
	GetJob(id string) (*Job, error)
	ListJobs() ([]*Job, error)
	UpdateJob(job *Job) error
	DeleteJob(id string) error

	CreateJobStep(step *JobStep) error
	GetJobStep(id string) (*JobStep, error)
	ListJobSteps(jobID string) ([]*JobStep, error)
	UpdateJobStep(step *JobStep) error
	DeleteJobStep(id string) error

	GetNextQueuedJob() (*Job, error)
	MarkJobRunning(jobID string) error
	MarkJobCompleted(jobID string, outputData map[string]interface{}) error
	MarkJobFailed(jobID string, err error) error
}
