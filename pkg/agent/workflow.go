package agent

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
)

// WorkflowStep represents a step in a workflow
type WorkflowStep struct {
	ID           string
	AgentName    string
	InputMapping string
	OutputField  string
	IsFirst      bool
}

// WorkflowResult represents the result of a workflow step execution
type WorkflowResult struct {
	AgentName   string
	OutputField string
	Content     string
	IsFirstStep bool
	StepID      string
	Error       error
}

// WorkflowContext holds the state of a workflow execution
type WorkflowContext struct {
	Results      map[string]WorkflowResult // Map of step ID to result
	CurrentInput PromptInput
	Path         string
}

// RunWorkflow executes a workflow with the given agents and input
func RunWorkflow(agents map[int]*Agent, promptInput PromptInput, path string) error {
	// This is the legacy workflow function, maintained for backward compatibility
	reasoningAgent, err := reasoningAgent(agents)
	if err != nil {
		return err
	}
	codeAgent, err := codeAgent(agents)
	if err != nil {
		return err
	}
	// reasoning, err := reasoningAgent.Generate(path, promptInput)
	reasoning, err := reasoningAgent.GenerateWithTools(path, promptInput)
	if err != nil {
		return err
	}
	reasoning = extractReasoning(reasoning, reasoningAgent.logger)
	codeAgent.SetPromptContext(reasoning)
	err = codeAgent.RunInPath(path, promptInput)
	if err != nil {
		return err
	}

	return nil
}

func extractReasoning(content string, logger logr.Logger) string {
	split := strings.Split(content, `</think>`)
	if len(split) < 2 {
		logger.Error(fmt.Errorf("reasoning agent did not return a think section"), "Reasoning agent did not return a think section, returning original content", "content", content)
		return content
	}
	logger.Info("Reasoning Response", "content", split[1])
	reasoning := strings.TrimSpace(split[1])
	return reasoning
}

// ExecuteWorkflow runs a workflow defined by the given steps using the provided agents
func ExecuteWorkflow(workflow []WorkflowStep, agentMap map[string]*Agent, promptInput PromptInput, path string) (map[string]WorkflowResult, error) {
	if len(workflow) == 0 {
		return nil, errors.New("workflow has no steps")
	}

	// Initialize workflow context
	ctx := &WorkflowContext{
		Results:      make(map[string]WorkflowResult),
		CurrentInput: promptInput,
		Path:         path,
	}

	// Find the first step
	var firstStep *WorkflowStep
	for i := range workflow {
		if workflow[i].IsFirst {
			firstStep = &workflow[i]
			break
		}
	}

	// If no step is marked as first, use the first one in the list
	if firstStep == nil {
		firstStep = &workflow[0]
	}

	// Execute the first step
	result, err := executeWorkflowStep(*firstStep, agentMap, ctx, nil)
	if err != nil {
		return ctx.Results, err
	}
	ctx.Results[firstStep.ID] = result

	// Execute the remaining steps in order
	executedSteps := map[string]bool{firstStep.ID: true}
	for len(executedSteps) < len(workflow) {
		// Find the next step to execute
		var nextStep *WorkflowStep
		for i := range workflow {
			if executedSteps[workflow[i].ID] {
				continue
			}

			// Check if all dependencies are satisfied
			dependenciesSatisfied := true
			for j := range workflow {
				if workflow[j].ID == workflow[i].ID {
					continue
				}
				if !executedSteps[workflow[j].ID] {
					dependenciesSatisfied = false
					break
				}
			}

			if dependenciesSatisfied {
				nextStep = &workflow[i]
				break
			}
		}

		if nextStep == nil {
			break // No more steps to execute
		}

		// Find the previous step result to use as input
		var prevResult *WorkflowResult
		for j := range workflow {
			if executedSteps[workflow[j].ID] {
				res := ctx.Results[workflow[j].ID]
				prevResult = &res
				break
			}
		}

		// Execute the step
		result, err := executeWorkflowStep(*nextStep, agentMap, ctx, prevResult)
		if err != nil {
			return ctx.Results, err
		}
		ctx.Results[nextStep.ID] = result
		executedSteps[nextStep.ID] = true
	}

	return ctx.Results, nil
}

// executeWorkflowStep executes a single step in the workflow
func executeWorkflowStep(step WorkflowStep, agentMap map[string]*Agent, ctx *WorkflowContext, prevResult *WorkflowResult) (WorkflowResult, error) {
	result := WorkflowResult{
		AgentName:   step.AgentName,
		OutputField: step.OutputField,
		IsFirstStep: step.IsFirst,
		StepID:      step.ID,
	}

	// Get the agent for this step
	agent, exists := agentMap[step.AgentName]
	if !exists {
		result.Error = fmt.Errorf("agent %s not found", step.AgentName)
		return result, result.Error
	}

	// Prepare input based on previous step if this is not the first step
	if !step.IsFirst && prevResult != nil {
		switch step.InputMapping {
		case "useAsPrompt":
			// Replace the prompt with the previous step's output
			ctx.CurrentInput.IssueBody = prevResult.Content
		case "appendToPrompt":
			// Append the previous step's output to the prompt
			ctx.CurrentInput.IssueBody += "\n\n" + prevResult.Content
		case "useAsContext":
			// Set the previous step's output as context for the agent
			agent.SetPromptContext(prevResult.Content)
		case "useAsInstructions":
			// Add the previous step's output as instructions
			ctx.CurrentInput.IssueBody = fmt.Sprintf("Instructions:\n%s\n\nOriginal request:\n%s",
				prevResult.Content, ctx.CurrentInput.IssueBody)
		case "useAsCodeInput":
			// Use the previous step's output as code to be processed
			ctx.CurrentInput.Diff = prevResult.Content
		case "useAsReviewTarget":
			// Use the previous step's output as the target for a review
			ctx.CurrentInput.PRCommentDiffHunk = prevResult.Content
			ctx.CurrentInput.IsPRComment = true
		}
	}

	// Execute the agent
	var content string
	var err error

	// Generate content using the agent
	content, err = agent.Generate(ctx.Path, ctx.CurrentInput)
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
		return content
	case "generatedTextWithReasoning":
		// For generatedTextWithReasoning, we keep the reasoning section if it exists
		return content
	case "extractedCode":
		return extractCode(content)
	case "summary":
		return extractSummary(content)
	case "actionItems":
		return extractActionItems(content)
	case "suggestedChanges":
		return extractSuggestedChanges(content)
	case "reviewComments":
		return extractReviewComments(content)
	case "testCases":
		return extractTestCases(content)
	case "documentationText":
		return extractDocumentation(content)
	default:
		return content
	}
}

// Helper functions to extract specific content from agent output

func extractCode(content string) string {
	// Extract code blocks from markdown
	codeBlocks := []string{}
	lines := strings.Split(content, "\n")
	inCodeBlock := false
	currentBlock := []string{}

	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				// End of code block
				inCodeBlock = false
				if len(currentBlock) > 0 {
					codeBlocks = append(codeBlocks, strings.Join(currentBlock, "\n"))
				}
				currentBlock = []string{}
			} else {
				// Start of code block
				inCodeBlock = true
			}
		} else if inCodeBlock {
			currentBlock = append(currentBlock, line)
		}
	}

	return strings.Join(codeBlocks, "\n\n")
}

func extractSummary(content string) string {
	// Look for a summary section
	summaryStart := strings.Index(strings.ToLower(content), "summary:")
	if summaryStart == -1 {
		summaryStart = strings.Index(strings.ToLower(content), "## summary")
	}

	if summaryStart != -1 {
		// Find the end of the summary (next heading or end of text)
		summaryEnd := len(content)
		nextHeading := strings.Index(content[summaryStart:], "\n#")
		if nextHeading != -1 {
			summaryEnd = summaryStart + nextHeading
		}

		return strings.TrimSpace(content[summaryStart:summaryEnd])
	}

	// If no explicit summary, return the first paragraph
	paragraphs := strings.Split(content, "\n\n")
	if len(paragraphs) > 0 {
		return strings.TrimSpace(paragraphs[0])
	}

	return content
}

func extractActionItems(content string) string {
	// Look for action items, tasks, or to-do lists
	actionItemsStart := -1

	possibleHeaders := []string{
		"action items:", "action items", "## action items",
		"tasks:", "tasks", "## tasks",
		"to-do:", "to-do", "## to-do",
		"todo:", "todo", "## todo",
	}

	for _, header := range possibleHeaders {
		actionItemsStart = strings.Index(strings.ToLower(content), header)
		if actionItemsStart != -1 {
			break
		}
	}

	if actionItemsStart != -1 {
		// Find the end of the action items (next heading or end of text)
		actionItemsEnd := len(content)
		nextHeading := strings.Index(content[actionItemsStart:], "\n#")
		if nextHeading != -1 {
			actionItemsEnd = actionItemsStart + nextHeading
		}

		return strings.TrimSpace(content[actionItemsStart:actionItemsEnd])
	}

	// Look for bullet points or numbered lists
	items := []string{}
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") ||
			(len(trimmed) > 2 && trimmed[0] >= '0' && trimmed[0] <= '9' && trimmed[1] == '.') {
			items = append(items, trimmed)
		}
	}

	if len(items) > 0 {
		return strings.Join(items, "\n")
	}

	return ""
}

func extractSuggestedChanges(content string) string {
	// Look for suggested changes or code modifications
	changesStart := -1

	possibleHeaders := []string{
		"suggested changes:", "suggested changes", "## suggested changes",
		"code changes:", "code changes", "## code changes",
		"modifications:", "modifications", "## modifications",
	}

	for _, header := range possibleHeaders {
		changesStart = strings.Index(strings.ToLower(content), header)
		if changesStart != -1 {
			break
		}
	}

	if changesStart != -1 {
		// Find the end of the changes (next heading or end of text)
		changesEnd := len(content)
		nextHeading := strings.Index(content[changesStart:], "\n#")
		if nextHeading != -1 {
			changesEnd = changesStart + nextHeading
		}

		return strings.TrimSpace(content[changesStart:changesEnd])
	}

	// If no explicit changes section, look for code blocks
	return extractCode(content)
}

func extractReviewComments(content string) string {
	// Look for review comments or feedback
	reviewStart := -1

	possibleHeaders := []string{
		"review:", "review", "## review",
		"feedback:", "feedback", "## feedback",
		"comments:", "comments", "## comments",
	}

	for _, header := range possibleHeaders {
		reviewStart = strings.Index(strings.ToLower(content), header)
		if reviewStart != -1 {
			break
		}
	}

	if reviewStart != -1 {
		// Find the end of the review (next heading or end of text)
		reviewEnd := len(content)
		nextHeading := strings.Index(content[reviewStart:], "\n#")
		if nextHeading != -1 {
			reviewEnd = reviewStart + nextHeading
		}

		return strings.TrimSpace(content[reviewStart:reviewEnd])
	}

	return content
}

func extractTestCases(content string) string {
	// Look for test cases or testing sections
	testStart := -1

	possibleHeaders := []string{
		"test cases:", "test cases", "## test cases",
		"tests:", "tests", "## tests",
		"testing:", "testing", "## testing",
	}

	for _, header := range possibleHeaders {
		testStart = strings.Index(strings.ToLower(content), header)
		if testStart != -1 {
			break
		}
	}

	if testStart != -1 {
		// Find the end of the test cases (next heading or end of text)
		testEnd := len(content)
		nextHeading := strings.Index(content[testStart:], "\n#")
		if nextHeading != -1 {
			testEnd = testStart + nextHeading
		}

		return strings.TrimSpace(content[testStart:testEnd])
	}

	// Look for code blocks that might contain tests
	codeBlocks := []string{}
	lines := strings.Split(content, "\n")
	inCodeBlock := false
	currentBlock := []string{}
	isTestBlock := false

	for _, line := range lines {
		if strings.HasPrefix(line, "```") {
			if inCodeBlock {
				// End of code block
				inCodeBlock = false
				if isTestBlock && len(currentBlock) > 0 {
					codeBlocks = append(codeBlocks, strings.Join(currentBlock, "\n"))
				}
				currentBlock = []string{}
				isTestBlock = false
			} else {
				// Start of code block
				inCodeBlock = true
				isTestBlock = strings.Contains(strings.ToLower(line), "test") ||
					strings.Contains(strings.ToLower(line), "spec")
			}
		} else if inCodeBlock {
			currentBlock = append(currentBlock, line)
			// Check if this looks like a test
			if !isTestBlock && (strings.Contains(strings.ToLower(line), "test") ||
				strings.Contains(strings.ToLower(line), "assert") ||
				strings.Contains(strings.ToLower(line), "expect")) {
				isTestBlock = true
			}
		}
	}

	if len(codeBlocks) > 0 {
		return strings.Join(codeBlocks, "\n\n")
	}

	return ""
}

func extractDocumentation(content string) string {
	// Look for documentation sections
	docStart := -1

	possibleHeaders := []string{
		"documentation:", "documentation", "## documentation",
		"docs:", "docs", "## docs",
		"usage:", "usage", "## usage",
		"api:", "api", "## api",
	}

	for _, header := range possibleHeaders {
		docStart = strings.Index(strings.ToLower(content), header)
		if docStart != -1 {
			break
		}
	}

	if docStart != -1 {
		// Find the end of the documentation (next heading or end of text)
		docEnd := len(content)
		nextHeading := strings.Index(content[docStart:], "\n#")
		if nextHeading != -1 {
			docEnd = docStart + nextHeading
		}

		return strings.TrimSpace(content[docStart:docEnd])
	}

	return content
}

func reasoningAgent(agents map[int]*Agent) (*Agent, error) {
	for _, agent := range agents {
		if agent.name == "reasoning" {
			return agent, nil
		}
	}
	return nil, fmt.Errorf("reasoning agent not found")
}

func codeAgent(agents map[int]*Agent) (*Agent, error) {
	for _, agent := range agents {
		if agent.name == "code" {
			return agent, nil
		}
	}
	return nil, fmt.Errorf("code agent not found")
}
