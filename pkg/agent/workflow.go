package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/mule-ai/mule/pkg/integration"
	"github.com/mule-ai/mule/pkg/integration/types"
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
	integrations        map[string]integration.Integration
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

func NewWorkflow(settings WorkflowSettings, agentMap map[int]*Agent, integrations map[string]integration.Integration, logger logr.Logger) *Workflow {
	w := &Workflow{
		settings:            settings,
		Steps:               settings.Steps,
		ValidationFunctions: settings.ValidationFunctions,
		triggerChannel:      make(chan any),
		outputChannels:      make([]chan any, len(settings.Outputs)),
		agentMap:            agentMap,
		logger:              logger,
		integrations:        integrations,
	}

	for _, step := range w.Steps {
		if step.Integration.Integration == "" && step.AgentID == 0 {
			w.logger.Error(fmt.Errorf("step %s has no integration or agentID", step.ID), "Step has no integration or agentID")
			panic(fmt.Errorf("step %s has no integration or agentID", step.ID))
		}
	}

	go func() {
		for data := range w.triggerChannel {
			dataString, ok := data.(string)
			if !ok {
				w.logger.Error(fmt.Errorf("data is not a string"), "Data is not a string")
				continue
			}
			w.Execute(dataString)
		}
	}()
	return w
}

func (w *Workflow) RegisterTriggers(integrations map[string]integration.Integration) error {
	for _, trigger := range w.settings.Triggers {
		integration, ok := integrations[trigger.Integration]
		if !ok {
			return fmt.Errorf("integration %s not found", trigger.Integration)
		}
		integration.RegisterTrigger(trigger.Event, trigger.Data, w.triggerChannel)
	}
	for i, output := range w.settings.Outputs {
		integration, ok := integrations[output.Integration]
		if !ok {
			return fmt.Errorf("integration %s not found", output.Integration)
		}
		w.outputChannels[i] = integration.GetChannel()
	}
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
		w.outputChannels[i] <- &types.TriggerSettings{
			Integration: w.settings.Outputs[i].Integration,
			Event:       w.settings.Outputs[i].Event,
			Data:        finalResult.Content,
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
		prevResult.Content, err = validation.Run(&validation.ValidationInput{
			Validations: validations,
			Logger:      validationLogger,
			Path:        ctx.Path,
		})

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

	if step.Integration.Integration != "" {
		// Get the integration for this step
		integration, exists := w.integrations[step.Integration.Integration]
		if !exists {
			result.Error = fmt.Errorf("integration with ID %s not found", step.Integration.Integration)
			return result, result.Error
		}
		w.logger.Info("Calling integration", "integration", step.Integration.Integration, "event", step.Integration.Event, "data", ctx.CurrentInput.Message)
		response, err := integration.Call(step.Integration.Event, ctx.CurrentInput.Message)
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

	// Prepare input based on previous step if this is not the first step
	if prevResult != nil {
		// Using previous result's content as the next step's input
		// Default behavior is to use the previous step's output as the next step's input
		agent.SetPromptContext(prevResult.Content)
	}

	// Execute the agent
	var content string
	var err error

	content, err = agent.GenerateWithTools(ctx.Path, ctx.CurrentInput)
	if err != nil {
		result.Error = err
		return result, err
	}

	// Process the output based on the specified output field
	result.Content = processOutput(content, step.OutputField)

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
	default:
		return content
	}
}

func extractReasoning(content string) string {
	split := strings.Split(content, `</think>`)
	if len(split) < 2 {
		return content
	}
	reasoning := strings.TrimSpace(split[1])
	return reasoning
}
