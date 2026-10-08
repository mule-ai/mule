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
		validTypes := []string{"agent", "wasm_module", "forge_conversation"}
		if !isValidEnum(step.StepType, validTypes) {
			errors = append(errors, ValidationError{
				Field:   "type",
				Message: "Step type must be one of agent, wasm_module, forge_conversation",
			})
		}
	}

	if step.StepType == "forge_conversation" {
		for _, err := range v.ValidateForgeConversationStep(step.Config) {
			errors = append(errors, err)
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

// ForgeConversationMode enumerates the address/content modes a
// forge_conversation step supports (Herd H3.2).
var ForgeConversationModes = []string{"message", "steer", "follow_up", "fork"}

// ValidateForgeConversationStep validates the config map of a
// forge_conversation workflow step (Herd H3.2).
//
//	{forge_url: string (required, http/https), api_key: string (required,
//	 the forge sk_forge_ key, inlined in config in v1), agent_id?:
//	 string, conversation_id?: string, mode?: "message"|"steer"|
//	 "follow_up"|"fork" (default "message"), content: string (required,
//	 a Go text/template rendered over the job input map), wait?: bool
//	 (default false), timeout_secs?: int (default 300, positive when
//	 set)}.
func (v *Validator) ValidateForgeConversationStep(config map[string]interface{}) ValidationErrors {
	var errors ValidationErrors

	if config == nil {
		errors = append(errors, ValidationError{
			Field:   "config",
			Message: "forge_conversation step requires a config object",
		})
		return errors
	}

	forgeURL := configString(config, "forge_url")
	if strings.TrimSpace(forgeURL) == "" {
		errors = append(errors, ValidationError{
			Field:   "config.forge_url",
			Message: "forge_url is required for forge_conversation steps",
		})
	} else if u, err := url.Parse(forgeURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errors = append(errors, ValidationError{
			Field:   "config.forge_url",
			Message: "forge_url must be an http or https URL",
		})
	}

	if apikey := configString(config, "api_key"); strings.TrimSpace(apikey) == "" {
		errors = append(errors, ValidationError{
			Field:   "config.api_key",
			Message: "api_key is required for forge_conversation steps (v1: the forge sk_forge_ key inlined in config)",
		})
	}

	if mode, ok := config["mode"]; ok {
		modeStr, isStr := mode.(string)
		if !isStr || !isValidEnum(modeStr, ForgeConversationModes) {
			errors = append(errors, ValidationError{
				Field:   "config.mode",
				Message: "mode must be one of: message, steer, follow_up, fork",
			})
		}
	}

	content := configString(config, "content")
	if strings.TrimSpace(content) == "" {
		errors = append(errors, ValidationError{
			Field:   "config.content",
			Message: "content is required for forge_conversation steps",
		})
	}

	if wait, ok := config["wait"]; ok {
		if _, isBool := wait.(bool); !isBool {
			errors = append(errors, ValidationError{
				Field:   "config.wait",
				Message: "wait must be a boolean",
			})
		}
	}

	if timeoutSecs, ok := config["timeout_secs"]; ok {
		if !configIntPositive(timeoutSecs) {
			errors = append(errors, ValidationError{
				Field:   "config.timeout_secs",
				Message: "timeout_secs must be a positive integer",
			})
		}
	}

	// Addressing: a step needs either a pinned conversation or an
	// agent to resolve one against. mode "fork" additionally needs
	// the agent (forks are created via POST /agents/:id/conversations).
	agentID := strings.TrimSpace(configString(config, "agent_id"))
	conversationID := strings.TrimSpace(configString(config, "conversation_id"))
	if conversationID == "" && agentID == "" {
		errors = append(errors, ValidationError{
			Field:   "config.agent_id",
			Message: "either agent_id or conversation_id is required for forge_conversation steps",
		})
	}
	if _, hasMode := config["mode"]; hasMode {
		if mode, isStr := config["mode"].(string); isStr && mode == "fork" && agentID == "" {
			errors = append(errors, ValidationError{
				Field:   "config.agent_id",
				Message: "mode fork requires agent_id (forks are created against an agent)",
			})
		}
	}

	return errors
}

// configString returns a config-map value as a string (zero value when
// absent or of the wrong type).
func configString(config map[string]interface{}, key string) string {
	s, _ := config[key].(string)
	return s
}

// configIntPositive reports whether a config-map value is a positive
// integer (JSON numbers decode as float64; the engine's parser accepts
// the same set).
func configIntPositive(value interface{}) bool {
	switch n := value.(type) {
	case float64:
		return n > 0 && n == float64(int64(n))
	case int:
		return n > 0
	case int64:
		return n > 0
	}
	return false
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
