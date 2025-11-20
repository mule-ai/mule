package validation

import (
	"testing"

	"github.com/mule-ai/mule/internal/primitive"
)

func TestValidateProvider(t *testing.T) {
	tests := []struct {
		name     string
		provider *primitive.Provider
		wantErr  bool
	}{
		{
			name: "valid provider",
			provider: &primitive.Provider{
				Name:       "openai",
				APIBaseURL: "https://api.openai.com",
				APIKeyEnc:  "sk-test",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			provider: &primitive.Provider{
				APIBaseURL: "https://api.openai.com",
				APIKeyEnc:  "sk-test",
			},
			wantErr: true,
		},
		{
			name: "missing API base URL",
			provider: &primitive.Provider{
				Name:      "openai",
				APIKeyEnc: "sk-test",
			},
			wantErr: true,
		},
		{
			name: "missing API key",
			provider: &primitive.Provider{
				Name:       "openai",
				APIBaseURL: "https://api.openai.com",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			err := v.ValidateProvider(tt.provider)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateProvider() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTool(t *testing.T) {
	tests := []struct {
		name    string
		tool    *primitive.Tool
		wantErr bool
	}{
		{
			name: "valid tool",
			tool: &primitive.Tool{
				Name:        "weather",
				Type:        "http",
				Description: "Get weather information",
				Config: map[string]interface{}{
					"url": "https://api.weather.com",
				},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			tool: &primitive.Tool{
				Type:        "http",
				Description: "Get weather information",
				Config: map[string]interface{}{
					"url": "https://api.weather.com",
				},
			},
			wantErr: true,
		},
		{
			name: "missing type",
			tool: &primitive.Tool{
				Name:        "weather",
				Description: "Get weather information",
				Config: map[string]interface{}{
					"url": "https://api.weather.com",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid type",
			tool: &primitive.Tool{
				Name:        "weather",
				Type:        "invalid",
				Description: "Get weather information",
				Config: map[string]interface{}{
					"url": "https://api.weather.com",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			err := v.ValidateTool(tt.tool)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTool() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateAgent(t *testing.T) {
	tests := []struct {
		name    string
		agent   *primitive.Agent
		wantErr bool
	}{
		{
			name: "valid agent",
			agent: &primitive.Agent{
				Name:         "assistant",
				ModelID:      "gpt-4",
				ProviderID:   "openai",
				SystemPrompt: "You are a helpful assistant",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			agent: &primitive.Agent{
				ModelID:      "gpt-4",
				ProviderID:   "openai",
				SystemPrompt: "You are a helpful assistant",
			},
			wantErr: true,
		},
		{
			name: "missing model",
			agent: &primitive.Agent{
				Name:         "assistant",
				ProviderID:   "openai",
				SystemPrompt: "You are a helpful assistant",
			},
			wantErr: true,
		},
		{
			name: "missing provider ID",
			agent: &primitive.Agent{
				Name:         "assistant",
				ModelID:      "gpt-4",
				SystemPrompt: "You are a helpful assistant",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			err := v.ValidateAgent(tt.agent)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAgent() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateWorkflow(t *testing.T) {
	tests := []struct {
		name     string
		workflow *primitive.Workflow
		wantErr  bool
	}{
		{
			name: "valid workflow",
			workflow: &primitive.Workflow{
				Name:        "data-processing",
				Description: "Process data pipeline",
			},
			wantErr: false,
		},
		{
			name: "missing name",
			workflow: &primitive.Workflow{
				Description: "Process data pipeline",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			err := v.ValidateWorkflow(tt.workflow)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWorkflow() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateWorkflowStep(t *testing.T) {
	tests := []struct {
		name    string
		step    *primitive.WorkflowStep
		wantErr bool
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
			wantErr: false,
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
			wantErr: false,
		},
		{
			name: "missing ID",
			step: &primitive.WorkflowStep{
				WorkflowID: "workflow1",
				StepOrder:  1,
				StepType:   "agent",
				Config:     map[string]interface{}{},
			},
			wantErr: true,
		},
		{
			name: "missing workflow ID",
			step: &primitive.WorkflowStep{
				ID:        "step1",
				StepOrder: 1,
				StepType:  "agent",
				Config:    map[string]interface{}{},
			},
			wantErr: true,
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
			wantErr: true,
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
			wantErr: true,
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
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := NewValidator()
			err := v.ValidateWorkflowStep(tt.step)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateWorkflowStep() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func stringPtr(s string) *string {
	return &s
}
