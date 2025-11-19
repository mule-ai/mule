package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"google.golang.org/adk/model"
	"google.golang.org/genai"

	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/internal/provider"
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

	// Determine which execution method to use based on provider configuration
	fmt.Printf("DEBUG: Provider APIBaseURL = '%s'\n", provider.APIBaseURL)
	fmt.Printf("DEBUG: Contains googleapis.com = %v\n", strings.Contains(provider.APIBaseURL, "googleapis.com"))
	fmt.Printf("DEBUG: Is empty = %v\n", provider.APIBaseURL == "")

	if provider.APIBaseURL != "" && !strings.Contains(provider.APIBaseURL, "googleapis.com") {
		// Use custom LLM provider for non-Google endpoints
		fmt.Printf("DEBUG: Routing to executeWithCustomLLM\n")
		return r.executeWithCustomLLM(ctx, targetAgent, provider, req.Messages)
	} else {
		// Use Google ADK for Google endpoints
		fmt.Printf("DEBUG: Routing to executeWithGoogleADK\n")
		return r.executeWithGoogleADK(ctx, targetAgent, provider, prompt.String())
	}
}

// executeWithGoogleADK executes the agent using Google's Generative AI
func (r *Runtime) executeWithGoogleADK(ctx context.Context, agent *primitive.Agent, provider *primitive.Provider, prompt string) (*ChatCompletionResponse, error) {
	// Create client config
	config := &genai.ClientConfig{
		APIKey: string(provider.APIKeyEnc),
	}

	// If a custom endpoint is provided, use it
	if provider.APIBaseURL != "" {
		config.HTTPOptions = genai.HTTPOptions{
			BaseURL: provider.APIBaseURL,
		}
	}

	// Log the endpoint being used for debugging
	fmt.Printf("Creating genai client with endpoint: %s and API key: %s\n", provider.APIBaseURL, string(provider.APIKeyEnc))

	client, err := genai.NewClient(ctx, config)
	if err != nil {
		fmt.Printf("Failed to create genai client: %v\n", err)
		return nil, fmt.Errorf("failed to create genai client: %w", err)
	}

	// Get the model - use the model from agent config if available, otherwise use a default
	modelName := "gemini-1.5-flash"
	if agent.ModelID != "" {
		modelName = agent.ModelID
	}
	fmt.Printf("Using model: %s\n", modelName)

	// Generate content
	fmt.Printf("Generating content with model: %s and prompt: %s\n", modelName, prompt)

	// Create generate config with system instruction if provided
	genConfig := &genai.GenerateContentConfig{}
	if agent.SystemPrompt != "" {
		genConfig.SystemInstruction = genai.NewContentFromText(agent.SystemPrompt, genai.RoleUser)
	}

	resp, err := client.Models.GenerateContent(ctx, modelName, genai.Text(prompt), genConfig)
	if err != nil {
		fmt.Printf("Failed to generate content: %v\n", err)
		// Print the type of error for debugging
		fmt.Printf("Error type: %T\n", err)
		return nil, fmt.Errorf("failed to generate content: %w", err)
	}

	if len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("no response generated")
	}

	// Extract the response text
	var responseText string
	if resp.Candidates[0].Content != nil {
		for _, part := range resp.Candidates[0].Content.Parts {
			if part.Text != "" {
				responseText += part.Text
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

// executeWithCustomLLM executes the agent using a custom LLM provider
func (r *Runtime) executeWithCustomLLM(ctx context.Context, agent *primitive.Agent, providerInfo *primitive.Provider, messages []ChatCompletionMessage) (*ChatCompletionResponse, error) {
	// Create custom LLM provider config
	config := provider.ProviderConfig{
		Name:    providerInfo.Name,
		APIKey:  string(providerInfo.APIKeyEnc),
		BaseURL: providerInfo.APIBaseURL,
		Model:   agent.ModelID,
	}

	// Create the custom LLM provider
	customProvider := provider.NewCustomLLMProvider(config)

	// Convert ChatCompletionMessage array to ADK genai.Content format
	contents := make([]*genai.Content, 0, len(messages))
	for _, msg := range messages {
		content := &genai.Content{
			Role: msg.Role,
			Parts: []*genai.Part{
				{Text: msg.Content},
			},
		}
		contents = append(contents, content)
	}

	// Create LLM request
	llmReq := &model.LLMRequest{
		Model:    agent.ModelID,
		Contents: contents,
	}

	// Generate content
	seq := customProvider.GenerateContent(ctx, llmReq, false)
	for resp, err := range seq {
		if err != nil {
			return nil, fmt.Errorf("failed to generate content: %w", err)
		}

		if resp.ErrorCode != "" {
			return nil, fmt.Errorf("LLM error [%s]: %s", resp.ErrorCode, resp.ErrorMessage)
		}

		// Extract response text
		var responseText string
		if resp.Content != nil {
			for _, part := range resp.Content.Parts {
				if part.Text != "" {
					responseText += part.Text
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
				PromptTokens:     int(getPromptTokenCount(resp)),
				CompletionTokens: int(getCandidatesTokenCount(resp)),
				TotalTokens:      int(getTotalTokenCount(resp)),
			},
		}

		return chatResp, nil
	}

	return nil, fmt.Errorf("no response generated")
}

// getPromptTokenCount safely gets the prompt token count from LLMResponse
func getPromptTokenCount(resp *model.LLMResponse) int32 {
	if resp.UsageMetadata != nil {
		return resp.UsageMetadata.PromptTokenCount
	}
	return 0
}

// getCandidatesTokenCount safely gets the candidates token count from LLMResponse
func getCandidatesTokenCount(resp *model.LLMResponse) int32 {
	if resp.UsageMetadata != nil {
		return resp.UsageMetadata.CandidatesTokenCount
	}
	return 0
}

// getTotalTokenCount safely gets the total token count from LLMResponse
func getTotalTokenCount(resp *model.LLMResponse) int32 {
	if resp.UsageMetadata != nil {
		return resp.UsageMetadata.TotalTokenCount
	}
	return 0
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
