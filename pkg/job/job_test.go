package job

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockJobStore implements JobStore interface for testing
type MockJobStore struct {
	jobs  map[string]Job
	steps map[string][]JobStep
}

func NewMockJobStore() *MockJobStore {
	return &MockJobStore{
		jobs:  make(map[string]Job),
		steps: make(map[string][]JobStep),
	}
}

func (m *MockJobStore) CreateJob(job *Job) error {
	m.jobs[job.ID] = *job
	return nil
}

func (m *MockJobStore) GetJob(id string) (*Job, error) {
	job, exists := m.jobs[id]
	if !exists {
		return nil, ErrJobNotFound
	}
	return &job, nil
}

func (m *MockJobStore) ListJobs() ([]*Job, error) {
	var jobs []*Job
	for _, j := range m.jobs {
		jobCopy := j
		jobs = append(jobs, &jobCopy)
	}
	return jobs, nil
}

func (m *MockJobStore) UpdateJob(job *Job) error {
	if _, exists := m.jobs[job.ID]; !exists {
		return ErrJobNotFound
	}
	m.jobs[job.ID] = *job
	return nil
}

func (m *MockJobStore) DeleteJob(id string) error {
	if _, exists := m.jobs[id]; !exists {
		return ErrJobNotFound
	}
	delete(m.jobs, id)
	return nil
}

func (m *MockJobStore) CreateJobStep(step *JobStep) error {
	if _, exists := m.steps[step.JobID]; !exists {
		m.steps[step.JobID] = []JobStep{}
	}
	m.steps[step.JobID] = append(m.steps[step.JobID], *step)
	return nil
}

func (m *MockJobStore) GetJobStep(id string) (*JobStep, error) {
	for _, steps := range m.steps {
		for _, step := range steps {
			if step.ID == id {
				stepCopy := step
				return &stepCopy, nil
			}
		}
	}
	return nil, ErrJobStepNotFound
}

func (m *MockJobStore) GetJobSteps(jobID string) ([]*JobStep, error) {
	steps, exists := m.steps[jobID]
	if !exists {
		return []*JobStep{}, nil
	}
	var result []*JobStep
	for _, step := range steps {
		stepCopy := step
		result = append(result, &stepCopy)
	}
	return result, nil
}

func (m *MockJobStore) UpdateJobStep(step *JobStep) error {
	steps, exists := m.steps[step.JobID]
	if !exists {
		return ErrJobStepNotFound
	}

	for i, s := range steps {
		if s.ID == step.ID {
			steps[i] = *step
			m.steps[step.JobID] = steps
			return nil
		}
	}
	return ErrJobStepNotFound
}

func (m *MockJobStore) GetNextQueuedJob() (*Job, error) {
	for _, job := range m.jobs {
		if job.Status == StatusQueued {
			jobCopy := job
			return &jobCopy, nil
		}
	}
	return nil, ErrJobNotFound
}

func (m *MockJobStore) MarkJobRunning(jobID string) error {
	if job, exists := m.jobs[jobID]; exists {
		job.Status = StatusRunning
		job.StartedAt = &[]time.Time{time.Now()}[0]
		m.jobs[jobID] = job
		return nil
	}
	return ErrJobNotFound
}

func (m *MockJobStore) MarkJobCompleted(jobID string, outputData map[string]interface{}) error {
	if job, exists := m.jobs[jobID]; exists {
		job.Status = StatusCompleted
		job.OutputData = outputData
		now := time.Now()
		job.CompletedAt = &now
		m.jobs[jobID] = job
		return nil
	}
	return ErrJobNotFound
}

func (m *MockJobStore) MarkJobFailed(jobID string, err error) error {
	if job, exists := m.jobs[jobID]; exists {
		job.Status = StatusFailed
		job.OutputData = map[string]interface{}{"error": err.Error()}
		now := time.Now()
		job.CompletedAt = &now
		m.jobs[jobID] = job
		return nil
	}
	return ErrJobNotFound
}

func (m *MockJobStore) CancelJob(jobID string) error {
	if job, exists := m.jobs[jobID]; exists {
		// Only allow cancelling queued or running jobs
		if job.Status == StatusQueued || job.Status == StatusRunning {
			job.Status = StatusCancelled
			now := time.Now()
			job.CompletedAt = &now
			m.jobs[jobID] = job
			return nil
		}
		return ErrJobNotFound
	}
	return ErrJobNotFound
}

func TestJobStatus(t *testing.T) {
	tests := []struct {
		status   Status
		expected string
	}{
		{StatusQueued, "queued"},
		{StatusRunning, "running"},
		{StatusCompleted, "completed"},
		{StatusFailed, "failed"},
		{StatusCancelled, "cancelled"},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.status.String())
	}
}

func TestJob(t *testing.T) {
	job := &Job{
		ID:          "test-job",
		WorkflowID:  "test-workflow",
		Status:      StatusQueued,
		InputData:   map[string]interface{}{"message": "hello"},
		OutputData:  map[string]interface{}{},
		StartedAt:   nil,
		CompletedAt: nil,
		CreatedAt:   time.Now(),
	}

	assert.Equal(t, "test-job", job.ID)
	assert.Equal(t, "test-workflow", job.WorkflowID)
	assert.Equal(t, StatusQueued, job.Status)
	assert.Equal(t, "hello", job.InputData["message"])
	assert.Nil(t, job.StartedAt)
	assert.Nil(t, job.CompletedAt)
}

func TestJobStep(t *testing.T) {
	step := &JobStep{
		ID:             "test-step",
		JobID:          "test-job",
		WorkflowStepID: "step1",
		Status:         StatusQueued,
		InputData:      map[string]interface{}{"data": "test"},
		OutputData:     map[string]interface{}{},
		StartedAt:      nil,
		CompletedAt:    nil,
	}

	assert.Equal(t, "test-step", step.ID)
	assert.Equal(t, "test-job", step.JobID)
	assert.Equal(t, "step1", step.WorkflowStepID)
	assert.Equal(t, StatusQueued, step.Status)
	assert.Equal(t, "test", step.InputData["data"])
	assert.Nil(t, step.StartedAt)
	assert.Nil(t, step.CompletedAt)
}

func TestMockJobStore(t *testing.T) {
	store := NewMockJobStore()

	// Test Job
	job := &Job{
		ID:         "test-job",
		WorkflowID: "test-workflow",
		Status:     StatusQueued,
		InputData:  map[string]interface{}{"message": "hello"},
		CreatedAt:  time.Now(),
	}

	err := store.CreateJob(job)
	require.NoError(t, err)

	retrieved, err := store.GetJob("test-job")
	require.NoError(t, err)
	assert.Equal(t, job.WorkflowID, retrieved.WorkflowID)
	assert.Equal(t, StatusQueued, retrieved.Status)

	jobs, err := store.ListJobs()
	require.NoError(t, err)
	assert.Len(t, jobs, 1)

	job.Status = StatusRunning
	err = store.UpdateJob(job)
	require.NoError(t, err)

	updated, err := store.GetJob("test-job")
	require.NoError(t, err)
	assert.Equal(t, StatusRunning, updated.Status)

	// Test JobStep
	step := &JobStep{
		ID:             "test-step",
		JobID:          "test-job",
		WorkflowStepID: "step1",
		Status:         StatusQueued,
		InputData:      map[string]interface{}{"data": "test"},
	}

	err = store.CreateJobStep(step)
	require.NoError(t, err)

	steps, err := store.GetJobSteps("test-job")
	require.NoError(t, err)
	assert.Len(t, steps, 1)
	assert.Equal(t, "test-step", steps[0].ID)

	step.Status = StatusRunning
	err = store.UpdateJobStep(step)
	require.NoError(t, err)

	updatedSteps, err := store.GetJobSteps("test-job")
	require.NoError(t, err)
	assert.Len(t, updatedSteps, 1)
	assert.Equal(t, StatusRunning, updatedSteps[0].Status)
}

func TestJobTransitions(t *testing.T) {
	job := &Job{
		ID:         "test-job",
		WorkflowID: "test-workflow",
		Status:     StatusQueued,
	}

	// Test valid transitions
	assert.True(t, job.Status.CanTransitionTo(StatusRunning))
	assert.True(t, job.Status.CanTransitionTo(StatusFailed))
	assert.True(t, job.Status.CanTransitionTo(StatusCancelled))

	// Test invalid transitions
	assert.False(t, job.Status.CanTransitionTo(StatusCompleted))

	// Test running state transitions
	job.Status = StatusRunning
	assert.True(t, job.Status.CanTransitionTo(StatusCompleted))
	assert.True(t, job.Status.CanTransitionTo(StatusFailed))
	assert.True(t, job.Status.CanTransitionTo(StatusCancelled))
	assert.False(t, job.Status.CanTransitionTo(StatusQueued))

	// Test completed state transitions
	job.Status = StatusCompleted
	assert.False(t, job.Status.CanTransitionTo(StatusRunning))
	assert.False(t, job.Status.CanTransitionTo(StatusQueued))
	assert.False(t, job.Status.CanTransitionTo(StatusCancelled))

	// Test cancelled state transitions
	job.Status = StatusCancelled
	assert.False(t, job.Status.CanTransitionTo(StatusRunning))
	assert.False(t, job.Status.CanTransitionTo(StatusQueued))
	assert.False(t, job.Status.CanTransitionTo(StatusCompleted))
	assert.False(t, job.Status.CanTransitionTo(StatusFailed))
}

func TestCancelJob(t *testing.T) {
	store := NewMockJobStore()

	// Test cancelling a queued job
	job := &Job{
		ID:         "test-job-1",
		WorkflowID: "test-workflow",
		Status:     StatusQueued,
		CreatedAt:  time.Now(),
	}

	err := store.CreateJob(job)
	require.NoError(t, err)

	err = store.CancelJob("test-job-1")
	require.NoError(t, err)

	updated, err := store.GetJob("test-job-1")
	require.NoError(t, err)
	assert.Equal(t, StatusCancelled, updated.Status)
	assert.NotNil(t, updated.CompletedAt)

	// Test cancelling a running job
	job2 := &Job{
		ID:         "test-job-2",
		WorkflowID: "test-workflow",
		Status:     StatusRunning,
		CreatedAt:  time.Now(),
	}

	err = store.CreateJob(job2)
	require.NoError(t, err)

	err = store.CancelJob("test-job-2")
	require.NoError(t, err)

	updated2, err := store.GetJob("test-job-2")
	require.NoError(t, err)
	assert.Equal(t, StatusCancelled, updated2.Status)
	assert.NotNil(t, updated2.CompletedAt)

	// Test cancelling a completed job (should fail)
	job3 := &Job{
		ID:         "test-job-3",
		WorkflowID: "test-workflow",
		Status:     StatusCompleted,
		CreatedAt:  time.Now(),
	}

	err = store.CreateJob(job3)
	require.NoError(t, err)

	err = store.CancelJob("test-job-3")
	assert.Error(t, err)
	assert.Equal(t, ErrJobNotFound, err)

	// Test cancelling a non-existent job
	err = store.CancelJob("non-existent-job")
	assert.Error(t, err)
	assert.Equal(t, ErrJobNotFound, err)
}
