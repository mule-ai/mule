package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"

	"github.com/mule-ai/mule/internal/primitive"
)

// Runtime handles agent execution using Google ADK
type Runtime struct {
	store primitive.PrimitiveStore
}

// NewRuntime creates a new agent runtime
func NewRuntime(store primitive.PrimitiveStore) *Runtime {
	return &Runtime{
		store: store,
	}
}

// ChatCompletionRequest represents the OpenAI-compatible request
type ChatCompletionRequest struct {
	Model    string                  `json:"model"`
	Messages []ChatCompletionMessage `json:"messages"`
	Stream   bool                    `json:"stream,omitempty"`
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

	// Get provider information
	provider, err := r.store.GetProvider(ctx, targetAgent.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}

	// Concatenate messages for the prompt
	var prompt strings.Builder
	for _, msg := range req.Messages {
		if msg.Role == "user" {
			prompt.WriteString(msg.Content + "\n")
		}
	}

	// Execute using Google ADK
	return r.executeWithGoogleADK(ctx, targetAgent, provider, prompt.String())
}

// executeWithGoogleADK executes the agent using Google's Generative AI
func (r *Runtime) executeWithGoogleADK(ctx context.Context, agent *primitive.Agent, provider *primitive.Provider, prompt string) (*ChatCompletionResponse, error) {
	// Create client with API key
	client, err := genai.NewClient(ctx, option.WithAPIKey(string(provider.APIKeyEnc)))
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}
	defer client.Close()

	// Get the model - use a default model for now, in future this should come from agent config
	model := client.GenerativeModel("gemini-1.5-flash")

	// Set system prompt if provided
	if agent.SystemPrompt != "" {
		model.SystemInstruction = genai.NewUserContent(genai.Text(agent.SystemPrompt))
	}

	// Generate content
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}

	if len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("no response generated")
	}

	// Extract the response text
	var responseText string
	if resp.Candidates[0].Content != nil {
		for _, part := range resp.Candidates[0].Content.Parts {
			if txt, ok := part.(genai.Text); ok {
				responseText += string(txt)
			}
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

// ExecuteWorkflow executes a workflow with the given request
func (r *Runtime) ExecuteWorkflow(ctx context.Context, req *ChatCompletionRequest) (*AsyncJobResponse, error) {
	// Parse model name to extract workflow name
	workflowName := strings.TrimPrefix(req.Model, "workflow/")

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

	// Create job through workflow engine (this would be injected)
	// For now, return a placeholder async response
	jobID := fmt.Sprintf("job-%d", time.Now().Unix())

	return &AsyncJobResponse{
		ID:      jobID,
		Object:  "async.job",
		Status:  "queued",
		Message: "The workflow has been started",
	}, nil
}
