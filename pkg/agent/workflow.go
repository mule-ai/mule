package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/mule-ai/mule/pkg/types"
	"github.com/mule-ai/mule/pkg/validation"
)

const numValidationAttempts = 20

type Workflow struct {
	settings WorkflowSettings
	// TODO: Remove and refactor
	Steps               []WorkflowStep
	ValidationFunctions []string
	triggerChannel      chan any
	outputChannels      []chan any
	agentMap            map[int]*Agent
	logger              logr.Logger
	integrations        map[string]types.Integration
	workflowMap         map[string]*Workflow // Map of workflow ID/name to workflow instances
}

type WorkflowSettings struct {
	ID                  string                  `json:"id"`
	Name                string                  `json:"name"`
	Description         string                  `json:"description"`
	IsDefault           bool                    `json:"isDefault"`
	Outputs             []types.TriggerSettings `json:"outputs"`
	Steps               []WorkflowStep          `json:"steps"`
	Triggers            []types.TriggerSettings `json:"triggers"`
	ValidationFunctions []string                `json:"validationFunctions"`
}

// WorkflowStep represents a step in a workflow
type WorkflowStep struct {
	ID          string                `json:"id"`
	AgentID     int                   `json:"agentID,omitempty"`
	AgentName   string                `json:"agentName,omitempty"`
	OutputField string                `json:"outputField"`
	Integration types.TriggerSettings `json:"integration,omitempty"`
	WorkflowID  string                `json:"workflowID,omitempty"`  // Reference to sub-workflow
	RetryConfig *RetryConfig          `json:"retryConfig,omitempty"` // Retry configuration for sub-workflows
}

// RetryConfig defines retry behavior for workflow steps
type RetryConfig struct {
	MaxAttempts int `json:"maxAttempts"`
	DelayMs     int `json:"delayMs"`
}

// WorkflowResult represents the result of a workflow step execution
type WorkflowResult struct {
	AgentID     int
	OutputField string
	Integration string
	Content     string
	StepID      string
	Error       error
}

// WorkflowContext holds the state of a workflow execution
type WorkflowContext struct {
	Results             map[string]WorkflowResult // Map of step ID to result
	CurrentInput        PromptInput
	Path                string
	Logger              logr.Logger
	ValidationFunctions []string
}

func NewWorkflow(settings WorkflowSettings, agentMap map[int]*Agent, integrations map[string]types.Integration, logger logr.Logger) *Workflow {
	w := &Workflow{
		settings:            settings,
		Steps:               settings.Steps,
		ValidationFunctions: settings.ValidationFunctions,
		triggerChannel:      make(chan any, 100), // Buffered channel to prevent blocking
		outputChannels:      make([]chan any, len(settings.Outputs)),
		agentMap:            agentMap,
		logger:              logger,
		integrations:        integrations,
		workflowMap:         make(map[string]*Workflow),
	}

	for _, step := range w.Steps {
		if step.Integration.Integration == "" && step.AgentID == 0 && step.WorkflowID == "" {
			w.logger.Error(fmt.Errorf("step %s has no integration, agentID, or workflowID", step.ID), "Step has no integration, agentID, or workflowID")
			panic(fmt.Errorf("step %s has no integration, agentID, or workflowID", step.ID))
		}
	}

	go func() {
		for data := range w.triggerChannel {
			var dataString string
			switch v := data.(type) {
			case string:
				dataString = v
			case *types.TriggerSettings:
				// Handle TriggerSettings by converting the Data field to a string
				if v.Data != nil {
					// Try to convert to JSON string first
					if jsonData, err := json.Marshal(v.Data); err == nil {
						dataString = string(jsonData)
					} else {
						// Fallback to fmt.Sprintf
						dataString = fmt.Sprintf("%v", v.Data)
					}
				} else {
					dataString = ""
				}
			default:
				// Try to convert to JSON string first
				if jsonData, err := json.Marshal(v); err == nil {
					dataString = string(jsonData)
				} else {
					// Fallback to fmt.Sprintf
					dataString = fmt.Sprintf("%v", v)
				}
			}
			w.Execute(dataString)
		}
	}()
	return w
}

func (w *Workflow) GetSettings() WorkflowSettings {
	return w.settings
}

// GetTriggerChannel returns the workflow's trigger channel
func (w *Workflow) GetTriggerChannel() chan any {
	return w.triggerChannel
}

// GetOutputChannels returns the workflow's output channels
func (w *Workflow) GetOutputChannels() []chan any {
	return w.outputChannels
}

// SetWorkflowReferences sets the workflow map for sub-workflow execution
func (w *Workflow) SetWorkflowReferences(workflows map[string]*Workflow) {
	w.workflowMap = workflows
}

func (w *Workflow) RegisterTriggers(integrations map[string]types.Integration) error {
	w.logger.Info("Registering triggers for workflow", "workflowName", w.settings.Name, "triggerCount", len(w.settings.Triggers))
	for i, trigger := range w.settings.Triggers {
		w.logger.Info("Attempting to register trigger", "workflowName", w.settings.Name, "triggerIndex", i, "triggerIntegration", trigger.Integration, "triggerEvent", trigger.Event, "triggerData", trigger.Data)
		integration, ok := integrations[trigger.Integration]
		if !ok {
			w.logger.Error(fmt.Errorf("integration %s not found", trigger.Integration), "Integration not found during trigger registration", "workflowName", w.settings.Name, "integrationName", trigger.Integration)
			return fmt.Errorf("integration %s not found", trigger.Integration)
		}
		w.logger.Info("Found integration for trigger", "workflowName", w.settings.Name, "integrationName", trigger.Integration, "integrationType", fmt.Sprintf("%T", integration))
		integration.RegisterTrigger(trigger.Event, trigger.Data, w.triggerChannel)
		w.logger.Info("Called RegisterTrigger on integration", "workflowName", w.settings.Name, "integrationName", trigger.Integration)
	}
	w.logger.Info("Registering outputs for workflow", "workflowName", w.settings.Name, "outputCount", len(w.settings.Outputs))
	for i, output := range w.settings.Outputs {
		w.logger.Info("Attempting to register output", "workflowName", w.settings.Name, "outputIndex", i, "outputIntegration", output.Integration, "outputEvent", output.Event)
		integration, ok := integrations[output.Integration]
		if !ok {
			w.logger.Error(fmt.Errorf("integration %s not found", output.Integration), "Integration not found during output registration", "workflowName", w.settings.Name, "integrationName", output.Integration)
			return fmt.Errorf("integration %s not found", output.Integration)
		}
		w.logger.Info("Found integration for output", "workflowName", w.settings.Name, "integrationName", output.Integration, "integrationType", fmt.Sprintf("%T", integration))
		w.outputChannels[i] = integration.GetChannel()
		w.logger.Info("Got channel from integration", "workflowName", w.settings.Name, "integrationName", output.Integration)
	}
	w.logger.Info("Finished registering triggers for workflow", "workflowName", w.settings.Name)
	return nil
}

func (w *Workflow) Execute(data string) {
	results, err := w.ExecuteWorkflow(w.Steps, w.agentMap, PromptInput{
		Message: data,
	}, "", w.logger, w.ValidationFunctions)
	if err != nil {
		w.logger.Error(err, "Error executing workflow")
	}
	finalResult, ok := results["final"]
	if !ok || finalResult.Error != nil || finalResult.Content == "" {
		w.logger.Error(fmt.Errorf("final result not found"), "Final result not found")
		finalResult.Content = "An error occurred while executing the workflow, please try again."
	}
	for i := range w.settings.Outputs {
		var data any
		if w.settings.Outputs[i].Data == nil || w.settings.Outputs[i].Data == "" {
			data = finalResult.Content
		} else {
			data = map[string]string{
				"output": finalResult.Content,
				"data":   w.settings.Outputs[i].Data.(string),
			}
		}
		if w.outputChannels[i] == nil {
			w.logger.Error(fmt.Errorf("output channel %d is nil", i), "Output channel is nil")
			continue
		}
		w.logger.Info("Sending output to integration", "integration", w.settings.Outputs[i].Integration, "event", w.settings.Outputs[i].Event, "data", data)
		w.outputChannels[i] <- &types.TriggerSettings{
			Integration: w.settings.Outputs[i].Integration,
			Event:       w.settings.Outputs[i].Event,
			Data:        data,
		}
	}
}

// ExecuteWorkflow runs a workflow defined by the given steps using the provided agents
func (w *Workflow) ExecuteWorkflow(workflow []WorkflowStep, agentMap map[int]*Agent, promptInput PromptInput, path string, logger logr.Logger, validationFunctions []string) (map[string]WorkflowResult, error) {
	if len(workflow) == 0 {
		return nil, errors.New("workflow has no steps")
	}

	// Initialize workflow context
	ctx := &WorkflowContext{
		Results:             make(map[string]WorkflowResult),
		CurrentInput:        promptInput,
		Path:                path,
		Logger:              logger,
		ValidationFunctions: validationFunctions,
	}

	// Initialize validations
	validations := make([]validation.ValidationFunc, len(validationFunctions))
	for i, fn := range validationFunctions {
		v, ok := validation.Get(fn)
		if ok {
			validations[i] = v
		} else {
			ctx.Logger.Error(fmt.Errorf("validation function %s not found", fn), "Validation function not found")
		}
	}

	var err error
	var prevResult WorkflowResult
	var finalResult WorkflowResult
	validationFailed := true
	for i := 0; i < numValidationAttempts; i++ {

		// Execute steps in sequence
		for _, step := range workflow {
			// Execute the step
			result, err := w.executeWorkflowStep(step, agentMap, ctx, &prevResult)
			if err != nil {
				return ctx.Results, err
			}

			// Store the result
			ctx.Results[step.ID] = result

			// Set this step as the previous for the next iteration
			prevResult = result
			finalResult = result
		}

		// Create a new logger for the validation
		validationLogger := ctx.Logger.WithName("validation").WithValues("id", uuid.New().String())
		// Run validations after the final step if they exist at the workflow level
		// For workflow validators that validate content (not files), we need to pass the content directly
		for _, validationFunc := range validations {
			validatedContent, err := validationFunc(prevResult.Content)
			if err != nil {
				break
			}
			prevResult.Content = validatedContent
		}

		if err != nil {
			errString := fmt.Sprintf("Validation attempt %d out of %d failed, retrying: %s", i, numValidationAttempts, err)
			validationLogger.Error(err, errString, "output", prevResult.Content)
			continue
		}
		validationFailed = false
		validationLogger.Info("Validation Succeeded")
		break
	}
	if validationFailed {
		return ctx.Results, fmt.Errorf("validation of workflow results failed")
	}
	ctx.Results["final"] = finalResult
	return ctx.Results, nil
}

// executeWorkflowStep executes a single step in the workflow
func (w *Workflow) executeWorkflowStep(step WorkflowStep, agentMap map[int]*Agent, ctx *WorkflowContext, prevResult *WorkflowResult) (WorkflowResult, error) {
	result := WorkflowResult{
		AgentID:     step.AgentID,
		Integration: step.Integration.Integration,
		OutputField: step.OutputField,
		StepID:      step.ID,
	}

	/*
		TODO: Break this into multiple functions
	*/

	// Handle sub-workflow execution
	if step.WorkflowID != "" {
		return w.executeSubWorkflow(step, ctx, prevResult)
	}

	if step.Integration.Integration != "" {
		// Get the integration for this step
		integration, exists := w.integrations[step.Integration.Integration]
		if !exists {
			result.Error = fmt.Errorf("integration with ID %s not found", step.Integration.Integration)
			return result, result.Error
		}
		// Use previous step result if available, otherwise use original input
		var inputData any = ctx.CurrentInput.Message
		if prevResult != nil && prevResult.Content != "" {
			inputData = prevResult.Content
		}
		w.logger.Info("Calling integration", "integration", step.Integration.Integration, "event", step.Integration.Event, "data", inputData)
		response, err := integration.Call(step.Integration.Event, inputData)
		if err != nil {
			result.Error = fmt.Errorf("error calling integration: %w", err)
			return result, result.Error
		}
		result.Content = response.(string)
		return result, nil
	}
	// Get the agent for this step
	agent, exists := agentMap[step.AgentID]
	if !exists {
		result.Error = fmt.Errorf("agent with ID %d not found", step.AgentID)
		return result, result.Error
	}

	// Clone the agent to avoid shared state issues in parallel execution
	agent = agent.Clone()

	// Prepare input based on previous step if this is not the first step
	if prevResult != nil {
		// Using previous result's content as the next step's input
		// Default behavior is to use the previous step's output as the next step's input
		agent.SetPromptContext(prevResult.Content)
	}

	// Prepare the input for this step
	stepInput := ctx.CurrentInput

	// If this step has integration data but no integration name, use the data as the message
	if step.Integration.Integration == "" && step.Integration.Data != nil {
		if dataStr, ok := step.Integration.Data.(string); ok && dataStr != "" {
			// Create a new PromptInput with the step's data as the message
			stepInput = PromptInput{
				IssueTitle:        ctx.CurrentInput.IssueTitle,
				IssueBody:         ctx.CurrentInput.IssueBody,
				Commits:           ctx.CurrentInput.Commits,
				Diff:              ctx.CurrentInput.Diff,
				IsPRComment:       ctx.CurrentInput.IsPRComment,
				PRComment:         ctx.CurrentInput.PRComment,
				PRCommentDiffHunk: ctx.CurrentInput.PRCommentDiffHunk,
				Message:           dataStr,
			}
		}
	}

	// Execute the agent
	var content string
	var err error

	content, err = agent.GenerateWithTools(ctx.Path, stepInput)
	if err != nil {
		result.Error = err
		return result, err
	}

	// Process the output based on the specified output field
	result.Content = processOutput(content, step.OutputField)

	// Handle passthrough: validation agents should validate but return previous content
	if step.OutputField == "passthrough" {
		// Check if validation failed
		if strings.Contains(content, "INVALID:") {
			// Extract the reason after INVALID:
			parts := strings.Split(content, "INVALID:")
			if len(parts) > 1 {
				reason := strings.TrimSpace(parts[1])
				result.Error = fmt.Errorf("validation failed: %s", reason)
				return result, result.Error
			}
			result.Error = fmt.Errorf("validation failed")
			return result, result.Error
		}

		// Validation passed - return previous step's content instead of validation response
		if prevResult != nil && prevResult.Content != "" {
			result.Content = prevResult.Content
		} else {
			result.Content = ctx.CurrentInput.Message
		}
	}

	// Process udiffs if enabled
	if agent.GetUDiffSettings().Enabled {
		if err := agent.ProcessUDiffs(content, ctx.Logger); err != nil {
			ctx.Logger.Error(err, "Error processing udiffs for step", "stepID", step.ID)
			// Don't fail the workflow because of udiff errors
		}
	}

	return result, nil
}

// processOutput extracts the specified field from the agent's output
func processOutput(content string, outputField string) string {
	switch outputField {
	case "generatedText":
		return extractReasoning(content)
	case "generatedTextWithReasoning":
		// For generatedTextWithReasoning, we keep the reasoning section if it exists
		return content
	case "passthrough":
		// For passthrough, the content will be replaced with previous step content in executeWorkflowStep
		return content
	default:
		return content
	}
}

func extractReasoning(content string) string {
	// Find the positions of think tags
	startIdx := strings.Index(content, "<think>")
	endIdx := strings.Index(content, "</think>")

	// Case 1: Both tags exist - remove everything between them
	if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
		before := content[:startIdx]
		after := content[endIdx+8:] // 8 is the length of "</think>"
		return strings.TrimSpace(before + after)
	}

	// Case 2: Only closing tag exists (opening tag might be missing) - remove everything before and including it
	if endIdx != -1 {
		after := content[endIdx+8:] // 8 is the length of "</think>"
		return strings.TrimSpace(after)
	}

	// Case 3: Only opening tag exists (incomplete) or no tags - return as-is
	return content
}

// executeSubWorkflow executes a sub-workflow with retry logic
func (w *Workflow) executeSubWorkflow(step WorkflowStep, ctx *WorkflowContext, prevResult *WorkflowResult) (WorkflowResult, error) {
	result := WorkflowResult{
		OutputField: step.OutputField,
		StepID:      step.ID,
	}

	// Look up the sub-workflow by ID or name
	subWorkflow, exists := w.workflowMap[step.WorkflowID]
	if !exists {
		// Try looking up by name if ID lookup fails
		for _, workflow := range w.workflowMap {
			if workflow.settings.ID == step.WorkflowID || workflow.settings.Name == step.WorkflowID {
				subWorkflow = workflow
				exists = true
				break
			}
		}
		if !exists {
			result.Error = fmt.Errorf("workflow with ID %s not found", step.WorkflowID)
			return result, result.Error
		}
	}

	// Prepare input for sub-workflow
	var inputData string
	if prevResult != nil && prevResult.Content != "" {
		inputData = prevResult.Content
	} else {
		inputData = ctx.CurrentInput.Message
	}

	// Determine retry attempts
	maxAttempts := 1
	delayMs := 0
	if step.RetryConfig != nil {
		if step.RetryConfig.MaxAttempts > 0 {
			maxAttempts = step.RetryConfig.MaxAttempts
		}
		delayMs = step.RetryConfig.DelayMs
	}

	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		ctx.Logger.Info("Executing sub-workflow", "workflowID", step.WorkflowID, "attempt", attempt, "maxAttempts", maxAttempts)

		// Execute the sub-workflow
		subResults, err := subWorkflow.ExecuteWorkflow(
			subWorkflow.Steps,
			subWorkflow.agentMap,
			PromptInput{Message: inputData},
			ctx.Path,
			ctx.Logger.WithName("sub-workflow").WithValues("id", step.WorkflowID),
			subWorkflow.ValidationFunctions,
		)

		if err == nil {
			// Success - get the final result from the sub-workflow
			if finalResult, ok := subResults["final"]; ok {
				result.Content = processOutput(finalResult.Content, step.OutputField)
				return result, nil
			}
			// No final result but no error - use last step result
			for _, subStep := range subWorkflow.Steps {
				if subResult, ok := subResults[subStep.ID]; ok {
					result.Content = processOutput(subResult.Content, step.OutputField)
				}
			}
			if result.Content != "" {
				return result, nil
			}
			err = fmt.Errorf("sub-workflow %s produced no output", step.WorkflowID)
		}

		lastErr = err
		ctx.Logger.Error(err, "Sub-workflow execution failed", "workflowID", step.WorkflowID, "attempt", attempt)

		// Sleep before retry if configured and not the last attempt
		if attempt < maxAttempts && delayMs > 0 {
			time.Sleep(time.Duration(delayMs) * time.Millisecond)
		}
	}

	result.Error = fmt.Errorf("sub-workflow %s failed after %d attempts: %w", step.WorkflowID, maxAttempts, lastErr)
	return result, result.Error
}
