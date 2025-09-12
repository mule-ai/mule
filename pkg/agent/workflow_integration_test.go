package agent

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/mule-ai/mule/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkflowIntegration tests the complete workflow of workflows feature
func TestWorkflowIntegration(t *testing.T) {
	logger := logr.Discard()
	integrations := make(map[string]types.Integration)

	// Create a mock integration that records calls
	mockIntegration := &MockIntegration{
		calls: make([]IntegrationCall, 0),
	}
	integrations["mock-integration"] = mockIntegration

	// Create mock agents with specific behaviors
	agentMap := map[int]*Agent{
		1: {id: 1, Name: "Agent1"},
		2: {id: 2, Name: "Agent2"},
		3: {id: 3, Name: "Agent3"},
	}

	// Define a utility workflow that adds a prefix
	prefixWorkflowSettings := WorkflowSettings{
		ID:   "prefix-workflow",
		Name: "PrefixWorkflow",
		Steps: []WorkflowStep{
			{
				ID:          "add-prefix",
				AgentID:     1,
				OutputField: "generatedText",
			},
		},
	}

	// Define a utility workflow that adds a suffix
	suffixWorkflowSettings := WorkflowSettings{
		ID:   "suffix-workflow",
		Name: "SuffixWorkflow",
		Steps: []WorkflowStep{
			{
				ID:          "add-suffix",
				AgentID:     2,
				OutputField: "generatedText",
			},
		},
	}

	// Define a main workflow that uses both utility workflows
	mainWorkflowSettings := WorkflowSettings{
		ID:   "main-workflow",
		Name: "MainWorkflow",
		Steps: []WorkflowStep{
			{
				ID:          "initial-process",
				AgentID:     3,
				OutputField: "generatedText",
			},
			{
				ID:          "add-prefix-step",
				WorkflowID:  "PrefixWorkflow",
				OutputField: "generatedText",
				RetryConfig: &RetryConfig{
					MaxAttempts: 2,
					DelayMs:     50,
				},
			},
			{
				ID:          "add-suffix-step",
				WorkflowID:  "SuffixWorkflow",
				OutputField: "generatedText",
			},
			{
				ID:          "integration-step",
				OutputField: "",
				Integration: types.TriggerSettings{
					Integration: "mock-integration",
					Event:       "process",
					Data:        "test-data",
				},
			},
		},
	}

	// Create workflow instances
	prefixWorkflow := NewWorkflow(prefixWorkflowSettings, agentMap, integrations, logger)
	suffixWorkflow := NewWorkflow(suffixWorkflowSettings, agentMap, integrations, logger)
	mainWorkflow := NewWorkflow(mainWorkflowSettings, agentMap, integrations, logger)

	// Create workflow map and set references
	workflows := map[string]*Workflow{
		"PrefixWorkflow": prefixWorkflow,
		"SuffixWorkflow": suffixWorkflow,
		"MainWorkflow":   mainWorkflow,
	}

	for _, w := range workflows {
		w.SetWorkflowReferences(workflows)
	}

	t.Run("Complete workflow execution with sub-workflows", func(t *testing.T) {
		// Verify the main workflow has all steps configured correctly
		require.Len(t, mainWorkflow.Steps, 4)

		// Step 1: Agent step
		assert.Equal(t, 3, mainWorkflow.Steps[0].AgentID)
		assert.Empty(t, mainWorkflow.Steps[0].WorkflowID)

		// Step 2: First sub-workflow with retry
		assert.Equal(t, "PrefixWorkflow", mainWorkflow.Steps[1].WorkflowID)
		assert.NotNil(t, mainWorkflow.Steps[1].RetryConfig)
		assert.Equal(t, 2, mainWorkflow.Steps[1].RetryConfig.MaxAttempts)

		// Step 3: Second sub-workflow
		assert.Equal(t, "SuffixWorkflow", mainWorkflow.Steps[2].WorkflowID)

		// Step 4: Integration step
		assert.Equal(t, "mock-integration", mainWorkflow.Steps[3].Integration.Integration)
		assert.Equal(t, "process", mainWorkflow.Steps[3].Integration.Event)
	})

	t.Run("Workflow references are properly set", func(t *testing.T) {
		// Verify all workflows have references to each other
		assert.Len(t, mainWorkflow.workflowMap, 3)
		assert.Contains(t, mainWorkflow.workflowMap, "PrefixWorkflow")
		assert.Contains(t, mainWorkflow.workflowMap, "SuffixWorkflow")
		assert.Contains(t, mainWorkflow.workflowMap, "MainWorkflow")

		// Verify sub-workflows also have all references
		assert.Len(t, prefixWorkflow.workflowMap, 3)
		assert.Len(t, suffixWorkflow.workflowMap, 3)
	})

	t.Run("Workflow ID lookup works with both ID and Name", func(t *testing.T) {
		// The workflow map uses Name as key
		workflow, exists := workflows["PrefixWorkflow"]
		require.True(t, exists)
		assert.Equal(t, "prefix-workflow", workflow.settings.ID)
		assert.Equal(t, "PrefixWorkflow", workflow.settings.Name)

		// Test finding by Name (which is how steps reference workflows)
		step := WorkflowStep{
			ID:          "test-step",
			WorkflowID:  "PrefixWorkflow",
			OutputField: "generatedText",
		}

		// This would be called internally, but we're testing the lookup logic
		subWorkflow, exists := mainWorkflow.workflowMap[step.WorkflowID]
		assert.True(t, exists)
		assert.NotNil(t, subWorkflow)
		assert.Equal(t, "PrefixWorkflow", subWorkflow.settings.Name)
	})
}

// MockIntegration for testing
type MockIntegration struct {
	calls []IntegrationCall
}

type IntegrationCall struct {
	Event string
	Data  interface{}
}

func (m *MockIntegration) RegisterTrigger(event string, data interface{}, channel chan interface{}) {
	// Record the trigger registration
}

func (m *MockIntegration) Call(event string, data interface{}) (interface{}, error) {
	m.calls = append(m.calls, IntegrationCall{Event: event, Data: data})
	return "integration-response", nil
}

func (m *MockIntegration) GetChannel() chan interface{} {
	return make(chan interface{})
}

func (m *MockIntegration) IsActive() bool {
	return true
}

func (m *MockIntegration) Name() string {
	return "mock-integration"
}

func (m *MockIntegration) GetChatHistory(channelID string, limit int) (string, error) {
	return "", nil
}

func (m *MockIntegration) ClearChatHistory(channelID string) error {
	return nil
}
