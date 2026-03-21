package validation

import (
	"context"
	"fmt"
	"net/url"
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

// addRequiredStringError adds a validation error if the string is empty or whitespace-only
func addRequiredStringError(errors *ValidationErrors, fieldName string, value string) {
	if strings.TrimSpace(value) == "" {
		*errors = append(*errors, ValidationError{
			Field:   fieldName,
			Message: fmt.Sprintf("%s is required", fieldName),
		})
	}
}

// addInvalidStringError adds a validation error for invalid string values
func addInvalidStringError(errors *ValidationErrors, fieldName string, message string) {
	*errors = append(*errors, ValidationError{
		Field:   fieldName,
		Message: message,
	})
}

// isValidEnum checks if a value is in a list of valid values
func isValidEnum(value string, validValues []string) bool {
	for _, v := range validValues {
		if value == v {
			return true
		}
	}
	return false
}

// ValidateProvider validates a provider
func (v *Validator) ValidateProvider(provider *primitive.Provider) ValidationErrors {
	var errors ValidationErrors

	addRequiredStringError(&errors, "name", provider.Name)
	addRequiredStringError(&errors, "api_base_url", provider.APIBaseURL)

	// Validate URL format if provided
	if strings.TrimSpace(provider.APIBaseURL) != "" {
		if _, err := url.Parse(provider.APIBaseURL); err != nil {
			addInvalidStringError(&errors, "api_base_url", "API base URL must be a valid URL")
		}
	}

	addRequiredStringError(&errors, "api_key_encrypted", provider.APIKeyEnc)

	return errors
}

// ValidateTool validates a tool
func (v *Validator) ValidateTool(tool *primitive.Tool) ValidationErrors {
	var errors ValidationErrors

	addRequiredStringError(&errors, "name", tool.Name)

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
			if !isValidEnum(toolType, validTypes) {
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

	addRequiredStringError(&errors, "name", agent.Name)
	addRequiredStringError(&errors, "provider_id", agent.ProviderID)
	addRequiredStringError(&errors, "model_id", agent.ModelID)
	addRequiredStringError(&errors, "system_prompt", agent.SystemPrompt)

	return errors
}

// ValidateWorkflow validates a workflow
func (v *Validator) ValidateWorkflow(workflow *primitive.Workflow) ValidationErrors {
	var errors ValidationErrors

	addRequiredStringError(&errors, "name", workflow.Name)

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
		if !isValidEnum(step.StepType, validTypes) {
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

	addRequiredStringError(&errors, fieldName, id)

	// Note: UUID format validation is intentionally lenient - we allow non-UUID IDs
	// since the database may use other ID formats in some cases

	return errors
}

// ValidateSkill validates a skill
func (v *Validator) ValidateSkill(skill *primitive.Skill) ValidationErrors {
	var errors ValidationErrors

	addRequiredStringError(&errors, "name", skill.Name)
	addRequiredStringError(&errors, "path", skill.Path)

	return errors
}

// ValidateSkillIDs validates that skill IDs exist in the database
func (v *Validator) ValidateSkillIDs(ctx context.Context, store primitive.PrimitiveStore, skillIDs []string) ValidationErrors {
	var errors ValidationErrors

	for i, skillID := range skillIDs {
		if strings.TrimSpace(skillID) == "" {
			errors = append(errors, ValidationError{
				Field:   fmt.Sprintf("skill_ids[%d]", i),
				Message: "Skill ID cannot be empty",
			})
			continue
		}

		// Check if skill exists
		_, err := store.GetSkill(ctx, skillID)
		if err != nil {
			if err == primitive.ErrNotFound {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("skill_ids[%d]", i),
					Message: fmt.Sprintf("Skill not found: %s", skillID),
				})
			} else {
				errors = append(errors, ValidationError{
					Field:   fmt.Sprintf("skill_ids[%d]", i),
					Message: fmt.Sprintf("Failed to validate skill: %s", skillID),
				})
			}
		}
	}

	return errors
}
