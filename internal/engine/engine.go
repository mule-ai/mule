package engine

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/mule-ai/mule/internal/agent"
	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/pkg/job"
)

// Engine handles workflow execution
type Engine struct {
	store        primitive.PrimitiveStore
	jobStore     job.JobStore
	agentRuntime *agent.Runtime
	wasmExecutor *WASMExecutor
	workers      int
	jobQueue     chan string
	stopCh       chan struct{}
	wg           sync.WaitGroup
	mu           sync.RWMutex
	running      bool
}

// Config holds engine configuration
type Config struct {
	Workers int
}

// NewEngine creates a new workflow engine
func NewEngine(store primitive.PrimitiveStore, jobStore job.JobStore, agentRuntime *agent.Runtime, wasmExecutor *WASMExecutor, config Config) *Engine {
	return &Engine{
		store:        store,
		jobStore:     jobStore,
		agentRuntime: agentRuntime,
		wasmExecutor: wasmExecutor,
		workers:      config.Workers,
		jobQueue:     make(chan string, 100), // Buffered channel for job IDs
		stopCh:       make(chan struct{}),
	}
}

// Start starts the workflow engine
func (e *Engine) Start(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.running {
		return fmt.Errorf("engine is already running")
	}

	e.running = true
	log.Printf("Starting workflow engine with %d workers", e.workers)

	// Start worker goroutines
	for i := 0; i < e.workers; i++ {
		e.wg.Add(1)
		go e.worker(ctx, i)
	}

	// Start job queue poller
	e.wg.Add(1)
	go e.jobPoller(ctx)

	return nil
}

// Stop stops the workflow engine
func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running {
		return
	}

	log.Println("Stopping workflow engine...")
	e.running = false
	close(e.stopCh)
	e.wg.Wait()
	log.Println("Workflow engine stopped")
}

// SubmitJob submits a new job for execution
func (e *Engine) SubmitJob(ctx context.Context, workflowID string, inputData map[string]interface{}) (*job.Job, error) {
	// Generate job ID
	jobID := uuid.New().String()

	// Create job
	newJob := &job.Job{
		ID:         jobID,
		WorkflowID: workflowID,
		Status:     job.StatusQueued,
		InputData:  inputData,
		OutputData: make(map[string]interface{}),
		CreatedAt:  time.Now(),
	}

	// Save job to database
	if err := e.jobStore.CreateJob(newJob); err != nil {
		return nil, fmt.Errorf("failed to create job: %w", err)
	}

	log.Printf("Submitted job %s for workflow %s", jobID, workflowID)
	return newJob, nil
}

// jobPoller polls for queued jobs and adds them to the processing queue
func (e *Engine) jobPoller(ctx context.Context) {
	defer e.wg.Done()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		case <-ticker.C:
			// Look for queued jobs
			nextJob, err := e.jobStore.GetNextQueuedJob()
			if err != nil {
				log.Printf("Error getting next queued job: %v", err)
				continue
			}

			if nextJob != nil {
				select {
				case e.jobQueue <- nextJob.ID:
					log.Printf("Queued job %s for processing", nextJob.ID)
				default:
					// Queue is full, will try again next iteration
					log.Printf("Job queue is full, skipping job %s", nextJob.ID)
				}
			}
		}
	}
}

// worker processes jobs from the queue
func (e *Engine) worker(ctx context.Context, workerID int) {
	defer e.wg.Done()

	log.Printf("Worker %d started", workerID)

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		case jobID := <-e.jobQueue:
			log.Printf("Worker %d processing job %s", workerID, jobID)
			if err := e.processJob(ctx, jobID); err != nil {
				log.Printf("Worker %d failed to process job %s: %v", workerID, jobID, err)
			}
		}
	}
}

// processJob processes a single job
func (e *Engine) processJob(ctx context.Context, jobID string) error {
	// Mark job as running
	if err := e.jobStore.MarkJobRunning(jobID); err != nil {
		return fmt.Errorf("failed to mark job as running: %w", err)
	}

	// Get job details
	currentJob, err := e.jobStore.GetJob(jobID)
	if err != nil {
		return fmt.Errorf("failed to get job: %w", err)
	}

	// Get workflow details
	workflow, err := e.store.GetWorkflow(ctx, currentJob.WorkflowID)
	if err != nil {
		_ = e.jobStore.MarkJobFailed(jobID, fmt.Errorf("failed to get workflow: %w", err))
		return fmt.Errorf("failed to get workflow: %w", err)
	}

	// Get job timeout setting
	settings, err := e.store.ListSettings(ctx)
	if err != nil {
		log.Printf("Warning: failed to get settings, using default timeout: %v", err)
	}

	var jobTimeoutSeconds int64 = 3600 // Default 1 hour
	for _, setting := range settings {
		if setting.Key == "timeout_job_seconds" {
			if val, parseErr := strconv.ParseInt(setting.Value, 10, 64); parseErr == nil {
				jobTimeoutSeconds = val
			}
			break
		}
	}

	// Create a context with timeout for the job
	jobCtx, cancel := context.WithTimeout(ctx, time.Duration(jobTimeoutSeconds)*time.Second)
	defer cancel()

	// Get workflow steps
	steps, err := e.store.ListWorkflowSteps(ctx, workflow.ID)
	if err != nil {
		_ = e.jobStore.MarkJobFailed(jobID, fmt.Errorf("failed to get workflow steps: %w", err))
		return fmt.Errorf("failed to get workflow steps: %w", err)
	}

	// Process each step
	stepOutput := currentJob.InputData
	for _, step := range steps {
		// Check if job has been cancelled or timed out
		select {
		case <-jobCtx.Done():
			// Context was cancelled (timeout or manual cancellation)
			if jobCtx.Err() == context.DeadlineExceeded {
				_ = e.jobStore.MarkJobFailed(jobID, fmt.Errorf("job timed out after %d seconds", jobTimeoutSeconds))
				return fmt.Errorf("job timed out after %d seconds", jobTimeoutSeconds)
			} else {
				_ = e.jobStore.CancelJob(jobID)
				return fmt.Errorf("job was cancelled")
			}
		default:
		}

		// Check if job status is cancelled in database
		updatedJob, err := e.jobStore.GetJob(jobID)
		if err != nil {
			return fmt.Errorf("failed to get job status: %w", err)
		}
		if updatedJob.Status == job.StatusCancelled {
			return fmt.Errorf("job was cancelled")
		}

		// Create job step record
		jobStep := &job.JobStep{
			ID:             uuid.New().String(),
			JobID:          jobID,
			WorkflowStepID: step.ID,
			StepOrder:      step.StepOrder,
			Status:         "queued",
			InputData:      stepOutput,
		}

		if err := e.jobStore.CreateJobStep(jobStep); err != nil {
			_ = e.jobStore.MarkJobFailed(jobID, fmt.Errorf("failed to create job step: %w", err))
			return fmt.Errorf("failed to create job step: %w", err)
		}

		// Mark step as running
		jobStep.Status = "running"
		if err := e.jobStore.UpdateJobStep(jobStep); err != nil {
			log.Printf("Warning: failed to update job step status to running: %v", err)
		}

		// Process the step
		stepResult, err := e.processStep(jobCtx, step, stepOutput)
		if err != nil {
			jobStep.Status = "failed"
			jobStep.ErrorMessage = err.Error()
			if updateErr := e.jobStore.UpdateJobStep(jobStep); updateErr != nil {
				log.Printf("Warning: failed to update failed job step: %v", updateErr)
			}
			_ = e.jobStore.MarkJobFailed(jobID, fmt.Errorf("step %d failed: %w", step.StepOrder, err))
			return fmt.Errorf("step %d failed: %w", step.StepOrder, err)
		}

		// Mark step as completed
		jobStep.Status = "completed"
		jobStep.OutputData = stepResult
		if err := e.jobStore.UpdateJobStep(jobStep); err != nil {
			log.Printf("Warning: failed to update completed job step: %v", err)
		}

		stepOutput = stepResult
	}

	// Mark job as completed
	if err := e.jobStore.MarkJobCompleted(jobID, stepOutput); err != nil {
		return fmt.Errorf("failed to mark job as completed: %w", err)
	}

	log.Printf("Job %s completed successfully", jobID)
	return nil
}

// processStep processes a single workflow step
func (e *Engine) processStep(ctx context.Context, step *primitive.WorkflowStep, inputData map[string]interface{}) (map[string]interface{}, error) {
	switch step.StepType {
	case "agent":
		return e.processAgentStep(ctx, step, inputData)
	case "wasm_module":
		return e.processWASMStep(ctx, step, inputData)
	default:
		return nil, fmt.Errorf("unknown step type: %s", step.StepType)
	}
}

// processAgentStep processes an agent step
func (e *Engine) processAgentStep(ctx context.Context, step *primitive.WorkflowStep, inputData map[string]interface{}) (map[string]interface{}, error) {
	// Get agent ID from step
	if step.AgentID == nil {
		return nil, fmt.Errorf("agent_id not found in step")
	}

	agentModel, err := e.store.GetAgent(ctx, *step.AgentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}

	// Convert input data to prompt string
	prompt, ok := inputData["prompt"].(string)
	if !ok {
		// If no prompt, try to convert entire input data to string
		prompt = fmt.Sprintf("%v", inputData)
	}

	// Create chat completion request
	req := &agent.ChatCompletionRequest{
		Model: fmt.Sprintf("agent/%s", agentModel.Name),
		Messages: []agent.ChatCompletionMessage{
			{Role: "user", Content: prompt},
		},
		Stream: false,
	}

	// Execute agent
	resp, err := e.agentRuntime.ExecuteAgent(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute agent: %w", err)
	}

	// Return response as prompt for next step
	return map[string]interface{}{
		"prompt": resp.Choices[0].Message.Content,
	}, nil
}

// processWASMStep processes a WASM step
func (e *Engine) processWASMStep(ctx context.Context, step *primitive.WorkflowStep, inputData map[string]interface{}) (map[string]interface{}, error) {
	if e.wasmExecutor == nil {
		return nil, fmt.Errorf("WASM executor not available")
	}

	// Get WASM module ID from step
	if step.WasmModuleID == nil {
		return nil, fmt.Errorf("wasm_module_id not found in step")
	}

	log.Printf("WASM step processing with inputData: %+v", inputData)

	// Execute WASM module
	result, err := e.wasmExecutor.Execute(ctx, *step.WasmModuleID, inputData)
	if err != nil {
		return nil, fmt.Errorf("failed to execute WASM module: %w", err)
	}

	// Extract just the output value from the result
	// The WASM executor returns a map with "output", "stdout", "stderr", etc.
	// We only want the "output" field to pass to the next step
	if output, ok := result["output"]; ok {
		return map[string]interface{}{
			"prompt": output,
		}, nil
	}

	// If no output field, return the whole result (backward compatibility)
	return result, nil
}

// GetWASMExecutor returns the WASM executor instance
func (e *Engine) GetWASMExecutor() *WASMExecutor {
	return e.wasmExecutor
}
