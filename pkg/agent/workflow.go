package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/mule-ai/mule/pkg/validation"
)

const numValidationAttempts = 20

// WorkflowStep represents a step in a workflow
type WorkflowStep struct {
	ID          string `json:"id"`
	AgentID     int    `json:"agentID"`
	AgentName   string `json:"agentName"`
	OutputField string `json:"outputField"`
}

// WorkflowResult represents the result of a workflow step execution
type WorkflowResult struct {
	AgentID     int
	OutputField string
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

// ExecuteWorkflow runs a workflow defined by the given steps using the provided agents
func ExecuteWorkflow(workflow []WorkflowStep, agentMap map[int]*Agent, promptInput PromptInput, path string, logger logr.Logger, validationFunctions []string) (map[string]WorkflowResult, error) {
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
	var prevResult *WorkflowResult
	validationFailed := true
	for i := 0; i < numValidationAttempts; i++ {

		// Execute steps in sequence
		for _, step := range workflow {
			// Execute the step
			result, err := executeWorkflowStep(step, agentMap, ctx, prevResult)
			if err != nil {
				return ctx.Results, err
			}

			// Store the result
			ctx.Results[step.ID] = result

			// Set this step as the previous for the next iteration
			prevResult = &result
		}

		// Run validations after the final step if they exist at the workflow level
		prevResult.Content, err = validation.Run(&validation.ValidationInput{
			Validations: validations,
			Logger:      ctx.Logger,
			Path:        ctx.Path,
		})

		if err != nil {
			errString := fmt.Sprintf("Validation attempt %d out of %d failed, retrying: %s", i, numValidationAttempts, err)
			ctx.Logger.Error(err, errString, "output", prevResult.Content)
			continue
		}
		validationFailed = false
		ctx.Logger.Info("Validation Succeeded")
		break
	}
	if validationFailed {
		return ctx.Results, fmt.Errorf("validation of workflow results failed")
	}
	return ctx.Results, nil
}

// executeWorkflowStep executes a single step in the workflow
func executeWorkflowStep(step WorkflowStep, agentMap map[int]*Agent, ctx *WorkflowContext, prevResult *WorkflowResult) (WorkflowResult, error) {
	result := WorkflowResult{
		AgentID:     step.AgentID,
		OutputField: step.OutputField,
		StepID:      step.ID,
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
