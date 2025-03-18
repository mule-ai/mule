package agent

import (
	"errors"
	"fmt"
	"strings"
)

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
	Results      map[string]WorkflowResult // Map of step ID to result
	CurrentInput PromptInput
	Path         string
}

// ExecuteWorkflow runs a workflow defined by the given steps using the provided agents
func ExecuteWorkflow(workflow []WorkflowStep, agentMap map[int]*Agent, promptInput PromptInput, path string) (map[string]WorkflowResult, error) {
	if len(workflow) == 0 {
		return nil, errors.New("workflow has no steps")
	}

	// Initialize workflow context
	ctx := &WorkflowContext{
		Results:      make(map[string]WorkflowResult),
		CurrentInput: promptInput,
		Path:         path,
	}

	var prevResult *WorkflowResult

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

	// Generate content using the agent
	content, err = agent.GenerateWithTools(ctx.Path, ctx.CurrentInput)
	if err != nil {
		result.Error = err
		return result, err
	}

	// Process the output based on the specified output field
	result.Content = processOutput(content, step.OutputField)

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
