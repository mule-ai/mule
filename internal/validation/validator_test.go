package validation

import (
	"testing"

	"github.com/mule-ai/mule/internal/primitive"
	"github.com/stretchr/testify/assert"
)

func TestValidateProvider(t *testing.T) {
	tests := []struct {
		name         string
		provider     *primitive.Provider
		expectErrors int
	}{
		{
			name: "valid provider",
			provider: &primitive.Provider{
				Name:       "openai",
				APIBaseURL: "https://api.openai.com",
				APIKeyEnc:  "sk-test",
			},
			expectErrors: 0,
		},
		{
			name: "missing name",
			provider: &primitive.Provider{
				APIBaseURL: "https://api.openai.com",
				APIKeyEnc:  "sk-test",
			},
			expectErrors: 1,
		},
		{
			name: "missing API base URL",
			provider: &primitive.Provider{
				Name:      "openai",
				APIKeyEnc: "sk-test",
			},
			expectErrors: 1,
		},
		{
			name: "missing API key",
			provider: &primitive.Provider{
				Name:       "openai",
				APIBaseURL: "https://api.openai.com",
			},
			expectErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			errs := v.ValidateProvider(tt.provider)
			assert.Len(t, errs, tt.expectErrors)
		})
	}
}

func TestValidateTool(t *testing.T) {
	tests := []struct {
		name         string
		tool         *primitive.Tool
		expectErrors int
	}{
		{
			name: "valid tool",
			tool: &primitive.Tool{
				Name:        "weather",
				Description: "Get weather information",
				Metadata: map[string]interface{}{
					"tool_type": "http",
					"config": map[string]interface{}{
						"url": "https://api.weather.com",
					},
				},
			},
			expectErrors: 0,
		},
		{
			name: "missing name",
			tool: &primitive.Tool{
				Description: "Get weather information",
				Metadata: map[string]interface{}{
					"tool_type": "http",
					"config": map[string]interface{}{
						"url": "https://api.weather.com",
					},
				},
			},
			expectErrors: 1,
		},
		{
			name: "missing type",
			tool: &primitive.Tool{
				Name:        "weather",
				Description: "Get weather information",
				Metadata: map[string]interface{}{
					"config": map[string]interface{}{
						"url": "https://api.weather.com",
					},
				},
			},
			expectErrors: 1,
		},
		{
			name: "invalid type",
			tool: &primitive.Tool{
				Name:        "weather",
				Description: "Get weather information",
				Metadata: map[string]interface{}{
					"tool_type": "invalid",
					"config": map[string]interface{}{
						"url": "https://api.weather.com",
					},
				},
			},
			expectErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			errs := v.ValidateTool(tt.tool)
			assert.Len(t, errs, tt.expectErrors)
		})
	}
}

func TestValidateAgent(t *testing.T) {
	tests := []struct {
		name         string
		agent        *primitive.Agent
		expectErrors int
	}{
		{
			name: "valid agent",
			agent: &primitive.Agent{
				Name:         "assistant",
				ModelID:      "gpt-4",
				ProviderID:   "openai",
				SystemPrompt: "You are a helpful assistant",
			},
			expectErrors: 0,
		},
		{
			name: "missing name",
			agent: &primitive.Agent{
				ModelID:      "gpt-4",
				ProviderID:   "openai",
				SystemPrompt: "You are a helpful assistant",
			},
			expectErrors: 1,
		},
		{
			name: "missing model",
			agent: &primitive.Agent{
				Name:         "assistant",
				ProviderID:   "openai",
				SystemPrompt: "You are a helpful assistant",
			},
			expectErrors: 1,
		},
		{
			name: "missing provider ID",
			agent: &primitive.Agent{
				Name:         "assistant",
				ModelID:      "gpt-4",
				SystemPrompt: "You are a helpful assistant",
			},
			expectErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			errs := v.ValidateAgent(tt.agent)
			assert.Len(t, errs, tt.expectErrors)
		})
	}
}

func TestValidateWorkflow(t *testing.T) {
	tests := []struct {
		name         string
		workflow     *primitive.Workflow
		expectErrors int
	}{
		{
			name: "valid workflow",
			workflow: &primitive.Workflow{
				Name:        "data-processing",
				Description: "Process data pipeline",
			},
			expectErrors: 0,
		},
		{
			name: "missing name",
			workflow: &primitive.Workflow{
				Description: "Process data pipeline",
			},
			expectErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			errs := v.ValidateWorkflow(tt.workflow)
			assert.Len(t, errs, tt.expectErrors)
		})
	}
}

func TestValidateWorkflowStep(t *testing.T) {
	tests := []struct {
		name         string
		step         *primitive.WorkflowStep
		expectErrors int
	}{
		{
			name: "valid agent step",
			step: &primitive.WorkflowStep{
				ID:         "step1",
				WorkflowID: "workflow1",
				StepOrder:  1,
				StepType:   "agent",
				AgentID:    stringPtr("agent1"),
				Config:     map[string]interface{}{},
			},
			expectErrors: 0,
		},
		{
			name: "valid wasm step",
			step: &primitive.WorkflowStep{
				ID:           "step2",
				WorkflowID:   "workflow1",
				StepOrder:    2,
				StepType:     "wasm_module",
				WasmModuleID: stringPtr("module1"),
				Config:       map[string]interface{}{},
			},
			expectErrors: 0,
		},
		{
			name: "missing ID",
			step: &primitive.WorkflowStep{
				WorkflowID: "workflow1",
				StepOrder:  1,
				StepType:   "agent",
				Config:     map[string]interface{}{},
			},
			expectErrors: 1,
		},
		{
			name: "missing workflow ID",
			step: &primitive.WorkflowStep{
				ID:        "step1",
				StepOrder: 1,
				StepType:  "agent",
				Config:    map[string]interface{}{},
			},
			expectErrors: 2, // missing workflow_id AND agent_id
		},
		{
			name: "invalid step type",
			step: &primitive.WorkflowStep{
				ID:         "step1",
				WorkflowID: "workflow1",
				StepOrder:  1,
				StepType:   "invalid",
				Config:     map[string]interface{}{},
			},
			expectErrors: 1,
		},
		{
			name: "agent step missing agent ID",
			step: &primitive.WorkflowStep{
				ID:         "step1",
				WorkflowID: "workflow1",
				StepOrder:  1,
				StepType:   "agent",
				Config:     map[string]interface{}{},
			},
			expectErrors: 1,
		},
		{
			name: "wasm step missing module ID",
			step: &primitive.WorkflowStep{
				ID:         "step2",
				WorkflowID: "workflow1",
				StepOrder:  2,
				StepType:   "wasm_module",
				Config:     map[string]interface{}{},
			},
			expectErrors: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			errs := v.ValidateWorkflowStep(tt.step)
			assert.Len(t, errs, tt.expectErrors)
		})
	}
}

func stringPtr(s string) *string {
	return &s
}
