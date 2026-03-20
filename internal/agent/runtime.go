package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/mule-ai/mule/internal/agent/pirc"
	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/internal/tools"
	"github.com/mule-ai/mule/pkg/job"
)

// Runtime handles agent execution using pi RPC
type Runtime struct {
	store          primitive.PrimitiveStore
	workflowEngine WorkflowEngine
	jobStore       job.JobStore
	toolRegistry   *tools.Registry
}

// NewRuntime creates a new agent runtime
func NewRuntime(store primitive.PrimitiveStore, jobStore job.JobStore) *Runtime {
	// Initialize the new tool registry with configuration support
	toolRegistry, err := tools.NewRegistryWithConfig(store)
	if err != nil {
		// Fall back to the old registry if the new one fails to initialize
		log.Printf("Failed to initialize new tool registry: %v, falling back to old registry", err)
		toolRegistry = tools.NewRegistry()
	}

	return &Runtime{
		store:        store,
		jobStore:     jobStore,
		toolRegistry: toolRegistry,
	}
}

// SetWorkflowEngine sets the workflow engine for the runtime
func (r *Runtime) SetWorkflowEngine(engine WorkflowEngine) {
	r.workflowEngine = engine
}

// ReinitializeMemoryTool reinitializes the memory tool when configuration changes
func (r *Runtime) ReinitializeMemoryTool() error {
	if r.toolRegistry != nil {
		return r.toolRegistry.ReinitializeMemoryTool()
	}
	return fmt.Errorf("tool registry not initialized")
}

// ChatCompletionRequest represents the OpenAI-compatible request
type ChatCompletionRequest struct {
	Model            string                  `json:"model"`
	Messages         []ChatCompletionMessage `json:"messages"`
	Stream           bool                    `json:"stream,omitempty"`
	WorkingDirectory string                  `json:"working_directory,omitempty"`
}

// ChatCompletionMessage represents a message in the chat
type ChatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionResponse represents the OpenAI-compatible response
type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
	Usage   ChatCompletionUsage    `json:"usage"`
}

// ChatCompletionChoice represents a choice in the response
type ChatCompletionChoice struct {
	Index        int                   `json:"index"`
	Message      ChatCompletionMessage `json:"message"`
	FinishReason string                `json:"finish_reason"`
}

// ChatCompletionUsage represents token usage
type ChatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// AsyncJobResponse represents an asynchronous job response
type AsyncJobResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

// ExecuteAgent executes an agent with the given request
func (r *Runtime) ExecuteAgent(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	// Call ExecuteAgentWithWorkingDir with empty working directory for backward compatibility
	return r.ExecuteAgentWithWorkingDir(ctx, req, "")
}

// ExecuteAgentWithWorkingDir executes an agent with the given request and working directory
func (r *Runtime) ExecuteAgentWithWorkingDir(ctx context.Context, req *ChatCompletionRequest, workingDir string) (*ChatCompletionResponse, error) {
	// Parse model name to extract agent name
	agentName := strings.TrimPrefix(req.Model, "agent/")

	// Find the agent by name
	agents, err := r.store.ListAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}

	var targetAgent *primitive.Agent
	for _, agent := range agents {
		if strings.ToLower(agent.Name) == agentName {
			targetAgent = agent
			break
		}
	}

	if targetAgent == nil {
		return nil, fmt.Errorf("agent '%s' not found", agentName)
	}

	// Concatenate messages for the prompt
	var prompt strings.Builder
	for _, msg := range req.Messages {
		if msg.Role == "user" {
			prompt.WriteString(msg.Content + "\n")
		}
	}

	// Use pi for agent execution
	return r.executeWithPI(ctx, targetAgent, prompt.String(), workingDir)
}

// executeWithPI executes the agent using pi RPC
func (r *Runtime) executeWithPI(ctx context.Context, agent *primitive.Agent, prompt string, workingDir string) (*ChatCompletionResponse, error) {
	// Get provider information for API key and provider name
	var apiKey string
	var providerName string

	if agent.ProviderID != "" {
		provider, err := r.store.GetProvider(ctx, agent.ProviderID)
		if err != nil {
			log.Printf("Warning: failed to get provider: %v, proceeding without API key", err)
		} else {
			apiKey = string(provider.APIKeyEnc)
			// Use the provider name as configured by the user
			providerName = provider.Name
		}
	}

	// Get skills for this agent
	skills, err := r.store.GetAgentSkills(ctx, agent.ID)
	if err != nil {
		log.Printf("Warning: failed to get agent skills: %v", err)
	}

	// Extract skill paths
	var skillPaths []string
	for _, skill := range skills {
		if skill.Enabled {
			skillPaths = append(skillPaths, skill.Path)
		}
	}

	// Get thinking level from pi_config
	thinkingLevel := "medium" // Default
	if agent.PIConfig != nil {
		if level, ok := agent.PIConfig["thinking_level"].(string); ok && level != "" {
			thinkingLevel = level
		}
	}

	// Build pi config
	cfg := pirc.Config{
		Provider:         providerName,
		ModelID:          agent.ModelID,
		APIKey:           apiKey,
		SystemPrompt:     agent.SystemPrompt,
		ThinkingLevel:    thinkingLevel,
		Skills:           skillPaths,
		WorkingDirectory: workingDir,
		Timeout:          5 * time.Minute, // Default timeout
	}

	// Create and start the pi bridge
	bridge := pirc.NewBridge(cfg)
	if err := bridge.Start(); err != nil {
		return nil, fmt.Errorf("failed to start pi: %w", err)
	}

	// Ensure bridge is stopped when done
	defer func() {
		if err := bridge.Stop(); err != nil {
			log.Printf("Error stopping pi bridge: %v", err)
		}
	}()

	// Send the prompt
	if err := bridge.Prompt(ctx, prompt); err != nil {
		return nil, fmt.Errorf("failed to send prompt to pi: %w", err)
	}

	// Collect events and build response
	var responseText string
	timeout := time.After(cfg.Timeout)

	// Use a labeled break to exit when agent finishes
AgentLoop:
	for {
		select {
		case <-ctx.Done():
			// Try to abort gracefully first
			if err := bridge.Abort(ctx); err != nil {
				log.Printf("failed to abort bridge: %v", err)
			}
			return nil, fmt.Errorf("agent execution cancelled: %w", ctx.Err())
		case <-timeout:
			if err := bridge.Abort(ctx); err != nil {
				log.Printf("failed to abort bridge: %v", err)
			}
			return nil, fmt.Errorf("agent execution timed out after %v", cfg.Timeout)
		case event := <-bridge.Events():
			// Only extract response from agent_end - ignore intermediate events
			// to avoid duplicate content
			switch event.Type {
			case "agent_end":
				// Extract text from messages array in the event
				// Use Messages field (plural) which contains the full messages array
				msgData := event.Messages
				if len(msgData) > 0 {
					// The JSON is an array of messages like [{"role":"user","content":[...]},{"role":"assistant","content":[{...}]}]
					// NOT an object with a "messages" field
					var messages []struct {
						Role    string `json:"role"`
						Content []struct {
							Type string `json:"type"`
							Text string `json:"text"`
						} `json:"content"`
					}
					if err := json.Unmarshal(msgData, &messages); err == nil {
						for _, m := range messages {
							if m.Role == "assistant" {
								for _, c := range m.Content {
									if c.Type == "text" {
										responseText += c.Text
									}
								}
							}
						}
					}
				}
				// Agent has finished - we can break out and return the response
				break AgentLoop
			case "error":
				// Error occurred
				var errMsg struct {
					Error string `json:"error"`
				}
				if len(event.Message) > 0 {
					if err := json.Unmarshal(event.Message, &errMsg); err != nil {
						log.Printf("failed to parse error message: %v", err)
					}
				}
				if errMsg.Error != "" {
					return nil, fmt.Errorf("pi error: %s", errMsg.Error)
				}
			default:
				// Ignore other events for now - we only care about agent_end
			}
		case err := <-bridge.Errors():
			return nil, fmt.Errorf("pi process error: %w", err)
		}

		// Check if bridge is still running
		if !bridge.IsRunning() {
			break
		}

		// If we have response text and bridge is still running, wait a bit more
		// for the final response (since events come in sequence)
		if responseText != "" && !bridge.IsRunning() {
			break
		}
	}

	// Create OpenAI-compatible response
	chatResp := &ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   fmt.Sprintf("agent/%s", strings.ToLower(agent.Name)),
		Choices: []ChatCompletionChoice{
			{
				Index: 0,
				Message: ChatCompletionMessage{
					Role:    "assistant",
					Content: responseText,
				},
				FinishReason: "stop",
			},
		},
		Usage: ChatCompletionUsage{
			PromptTokens:     estimateTokens(prompt),
			CompletionTokens: estimateTokens(responseText),
			TotalTokens:      estimateTokens(prompt) + estimateTokens(responseText),
		},
	}

	return chatResp, nil
}

// estimateTokens provides a rough token estimation (in real implementation, use proper tokenizer)
func estimateTokens(text string) int {
	// Rough estimation: ~4 characters per token
	return len(text) / 4
}

// ExecuteWorkflow submits a workflow for execution and returns the job
func (r *Runtime) ExecuteWorkflow(ctx context.Context, req *ChatCompletionRequest) (*job.Job, error) {
	// Call ExecuteWorkflowWithWorkingDir with empty working directory for backward compatibility
	return r.ExecuteWorkflowWithWorkingDir(ctx, req, "")
}

// ExecuteWorkflowWithWorkingDir submits a workflow for execution with a specified working directory and returns the job
func (r *Runtime) ExecuteWorkflowWithWorkingDir(ctx context.Context, req *ChatCompletionRequest, workingDir string) (*job.Job, error) {
	// Parse model name to extract workflow name
	// Handle both "workflow/" and "async/workflow/" prefixes
	workflowName := req.Model
	if strings.HasPrefix(workflowName, "async/workflow/") {
		workflowName = strings.TrimPrefix(workflowName, "async/workflow/")
	} else if strings.HasPrefix(workflowName, "workflow/") {
		workflowName = strings.TrimPrefix(workflowName, "workflow/")
	}

	// Find the workflow by name
	workflows, err := r.store.ListWorkflows(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}

	var targetWorkflow *primitive.Workflow
	for _, workflow := range workflows {
		if strings.ToLower(workflow.Name) == workflowName {
			targetWorkflow = workflow
			break
		}
	}

	if targetWorkflow == nil {
		return nil, fmt.Errorf("workflow '%s' not found", workflowName)
	}

	// Concatenate messages for input data
	var prompt strings.Builder
	for _, msg := range req.Messages {
		prompt.WriteString(msg.Content + "\n")
	}

	// Prepare input data
	inputData := map[string]interface{}{
		"prompt": prompt.String(),
	}

	// Check if workflow engine is available
	if r.workflowEngine == nil {
		return nil, fmt.Errorf("workflow engine not available")
	}

	// Submit job to workflow engine with working directory
	job, err := r.workflowEngine.SubmitJob(ctx, targetWorkflow.ID, inputData)
	if err != nil {
		return nil, fmt.Errorf("failed to submit job: %w", err)
	}

	// If a working directory was specified, update the job with it
	if workingDir != "" {
		job.WorkingDirectory = workingDir
		if err := r.jobStore.UpdateJob(job); err != nil {
			// Log the error but don't fail the job creation
			log.Printf("Warning: failed to update job with working directory: %v", err)
		}
	}

	return job, nil
}
