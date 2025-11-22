package validation

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/mule-ai/mule/internal/primitive"
)

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Error implements the error interface
func (v ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", v.Field, v.Message)
}

// ValidationErrors represents multiple validation errors
type ValidationErrors []ValidationError

// Error implements the error interface
func (ve ValidationErrors) Error() string {
	var messages []string
	for _, err := range ve {
		messages = append(messages, err.Error())
	}
	return strings.Join(messages, "; ")
}

// Validator provides validation functions
type Validator struct{}

// NewValidator creates a new validator
func NewValidator() *Validator {
	return &Validator{}
}

// ValidateProvider validates a provider
func (v *Validator) ValidateProvider(provider *primitive.Provider) ValidationErrors {
	var errors ValidationErrors

	if strings.TrimSpace(provider.Name) == "" {
		errors = append(errors, ValidationError{
			Field:   "name",
			Message: "Name is required",
		})
	}

	if strings.TrimSpace(provider.APIBaseURL) == "" {
		errors = append(errors, ValidationError{
			Field:   "api_base_url",
			Message: "API base URL is required",
		})
	} else {
		if _, err := url.Parse(provider.APIBaseURL); err != nil {
			errors = append(errors, ValidationError{
				Field:   "api_base_url",
				Message: "API base URL must be a valid URL",
			})
		}
	}

	if strings.TrimSpace(provider.APIKeyEnc) == "" {
		errors = append(errors, ValidationError{
			Field:   "api_key_encrypted",
			Message: "API key is required",
		})
	}

	return errors
}

// ValidateTool validates a tool
func (v *Validator) ValidateTool(tool *primitive.Tool) ValidationErrors {
	var errors ValidationErrors

	if strings.TrimSpace(tool.Name) == "" {
		errors = append(errors, ValidationError{
			Field:   "name",
			Message: "Name is required",
		})
	}

	if tool.Metadata == nil {
		errors = append(errors, ValidationError{
			Field:   "metadata",
			Message: "Metadata is required",
		})
	} else {
		// Extract tool_type from metadata
		toolType, ok := tool.Metadata["tool_type"].(string)
		if !ok || strings.TrimSpace(toolType) == "" {
			errors = append(errors, ValidationError{
				Field:   "metadata.tool_type",
				Message: "Tool type is required in metadata",
			})
		} else {
			validTypes := []string{"http", "database", "memory", "filesystem"}
			isValid := false
			for _, validType := range validTypes {
				if toolType == validType {
					isValid = true
					break
				}
			}
			if !isValid {
				errors = append(errors, ValidationError{
					Field:   "metadata.tool_type",
					Message: "Tool type must be one of: http, database, memory, filesystem",
				})
			}
		}
	}

	return errors
}

// ValidateAgent validates an agent
func (v *Validator) ValidateAgent(agent *primitive.Agent) ValidationErrors {
	var errors ValidationErrors

	if strings.TrimSpace(agent.Name) == "" {
		errors = append(errors, ValidationError{
			Field:   "name",
			Message: "Name is required",
		})
	}

	if strings.TrimSpace(agent.ProviderID) == "" {
		errors = append(errors, ValidationError{
			Field:   "provider_id",
			Message: "Provider ID is required",
		})
	}

	if strings.TrimSpace(agent.ModelID) == "" {
		errors = append(errors, ValidationError{
			Field:   "model_id",
			Message: "Model ID is required",
		})
	}

	if strings.TrimSpace(agent.SystemPrompt) == "" {
		errors = append(errors, ValidationError{
			Field:   "system_prompt",
			Message: "System prompt is required",
		})
	}

	return errors
}

// ValidateWorkflow validates a workflow
func (v *Validator) ValidateWorkflow(workflow *primitive.Workflow) ValidationErrors {
	var errors ValidationErrors

	if strings.TrimSpace(workflow.Name) == "" {
		errors = append(errors, ValidationError{
			Field:   "name",
			Message: "Name is required",
		})
	}

	return errors
}

// ValidateWorkflowStep validates a workflow step
func (v *Validator) ValidateWorkflowStep(step *primitive.WorkflowStep) ValidationErrors {
	var errors ValidationErrors

	if strings.TrimSpace(step.WorkflowID) == "" {
		errors = append(errors, ValidationError{
			Field:   "workflow_id",
			Message: "Workflow ID is required",
		})
	}

	if step.StepOrder < 0 {
		errors = append(errors, ValidationError{
			Field:   "step_order",
			Message: "Step order must be non-negative",
		})
	}

	if strings.TrimSpace(step.StepType) == "" {
		errors = append(errors, ValidationError{
			Field:   "type",
			Message: "Step type is required",
		})
	} else {
		validTypes := []string{"agent", "wasm_module"}
		isValid := false
		for _, validType := range validTypes {
			if step.StepType == validType {
				isValid = true
				break
			}
		}
		if !isValid {
			errors = append(errors, ValidationError{
				Field:   "type",
				Message: "Step type must be either agent or wasm_module",
			})
		}
	}

	if step.StepType == "agent" && (step.AgentID == nil || strings.TrimSpace(*step.AgentID) == "") {
		errors = append(errors, ValidationError{
			Field:   "agent_id",
			Message: "Agent ID is required for agent steps",
		})
	}

	if step.StepType == "wasm_module" && (step.WasmModuleID == nil || strings.TrimSpace(*step.WasmModuleID) == "") {
		errors = append(errors, ValidationError{
			Field:   "wasm_module_id",
			Message: "WASM module ID is required for WASM steps",
		})
	}

	return errors
}

// ValidateChatCompletionRequest validates a chat completion request
func (v *Validator) ValidateChatCompletionRequest(model string, messages []map[string]interface{}) ValidationErrors {
	var errors ValidationErrors

	if strings.TrimSpace(model) == "" {
		errors = append(errors, ValidationError{
			Field:   "model",
			Message: "Model is required",
		})
	} else {
		// Check if model starts with agent/, workflow/, or async/workflow/
		if !strings.HasPrefix(model, "agent/") && !strings.HasPrefix(model, "workflow/") && !strings.HasPrefix(model, "async/workflow/") {
			errors = append(errors, ValidationError{
				Field:   "model",
				Message: "Model must start with 'agent/', 'workflow/', or 'async/workflow/'",
			})
		}
	}

	if len(messages) == 0 {
		errors = append(errors, ValidationError{
			Field:   "messages",
			Message: "At least one message is required",
		})
	} else {
		for i, msg := range messages {
			if role, ok := msg["role"].(string); !ok || strings.TrimSpace(role) == "" {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("messages[%d].role", i),
					Message: "Message role is required",
				})
			}

			if content, ok := msg["content"].(string); !ok || strings.TrimSpace(content) == "" {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("messages[%d].content", i),
					Message: "Message content is required",
				})
			}
		}
	}

	return errors
}

// ValidateID validates a UUID or string ID
func (v *Validator) ValidateID(id string, fieldName string) ValidationErrors {
	var errors ValidationErrors

	if strings.TrimSpace(id) == "" {
		errors = append(errors, ValidationError{
			Field:   fieldName,
			Message: "ID is required",
		})
	} else {
		// Basic UUID format validation (simplified)
		uuidRegex := regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
		if !uuidRegex.MatchString(id) {
			// Allow non-UUID IDs for now, but could be stricter in future
			_ = fmt.Sprintf("ID %s is not a valid UUID format", id)
		}
	}

	return errors
}
