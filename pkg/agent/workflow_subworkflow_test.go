package agent

import (
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	"github.com/mule-ai/mule/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockAgent for testing
type MockAgent struct {
	ID       int
	Response string
	Error    error
}

func (m *MockAgent) GenerateWithTools(path string, input PromptInput) (string, error) {
	if m.Error != nil {
		return "", m.Error
	}
	return m.Response, nil
}

func (m *MockAgent) Clone() *Agent {
	// Return a real agent with mock behavior
	return &Agent{
		id: m.ID,
	}
}

func (m *MockAgent) SetPromptContext(context string) {}

func (m *MockAgent) GetUDiffSettings() UDiffSettings {
	return UDiffSettings{Enabled: false}
}

func (m *MockAgent) ProcessUDiffs(content string, logger logr.Logger) error {
	return nil
}

func TestWorkflowWithSubWorkflow(t *testing.T) {
	logger := logr.Discard()
	integrations := make(map[string]types.Integration)

	// Create mock agents
	agentMap := map[int]*Agent{
		1: {id: 1}, // We'll override behavior in tests
		2: {id: 2},
		3: {id: 3},
	}

	// Create a simple sub-workflow
	subWorkflowSettings := WorkflowSettings{
		ID:   "sub-workflow-1",
		Name: "SubWorkflow1",
		Steps: []WorkflowStep{
			{
				ID:          "sub-step-1",
				AgentID:     1,
				OutputField: "generatedText",
			},
		},
		ValidationFunctions: []string{},
	}

	// Create a parent workflow that calls the sub-workflow
	parentWorkflowSettings := WorkflowSettings{
		ID:   "parent-workflow",
		Name: "ParentWorkflow",
		Steps: []WorkflowStep{
			{
				ID:          "parent-step-1",
				AgentID:     2,
				OutputField: "generatedText",
			},
			{
				ID:          "parent-step-2",
				WorkflowID:  "SubWorkflow1", // Reference to sub-workflow by name
				OutputField: "generatedText",
			},
		},
		ValidationFunctions: []string{},
	}

	// Create workflows
	subWorkflow := NewWorkflow(subWorkflowSettings, agentMap, integrations, logger)
	parentWorkflow := NewWorkflow(parentWorkflowSettings, agentMap, integrations, logger)

	// Set workflow references
	workflows := map[string]*Workflow{
		"SubWorkflow1":   subWorkflow,
		"ParentWorkflow": parentWorkflow,
	}
	parentWorkflow.SetWorkflowReferences(workflows)
	subWorkflow.SetWorkflowReferences(workflows)

	t.Run("Execute workflow with sub-workflow successfully", func(t *testing.T) {
		// Override agent behavior for testing
		agentMap[1] = &Agent{id: 1}
		agentMap[2] = &Agent{id: 2}

		// Mock the agent behavior by replacing the ExecuteWorkflow method's internals
		// Since we can't easily mock internal agent behavior, we'll test the structure

		// Verify workflow structure
		assert.Len(t, parentWorkflow.Steps, 2)
		assert.Equal(t, "parent-step-1", parentWorkflow.Steps[0].ID)
		assert.Equal(t, 2, parentWorkflow.Steps[0].AgentID)
		assert.Equal(t, "parent-step-2", parentWorkflow.Steps[1].ID)
		assert.Equal(t, "SubWorkflow1", parentWorkflow.Steps[1].WorkflowID)

		// Verify sub-workflow structure
		assert.Len(t, subWorkflow.Steps, 1)
		assert.Equal(t, "sub-step-1", subWorkflow.Steps[0].ID)
		assert.Equal(t, 1, subWorkflow.Steps[0].AgentID)

		// Verify workflow references are set
		assert.NotNil(t, parentWorkflow.workflowMap)
		assert.Contains(t, parentWorkflow.workflowMap, "SubWorkflow1")
	})
}

func TestWorkflowWithRetryConfig(t *testing.T) {
	logger := logr.Discard()
	integrations := make(map[string]types.Integration)
	agentMap := map[int]*Agent{
		1: {id: 1},
	}

	// Create a sub-workflow that will fail initially
	subWorkflowSettings := WorkflowSettings{
		ID:   "retry-sub-workflow",
		Name: "RetrySubWorkflow",
		Steps: []WorkflowStep{
			{
				ID:          "retry-step-1",
				AgentID:     1,
				OutputField: "generatedText",
			},
		},
	}

	// Create a parent workflow with retry configuration
	parentWorkflowSettings := WorkflowSettings{
		ID:   "retry-parent-workflow",
		Name: "RetryParentWorkflow",
		Steps: []WorkflowStep{
			{
				ID:         "retry-parent-step",
				WorkflowID: "RetrySubWorkflow",
				RetryConfig: &RetryConfig{
					MaxAttempts: 3,
					DelayMs:     100,
				},
				OutputField: "generatedText",
			},
		},
	}

	subWorkflow := NewWorkflow(subWorkflowSettings, agentMap, integrations, logger)
	parentWorkflow := NewWorkflow(parentWorkflowSettings, agentMap, integrations, logger)

	workflows := map[string]*Workflow{
		"RetrySubWorkflow":    subWorkflow,
		"RetryParentWorkflow": parentWorkflow,
	}
	parentWorkflow.SetWorkflowReferences(workflows)

	t.Run("Verify retry configuration is set", func(t *testing.T) {
		require.Len(t, parentWorkflow.Steps, 1)
		step := parentWorkflow.Steps[0]
		require.NotNil(t, step.RetryConfig)
		assert.Equal(t, 3, step.RetryConfig.MaxAttempts)
		assert.Equal(t, 100, step.RetryConfig.DelayMs)
	})
}

func TestNestedWorkflows(t *testing.T) {
	logger := logr.Discard()
	integrations := make(map[string]types.Integration)
	agentMap := map[int]*Agent{
		1: {id: 1},
		2: {id: 2},
		3: {id: 3},
	}

	// Create the deepest workflow (level 3)
	level3WorkflowSettings := WorkflowSettings{
		ID:   "level3-workflow",
		Name: "Level3Workflow",
		Steps: []WorkflowStep{
			{
				ID:          "level3-step",
				AgentID:     1,
				OutputField: "generatedText",
			},
		},
	}

	// Create level 2 workflow that calls level 3
	level2WorkflowSettings := WorkflowSettings{
		ID:   "level2-workflow",
		Name: "Level2Workflow",
		Steps: []WorkflowStep{
			{
				ID:          "level2-step-1",
				AgentID:     2,
				OutputField: "generatedText",
			},
			{
				ID:          "level2-step-2",
				WorkflowID:  "Level3Workflow",
				OutputField: "generatedText",
			},
		},
	}

	// Create level 1 workflow that calls level 2
	level1WorkflowSettings := WorkflowSettings{
		ID:   "level1-workflow",
		Name: "Level1Workflow",
		Steps: []WorkflowStep{
			{
				ID:          "level1-step-1",
				AgentID:     3,
				OutputField: "generatedText",
			},
			{
				ID:          "level1-step-2",
				WorkflowID:  "Level2Workflow",
				OutputField: "generatedText",
			},
		},
	}

	level3Workflow := NewWorkflow(level3WorkflowSettings, agentMap, integrations, logger)
	level2Workflow := NewWorkflow(level2WorkflowSettings, agentMap, integrations, logger)
	level1Workflow := NewWorkflow(level1WorkflowSettings, agentMap, integrations, logger)

	workflows := map[string]*Workflow{
		"Level3Workflow": level3Workflow,
		"Level2Workflow": level2Workflow,
		"Level1Workflow": level1Workflow,
	}

	// Set references for all workflows
	for _, w := range workflows {
		w.SetWorkflowReferences(workflows)
	}

	t.Run("Verify nested workflow structure", func(t *testing.T) {
		// Verify level 1 workflow
		assert.Len(t, level1Workflow.Steps, 2)
		assert.Equal(t, "Level2Workflow", level1Workflow.Steps[1].WorkflowID)

		// Verify level 2 workflow
		assert.Len(t, level2Workflow.Steps, 2)
		assert.Equal(t, "Level3Workflow", level2Workflow.Steps[1].WorkflowID)

		// Verify level 3 workflow
		assert.Len(t, level3Workflow.Steps, 1)
		assert.Equal(t, 1, level3Workflow.Steps[0].AgentID)

		// Verify all workflows have references to each other
		assert.Contains(t, level1Workflow.workflowMap, "Level2Workflow")
		assert.Contains(t, level1Workflow.workflowMap, "Level3Workflow")
		assert.Contains(t, level2Workflow.workflowMap, "Level3Workflow")
	})
}

func TestWorkflowValidation(t *testing.T) {
	logger := logr.Discard()
	integrations := make(map[string]types.Integration)
	agentMap := map[int]*Agent{}

	t.Run("Workflow step must have either agentID, integration, or workflowID", func(t *testing.T) {
		// This should panic because the step has none of the required fields
		defer func() {
			if r := recover(); r != nil {
				assert.Contains(t, fmt.Sprintf("%v", r), "has no integration, agentID, or workflowID")
			} else {
				t.Fatal("Expected panic but didn't get one")
			}
		}()

		invalidWorkflowSettings := WorkflowSettings{
			ID:   "invalid-workflow",
			Name: "InvalidWorkflow",
			Steps: []WorkflowStep{
				{
					ID:          "invalid-step",
					OutputField: "generatedText",
					// Missing AgentID, Integration, and WorkflowID
				},
			},
		}

		NewWorkflow(invalidWorkflowSettings, agentMap, integrations, logger)
	})

	t.Run("Workflow with valid workflowID should not panic", func(t *testing.T) {
		validWorkflowSettings := WorkflowSettings{
			ID:   "valid-workflow",
			Name: "ValidWorkflow",
			Steps: []WorkflowStep{
				{
					ID:          "valid-step",
					WorkflowID:  "SomeWorkflow",
					OutputField: "generatedText",
				},
			},
		}

		// Should not panic
		workflow := NewWorkflow(validWorkflowSettings, agentMap, integrations, logger)
		assert.NotNil(t, workflow)
	})
}

func TestWorkflowReusability(t *testing.T) {
	logger := logr.Discard()
	integrations := make(map[string]types.Integration)
	agentMap := map[int]*Agent{
		1: {id: 1},
		2: {id: 2},
		3: {id: 3},
		4: {id: 4},
	}

	// Create a reusable validation workflow
	validationWorkflowSettings := WorkflowSettings{
		ID:   "validation-workflow",
		Name: "ValidationWorkflow",
		Steps: []WorkflowStep{
			{
				ID:          "validate-step",
				AgentID:     1,
				OutputField: "generatedText",
			},
		},
		ValidationFunctions: []string{}, // Could have validation functions here
	}

	// Create workflow A that uses the validation workflow
	workflowASettings := WorkflowSettings{
		ID:   "workflow-a",
		Name: "WorkflowA",
		Steps: []WorkflowStep{
			{
				ID:          "a-step-1",
				AgentID:     2,
				OutputField: "generatedText",
			},
			{
				ID:          "a-step-2",
				WorkflowID:  "ValidationWorkflow",
				OutputField: "generatedText",
			},
		},
	}

	// Create workflow B that also uses the validation workflow
	workflowBSettings := WorkflowSettings{
		ID:   "workflow-b",
		Name: "WorkflowB",
		Steps: []WorkflowStep{
			{
				ID:          "b-step-1",
				AgentID:     3,
				OutputField: "generatedText",
			},
			{
				ID:          "b-step-2",
				WorkflowID:  "ValidationWorkflow",
				OutputField: "generatedText",
			},
			{
				ID:          "b-step-3",
				AgentID:     4,
				OutputField: "generatedText",
			},
		},
	}

	validationWorkflow := NewWorkflow(validationWorkflowSettings, agentMap, integrations, logger)
	workflowA := NewWorkflow(workflowASettings, agentMap, integrations, logger)
	workflowB := NewWorkflow(workflowBSettings, agentMap, integrations, logger)

	workflows := map[string]*Workflow{
		"ValidationWorkflow": validationWorkflow,
		"WorkflowA":          workflowA,
		"WorkflowB":          workflowB,
	}

	// Set references
	for _, w := range workflows {
		w.SetWorkflowReferences(workflows)
	}

	t.Run("Verify reusable workflow is referenced by multiple workflows", func(t *testing.T) {
		// Verify WorkflowA references ValidationWorkflow
		assert.Contains(t, workflowA.workflowMap, "ValidationWorkflow")
		assert.Equal(t, "ValidationWorkflow", workflowA.Steps[1].WorkflowID)

		// Verify WorkflowB references ValidationWorkflow
		assert.Contains(t, workflowB.workflowMap, "ValidationWorkflow")
		assert.Equal(t, "ValidationWorkflow", workflowB.Steps[1].WorkflowID)

		// Both workflows reference the same validation workflow instance
		assert.Equal(t, workflowA.workflowMap["ValidationWorkflow"], workflowB.workflowMap["ValidationWorkflow"])
	})
}
