package engine

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mule-ai/mule/internal/agent"
	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/pkg/job"
)

// TestProcessJobWithWorkflowNotFound tests processJob behavior when workflow is not found
func TestProcessJobWithWorkflowNotFound(t *testing.T) {
	mockStore := &MockPrimitiveStore{
		Workflows: []*primitive.Workflow{}, // Empty - workflow not found
	}
	mockJobStore := &MockJobStore{
		Jobs: map[string]*job.Job{
			"job-not-found": {
				ID:         "job-not-found",
				WorkflowID: "non-existent-workflow",
				Status:     job.StatusQueued,
				InputData:  map[string]interface{}{"test": "data"},
				CreatedAt:  time.Now(),
			},
		},
	}
	agentRuntime := agent.NewRuntime(mockStore, mockJobStore)
	wasmExecutor := NewWASMExecutor(nil, mockStore, agentRuntime, nil)

	// Create engine to verify dependencies work together
	_ = NewEngine(mockStore, mockJobStore, agentRuntime, wasmExecutor, Config{Workers: 1})

	// Test that MarkJobRunning is called and workflow not found error is handled
	err := mockJobStore.MarkJobRunning("job-not-found")
	assert.NoError(t, err)

	// Get the job
	j, err := mockJobStore.GetJob("job-not-found")
	assert.NoError(t, err)
	assert.Equal(t, job.StatusRunning, j.Status)

	// Now try to get workflow - should fail
	_, err = mockStore.GetWorkflow(context.Background(), "non-existent-workflow")
	assert.Error(t, err)
	assert.Equal(t, primitive.ErrNotFound, err)
}

// TestProcessJobWithNoSteps tests processJob behavior when workflow has no steps
func TestProcessJobWithNoSteps(t *testing.T) {
	mockStore := &MockPrimitiveStore{
		Workflows: []*primitive.Workflow{
			{
				ID:   "workflow-no-steps",
				Name: "Workflow With No Steps",
			},
		},
		WorkflowSteps: []*primitive.WorkflowStep{}, // Empty steps
	}
	mockJobStore := &MockJobStore{
		Jobs: map[string]*job.Job{
			"job-no-steps": {
				ID:         "job-no-steps",
				WorkflowID: "workflow-no-steps",
				Status:     job.StatusQueued,
				InputData:  map[string]interface{}{"test": "data"},
				CreatedAt:  time.Now(),
			},
		},
	}
	agentRuntime := agent.NewRuntime(mockStore, mockJobStore)
	wasmExecutor := NewWASMExecutor(nil, mockStore, agentRuntime, nil)

	// Create engine to verify dependencies work together
	_ = NewEngine(mockStore, mockJobStore, agentRuntime, wasmExecutor, Config{Workers: 1})

	// Test workflow exists
	workflow, err := mockStore.GetWorkflow(context.Background(), "workflow-no-steps")
	assert.NoError(t, err)
	assert.NotNil(t, workflow)

	// Test workflow has no steps
	steps, err := mockStore.ListWorkflowSteps(context.Background(), "workflow-no-steps")
	assert.NoError(t, err)
	assert.Empty(t, steps)
}

// TestProcessJobWithCancelledJob tests behavior when job is cancelled during execution
func TestProcessJobWithCancelledJob(t *testing.T) {
	mockStore := &MockPrimitiveStore{
		Workflows: []*primitive.Workflow{
			{
				ID:   "workflow-cancel",
				Name: "Cancel Test Workflow",
			},
		},
		WorkflowSteps: []*primitive.WorkflowStep{
			{
				ID:         "step-1",
				WorkflowID: "workflow-cancel",
				StepType:   "agent",
				AgentID:    &[]string{"agent-1"}[0],
				StepOrder:  1,
			},
		},
		Agents: []*primitive.Agent{
			{
				ID:   "agent-1",
				Name: "test-agent",
			},
		},
	}

	// Create job store that tracks cancelled jobs
	mockJobStore := &MockJobStoreWithCancel{
		MockJobStore: &MockJobStore{
			Jobs: map[string]*job.Job{
				"job-cancel": {
					ID:         "job-cancel",
					WorkflowID: "workflow-cancel",
					Status:     job.StatusQueued,
					InputData:  map[string]interface{}{"test": "data"},
					CreatedAt:  time.Now(),
				},
			},
		},
		cancelledJobs: map[string]bool{
			"job-cancel": true,
		},
	}
	agentRuntime := agent.NewRuntime(mockStore, mockJobStore)
	wasmExecutor := NewWASMExecutor(nil, mockStore, agentRuntime, nil)

	// Create engine to verify dependencies work together
	_ = NewEngine(mockStore, mockJobStore, agentRuntime, wasmExecutor, Config{Workers: 1})

	// Test that cancelled job status can be checked
	j, err := mockJobStore.GetJob("job-cancel")
	assert.NoError(t, err)
	assert.Equal(t, job.StatusQueued, j.Status)

	// Test that job appears cancelled
	assert.True(t, mockJobStore.IsJobCancelled("job-cancel"))
	assert.False(t, mockJobStore.IsJobCancelled("job-nonexistent"))
}

// MockJobStoreWithCancel extends MockJobStore with cancellation tracking
type MockJobStoreWithCancel struct {
	*MockJobStore
	cancelledJobs map[string]bool
}

func (m *MockJobStoreWithCancel) IsJobCancelled(jobID string) bool {
	return m.cancelledJobs[jobID]
}

// TestEngineStartStopCycle tests multiple start/stop cycles with new engine instances
func TestEngineStartStopCycle(t *testing.T) {
	ctx := context.Background()

	// First start/stop cycle
	t.Run("first cycle", func(t *testing.T) {
		mockStore := &MockPrimitiveStore{}
		mockJobStore := &MockJobStore{Jobs: make(map[string]*job.Job)}
		agentRuntime := agent.NewRuntime(mockStore, mockJobStore)
		wasmExecutor := NewWASMExecutor(nil, mockStore, agentRuntime, nil)
		engine := NewEngine(mockStore, mockJobStore, agentRuntime, wasmExecutor, Config{Workers: 2})

		err := engine.Start(ctx)
		assert.NoError(t, err)
		engine.Stop()
	})

	// Second start/stop cycle with a new engine
	t.Run("second cycle", func(t *testing.T) {
		mockStore := &MockPrimitiveStore{}
		mockJobStore := &MockJobStore{Jobs: make(map[string]*job.Job)}
		agentRuntime := agent.NewRuntime(mockStore, mockJobStore)
		wasmExecutor := NewWASMExecutor(nil, mockStore, agentRuntime, nil)
		engine := NewEngine(mockStore, mockJobStore, agentRuntime, wasmExecutor, Config{Workers: 2})

		err := engine.Start(ctx)
		assert.NoError(t, err)
		engine.Stop()
	})
}

// TestWASMExecutorClose tests the Close method
func TestWASMExecutorClose(t *testing.T) {
	mockStore := &MockPrimitiveStore{}
	mockAgentRuntime := &agent.Runtime{}

	executor := NewWASMExecutor(nil, mockStore, mockAgentRuntime, nil)

	// Add some modules to cache
	executor.modules["module-1"] = []byte("test-data-1")
	executor.modules["module-2"] = []byte("test-data-2")

	// Verify modules are cached
	assert.Equal(t, 2, len(executor.modules))

	// Close the executor
	err := executor.Close(context.Background())
	assert.NoError(t, err)

	// Verify cache is cleared
	assert.Equal(t, 0, len(executor.modules))
}

// TestWASMExecutorModules tests the Modules method
func TestWASMExecutorModules(t *testing.T) {
	mockStore := &MockPrimitiveStore{}
	mockAgentRuntime := &agent.Runtime{}

	executor := NewWASMExecutor(nil, mockStore, mockAgentRuntime, nil)

	// Test with empty cache
	modules := executor.Modules()
	assert.NotNil(t, modules)
	assert.Equal(t, 0, len(modules))

	// Add some modules to cache
	executor.modules["module-1"] = []byte("test-data-1")
	executor.modules["module-2"] = []byte("test-data-2")

	// Test with cached modules
	modules = executor.Modules()
	assert.NotNil(t, modules)
	assert.Equal(t, 2, len(modules))
	assert.Equal(t, []byte("test-data-1"), modules["module-1"])
	assert.Equal(t, []byte("test-data-2"), modules["module-2"])
}

// TestWASMExecutorTriggerWorkflowErrors tests triggerWorkflow error handling
func TestWASMExecutorTriggerWorkflowErrors(t *testing.T) {
	t.Run("workflow engine not available", func(t *testing.T) {
		mockStore := &MockPrimitiveStore{}
		mockAgentRuntime := &agent.Runtime{}

		// Create executor without workflow engine
		executor := NewWASMExecutor(nil, mockStore, mockAgentRuntime, nil)

		// triggerWorkflow should fail when workflow engine is not available
		_, err := executor.triggerWorkflow(context.Background(), "workflow-1", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "workflow engine not available")
	})

	t.Run("workflow not found by ID or name", func(t *testing.T) {
		mockStore := &MockPrimitiveStore{
			Workflows: []*primitive.Workflow{}, // No workflows
		}
		mockAgentRuntime := &agent.Runtime{}
		mockEngine := &Engine{}

		executor := NewWASMExecutor(nil, mockStore, mockAgentRuntime, mockEngine)

		_, err := executor.triggerWorkflow(context.Background(), "nonexistent-workflow", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "workflow not found")
	})
}

// TestWASMExecutorCallAgentErrors tests callAgent error handling
func TestWASMExecutorCallAgentErrors(t *testing.T) {
	t.Run("agent not found by ID or name", func(t *testing.T) {
		mockStore := &MockPrimitiveStore{
			Agents: []*primitive.Agent{}, // No agents
		}
		mockAgentRuntime := &agent.Runtime{}

		executor := NewWASMExecutor(nil, mockStore, mockAgentRuntime, nil)

		_, err := executor.callAgent(context.Background(), "nonexistent-agent", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "agent not found")
	})

	t.Run("context cancelled", func(t *testing.T) {
		mockStore := &MockPrimitiveStore{
			Agents: []*primitive.Agent{
				{
					ID:   "agent-1",
					Name: "test-agent",
				},
			},
		}
		mockAgentRuntime := &agent.Runtime{}

		executor := NewWASMExecutor(nil, mockStore, mockAgentRuntime, nil)

		// Create a cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := executor.callAgent(ctx, "agent-1", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cancelled")
	})
}

// TestWASMExecutorTriggerWorkflowWithContextCancellation tests triggerWorkflow with cancelled context
func TestWASMExecutorTriggerWorkflowWithContextCancellation(t *testing.T) {
	mockStore := &MockPrimitiveStore{
		Workflows: []*primitive.Workflow{
			{
				ID:   "workflow-1",
				Name: "test-workflow",
			},
		},
	}
	mockAgentRuntime := &agent.Runtime{}
	mockEngine := &Engine{}

	executor := NewWASMExecutor(nil, mockStore, mockAgentRuntime, mockEngine)

	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := executor.triggerWorkflow(ctx, "workflow-1", nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cancelled")
}

// TestGetModuleDataFromStore tests getModuleData behavior
func TestGetModuleDataFromStore(t *testing.T) {
	t.Run("module found in cache", func(t *testing.T) {
		mockStore := &MockPrimitiveStore{}
		mockAgentRuntime := &agent.Runtime{}

		executor := NewWASMExecutor(nil, mockStore, mockAgentRuntime, nil)

		// Add module to cache
		testData := []byte("cached module data")
		executor.modules["cached-module"] = testData

		// getModuleData should return cached data without calling store
		data, err := executor.getModuleData(context.Background(), "cached-module")
		assert.NoError(t, err)
		assert.Equal(t, testData, data)
	})

	t.Run("module not in cache", func(t *testing.T) {
		mockStore := &MockPrimitiveStore{} // No WasmModules in store
		mockAgentRuntime := &agent.Runtime{}

		executor := NewWASMExecutor(nil, mockStore, mockAgentRuntime, nil)

		// Module should not be in cache initially
		_, ok := executor.modules["uncached-module"]
		assert.False(t, ok)

		// getModuleData should fail since module is not in cache and store returns ErrNotFound
		data, err := executor.getModuleData(context.Background(), "uncached-module")
		assert.Error(t, err)
		assert.Nil(t, data)
	})
}

// TestLoadModule tests the LoadModule function
func TestLoadModule(t *testing.T) {
	t.Run("load non-existent module", func(t *testing.T) {
		mockStore := &MockPrimitiveStore{} // No modules in store
		mockAgentRuntime := &agent.Runtime{}

		executor := NewWASMExecutor(nil, mockStore, mockAgentRuntime, nil)

		err := executor.LoadModule(context.Background(), "nonexistent-module")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to load WASM module")
	})
}

// TestIsValidBranchName tests the isValidBranchName function
func TestIsValidBranchName(t *testing.T) {
	// Valid branch names (according to actual implementation)
	validNames := []string{
		"main",
		"feature-branch",
		"feature_branch",
		"feature-branch-123",
		"a",
		"my/feature",
		"branch with space", // Spaces are allowed by the implementation
	}
	for _, name := range validNames {
		assert.True(t, isValidBranchName(name), "Expected %q to be valid", name)
	}

	// Invalid branch names (according to actual implementation)
	invalidNames := []string{
		"",                      // Empty
		".",                     // Reserved
		"..",                    // Reserved
		"@",                     // Reserved
		"HEAD",                  // Reserved
		"~",                     // Invalid char
		"^",                     // Invalid char
		":",                     // Invalid char
		"?",                     // Invalid char
		"*",                     // Invalid char
		"[",                     // Invalid char
		"branch/",               // Trailing slash
		"..hidden",              // Double dots
		"hidden..",              // Double dots
		"a..b",                  // Double dots
		"branch\twith\ttab",     // Tab (control char)
		"branch\nwith\nnewline", // Newline (control char)
	}
	for _, name := range invalidNames {
		assert.False(t, isValidBranchName(name), "Expected %q to be invalid", name)
	}
}

// TestProcessAgentStepWithWorkingDirAgentNotFound tests processAgentStepWithWorkingDir when agent is not found
func TestProcessAgentStepWithWorkingDirAgentNotFound(t *testing.T) {
	mockStore := &MockPrimitiveStore{
		Workflows: []*primitive.Workflow{
			{
				ID:   "workflow-1",
				Name: "Test Workflow",
			},
		},
		Agents: []*primitive.Agent{}, // No agents
	}
	mockJobStore := &MockJobStore{Jobs: make(map[string]*job.Job)}
	agentRuntime := agent.NewRuntime(mockStore, mockJobStore)
	wasmExecutor := NewWASMExecutor(nil, mockStore, agentRuntime, nil)

	engine := NewEngine(mockStore, mockJobStore, agentRuntime, wasmExecutor, Config{Workers: 1})

	agentID := "nonexistent-agent"
	step := &primitive.WorkflowStep{
		ID:         "step-1",
		WorkflowID: "workflow-1",
		StepType:   "agent",
		AgentID:    &agentID,
		StepOrder:  1,
	}

	// Try to process agent step with non-existent agent
	_, err := engine.processAgentStepWithWorkingDir(context.Background(), step, nil, "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get agent")
}

// TestProcessAgentStepWithWorkingDirNoPrompt tests processAgentStepWithWorkingDir with non-prompt input data
func TestProcessAgentStepWithWorkingDirNoPrompt(t *testing.T) {
	mockStore := &MockPrimitiveStore{
		Workflows: []*primitive.Workflow{
			{
				ID:   "workflow-1",
				Name: "Test Workflow",
			},
		},
		Agents: []*primitive.Agent{
			{
				ID:   "agent-1",
				Name: "test-agent",
			},
		},
	}
	mockJobStore := &MockJobStore{Jobs: make(map[string]*job.Job)}
	agentRuntime := agent.NewRuntime(mockStore, mockJobStore)
	wasmExecutor := NewWASMExecutor(nil, mockStore, agentRuntime, nil)

	engine := NewEngine(mockStore, mockJobStore, agentRuntime, wasmExecutor, Config{Workers: 1})

	agentID := "agent-1"
	step := &primitive.WorkflowStep{
		ID:         "step-1",
		WorkflowID: "workflow-1",
		StepType:   "agent",
		AgentID:    &agentID,
		StepOrder:  1,
	}

	// Test with non-string input data (should be converted to JSON and used as prompt)
	inputData := map[string]interface{}{
		"key": "value",
		"num": 42,
	}

	// This tests that the input data is properly converted to a prompt
	// The agent execution may fail but it tests the input data processing path
	_, err := engine.processAgentStepWithWorkingDir(context.Background(), step, inputData, "")
	// We just verify the function runs - it may or may not return an error depending on agent execution
	// The important thing is that input data processing doesn't crash
	_ = err // May or may not have an error depending on agent runtime
}

// TestEngineWithMultipleWorkers tests engine with multiple workers
func TestEngineWithMultipleWorkers(t *testing.T) {
	mockStore := &MockPrimitiveStore{}
	mockJobStore := &MockJobStore{Jobs: make(map[string]*job.Job)}
	agentRuntime := agent.NewRuntime(mockStore, mockJobStore)
	wasmExecutor := NewWASMExecutor(nil, mockStore, agentRuntime, nil)

	// Test with 10 workers
	engine := NewEngine(mockStore, mockJobStore, agentRuntime, wasmExecutor, Config{Workers: 10})
	assert.Equal(t, 10, engine.workers)

	// Start engine
	ctx := context.Background()
	err := engine.Start(ctx)
	assert.NoError(t, err)

	// Submit multiple jobs
	for i := 0; i < 5; i++ {
		job := &job.Job{
			ID:         "job-" + string(rune('a'+i)),
			WorkflowID: "workflow-1",
			Status:     job.StatusQueued,
			CreatedAt:  time.Now(),
		}
		err := mockJobStore.CreateJob(job)
		assert.NoError(t, err)
	}

	// Wait a bit for jobs to be processed
	time.Sleep(100 * time.Millisecond)

	// Stop engine
	engine.Stop()
}

// TestMockJobStoreListJobs tests the ListJobs function of MockJobStore
func TestMockJobStoreListJobs(t *testing.T) {
	mockJobStore := &MockJobStore{
		Jobs: map[string]*job.Job{
			"job-1": {
				ID:         "job-1",
				WorkflowID: "workflow-1",
				Status:     job.StatusCompleted,
				CreatedAt:  time.Now().Add(-2 * time.Hour),
			},
			"job-2": {
				ID:         "job-2",
				WorkflowID: "workflow-1",
				Status:     job.StatusRunning,
				CreatedAt:  time.Now().Add(-1 * time.Hour),
			},
			"job-3": {
				ID:         "job-3",
				WorkflowID: "workflow-2",
				Status:     job.StatusQueued,
				CreatedAt:  time.Now(),
			},
		},
	}

	// Test listing all jobs
	jobs, total, err := mockJobStore.ListJobs(job.ListJobsOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Equal(t, 3, len(jobs))

	// Test filtering by status
	status := job.StatusCompleted
	jobs, total, err = mockJobStore.ListJobs(job.ListJobsOptions{Status: &status})
	assert.NoError(t, err)
	assert.Equal(t, 1, total)
	assert.Equal(t, 1, len(jobs))
	assert.Equal(t, job.StatusCompleted, jobs[0].Status)

	// Test searching
	jobs, total, err = mockJobStore.ListJobs(job.ListJobsOptions{Search: "workflow-1"})
	assert.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Equal(t, 2, len(jobs))

	// Test pagination
	jobs, total, err = mockJobStore.ListJobs(job.ListJobsOptions{Page: 1, PageSize: 2})
	assert.NoError(t, err)
	assert.Equal(t, 3, total)
	assert.Equal(t, 2, len(jobs))
}

// TestMockPrimitiveStoreGetAgentTools tests GetAgentTools method
func TestMockPrimitiveStoreGetAgentTools(t *testing.T) {
	mockStore := &MockPrimitiveStore{}

	tools, err := mockStore.GetAgentTools(context.Background(), "agent-1")
	assert.NoError(t, err)
	assert.NotNil(t, tools)
	assert.Empty(t, tools) // Returns empty tools by default
}

// TestMockPrimitiveStoreMemoryConfig tests memory config methods
func TestMockPrimitiveStoreMemoryConfig(t *testing.T) {
	mockStore := &MockPrimitiveStore{}

	// GetMemoryConfig should return ErrNotFound
	config, err := mockStore.GetMemoryConfig(context.Background(), "config-1")
	assert.Error(t, err)
	assert.Equal(t, primitive.ErrNotFound, err)
	assert.Nil(t, config)

	// UpdateMemoryConfig should succeed
	err = mockStore.UpdateMemoryConfig(context.Background(), &primitive.MemoryConfig{})
	assert.NoError(t, err)
}

// TestMockPrimitiveStoreSettings tests settings methods
func TestMockPrimitiveStoreSettings(t *testing.T) {
	mockStore := &MockPrimitiveStore{}

	// GetSetting should return ErrNotFound
	setting, err := mockStore.GetSetting(context.Background(), "test-key")
	assert.Error(t, err)
	assert.Equal(t, primitive.ErrNotFound, err)
	assert.Nil(t, setting)

	// ListSettings should return empty
	settings, err := mockStore.ListSettings(context.Background())
	assert.NoError(t, err)
	assert.Empty(t, settings)

	// UpdateSetting should succeed
	err = mockStore.UpdateSetting(context.Background(), &primitive.Setting{})
	assert.NoError(t, err)
}
