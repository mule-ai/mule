package agent

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/mule-ai/mule/pkg/types"
	"github.com/mule-ai/mule/pkg/validation"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeepResearchWorkflowStructure(t *testing.T) {
	logger := logr.Discard()
	integrations := make(map[string]types.Integration)
	agentMap := map[int]*Agent{
		17: {id: 17, Name: "Planning"},
		18: {id: 18, Name: "Research"},
		21: {id: 21, Name: "PlanValidator"},
		22: {id: 22, Name: "ResearchValidator"},
	}

	// Create planning workflow with validation
	planningWorkflowSettings := WorkflowSettings{
		ID:   "workflow_planning_with_validation",
		Name: "PlanningWithValidation",
		Steps: []WorkflowStep{
			{
				ID:          "step_create_plan",
				AgentID:     17,
				OutputField: "generatedText",
			},
			{
				ID:          "step_validate_plan",
				AgentID:     21,
				OutputField: "generatedText",
			},
		},
		ValidationFunctions: []string{"planValidation"},
	}

	// Create research workflow with validation
	researchWorkflowSettings := WorkflowSettings{
		ID:   "workflow_research_with_validation",
		Name: "ResearchWithValidation",
		Steps: []WorkflowStep{
			{
				ID:          "step_execute_research",
				AgentID:     18,
				OutputField: "generatedText",
			},
			{
				ID:          "step_validate_research",
				AgentID:     22,
				OutputField: "generatedText",
			},
		},
		ValidationFunctions: []string{"researchValidation"},
	}

	// Create main deep research workflow
	deepResearchWorkflowSettings := WorkflowSettings{
		ID:   "workflow_deep_research",
		Name: "Deep Research",
		Steps: []WorkflowStep{
			{
				ID:         "step_planning_phase",
				WorkflowID: "PlanningWithValidation",
				RetryConfig: &RetryConfig{
					MaxAttempts: 3,
					DelayMs:     1000,
				},
				OutputField: "generatedText",
			},
			{
				ID:         "step_research_phase",
				WorkflowID: "ResearchWithValidation",
				RetryConfig: &RetryConfig{
					MaxAttempts: 3,
					DelayMs:     1500,
				},
				OutputField: "generatedText",
			},
		},
	}

	// Create workflow instances
	planningWorkflow := NewWorkflow(planningWorkflowSettings, agentMap, integrations, logger)
	researchWorkflow := NewWorkflow(researchWorkflowSettings, agentMap, integrations, logger)
	deepResearchWorkflow := NewWorkflow(deepResearchWorkflowSettings, agentMap, integrations, logger)

	// Set up workflow references
	workflows := map[string]*Workflow{
		"PlanningWithValidation": planningWorkflow,
		"ResearchWithValidation": researchWorkflow,
		"Deep Research":          deepResearchWorkflow,
	}

	for _, w := range workflows {
		w.SetWorkflowReferences(workflows)
	}

	t.Run("Planning workflow has correct structure", func(t *testing.T) {
		require.Len(t, planningWorkflow.Steps, 2)

		// First step: create plan
		assert.Equal(t, "step_create_plan", planningWorkflow.Steps[0].ID)
		assert.Equal(t, 17, planningWorkflow.Steps[0].AgentID)
		assert.Equal(t, "generatedText", planningWorkflow.Steps[0].OutputField)

		// Second step: validate plan
		assert.Equal(t, "step_validate_plan", planningWorkflow.Steps[1].ID)
		assert.Equal(t, 21, planningWorkflow.Steps[1].AgentID)
		assert.Equal(t, "generatedText", planningWorkflow.Steps[1].OutputField)

		// Has validation function
		require.Len(t, planningWorkflow.ValidationFunctions, 1)
		assert.Equal(t, "planValidation", planningWorkflow.ValidationFunctions[0])
	})

	t.Run("Research workflow has correct structure", func(t *testing.T) {
		require.Len(t, researchWorkflow.Steps, 2)

		// First step: execute research
		assert.Equal(t, "step_execute_research", researchWorkflow.Steps[0].ID)
		assert.Equal(t, 18, researchWorkflow.Steps[0].AgentID)
		assert.Equal(t, "generatedText", researchWorkflow.Steps[0].OutputField)

		// Second step: validate research
		assert.Equal(t, "step_validate_research", researchWorkflow.Steps[1].ID)
		assert.Equal(t, 22, researchWorkflow.Steps[1].AgentID)
		assert.Equal(t, "generatedText", researchWorkflow.Steps[1].OutputField)

		// Has validation function
		require.Len(t, researchWorkflow.ValidationFunctions, 1)
		assert.Equal(t, "researchValidation", researchWorkflow.ValidationFunctions[0])
	})

	t.Run("Deep research workflow composition", func(t *testing.T) {
		require.Len(t, deepResearchWorkflow.Steps, 2)

		// First step: planning phase (sub-workflow)
		step1 := deepResearchWorkflow.Steps[0]
		assert.Equal(t, "step_planning_phase", step1.ID)
		assert.Equal(t, "PlanningWithValidation", step1.WorkflowID)
		assert.Equal(t, 0, step1.AgentID) // No direct agent
		require.NotNil(t, step1.RetryConfig)
		assert.Equal(t, 3, step1.RetryConfig.MaxAttempts)
		assert.Equal(t, 1000, step1.RetryConfig.DelayMs)

		// Second step: research phase (sub-workflow)
		step2 := deepResearchWorkflow.Steps[1]
		assert.Equal(t, "step_research_phase", step2.ID)
		assert.Equal(t, "ResearchWithValidation", step2.WorkflowID)
		assert.Equal(t, 0, step2.AgentID) // No direct agent
		require.NotNil(t, step2.RetryConfig)
		assert.Equal(t, 3, step2.RetryConfig.MaxAttempts)
		assert.Equal(t, 1500, step2.RetryConfig.DelayMs)

		// Workflow references are set
		assert.Contains(t, deepResearchWorkflow.workflowMap, "PlanningWithValidation")
		assert.Contains(t, deepResearchWorkflow.workflowMap, "ResearchWithValidation")
	})

	t.Run("Workflow benefits are achieved", func(t *testing.T) {
		// Reusability: planning and research workflows can be used independently
		assert.NotEqual(t, planningWorkflow, deepResearchWorkflow)
		assert.NotEqual(t, researchWorkflow, deepResearchWorkflow)

		// Composability: main workflow is composed of sub-workflows with their own validation
		assert.Len(t, planningWorkflow.ValidationFunctions, 1)
		assert.Len(t, researchWorkflow.ValidationFunctions, 1)
		assert.Empty(t, deepResearchWorkflow.ValidationFunctions) // No validation at main level

		// Resiliency: sub-workflow steps have retry configuration
		for _, step := range deepResearchWorkflow.Steps {
			if step.WorkflowID != "" {
				assert.NotNil(t, step.RetryConfig, "Sub-workflow step should have retry config")
				assert.Greater(t, step.RetryConfig.MaxAttempts, 1, "Should have retry attempts")
			}
		}
	})
}

func TestDeepResearchWorkflowValidation(t *testing.T) {
	t.Run("Planning validation works correctly", func(t *testing.T) {
		// Test valid plan
		validPlan := "VALID: The plan is comprehensive and well-structured."
		result, err := validation.PlanValidation(validPlan)
		require.NoError(t, err)
		assert.Equal(t, validPlan, result)

		// Test invalid plan
		invalidPlan := "INVALID: The plan lacks specific research steps."
		result, err = validation.PlanValidation(invalidPlan)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plan validation failed")
		assert.Equal(t, invalidPlan, result)
	})

	t.Run("Research validation works correctly", func(t *testing.T) {
		// Test valid research
		validResearch := "VALID: The report thoroughly answers the original question."
		result, err := validation.ResearchValidation(validResearch)
		require.NoError(t, err)
		assert.Equal(t, validResearch, result)

		// Test invalid research
		invalidResearch := "INVALID: The report lacks citations and depth."
		result, err = validation.ResearchValidation(invalidResearch)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "research validation failed")
		assert.Equal(t, invalidResearch, result)
	})
}
