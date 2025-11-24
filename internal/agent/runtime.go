package agent

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"

	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/internal/provider"
	"github.com/mule-ai/mule/internal/tools"
	"github.com/mule-ai/mule/pkg/job"
)

// Runtime handles agent execution using Google ADK
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

	// Get tools for this agent and add them to the request
	adkTools, err := r.getAgentTools(ctx, agent.ID)
	if err == nil && len(adkTools) > 0 {
		// Convert ADK tools to FunctionDeclarations for Google GenAI SDK
		funcDecls := make([]*genai.FunctionDeclaration, 0, len(adkTools))
		for _, t := range adkTools {
			funcDecl := &genai.FunctionDeclaration{
				Name:        t.Name(),
				Description: t.Description(),
				// Note: Parameters would need to be extracted from the tool's schema
			}
			funcDecls = append(funcDecls, funcDecl)
		}

		// Add tools to the config
		genConfig.Tools = []*genai.Tool{
			{
				FunctionDeclarations: funcDecls,
			},
		}
		fmt.Printf("Added %d tools to the request\n", len(adkTools))
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

	// Add system prompt if available
	if agent.SystemPrompt != "" {
		// Create a system message as the first content
		systemContent := &genai.Content{
			Role: "system",
			Parts: []*genai.Part{
				{Text: agent.SystemPrompt},
			},
		}
		// Insert system content at the beginning
		llmReq.Contents = append([]*genai.Content{systemContent}, llmReq.Contents...)
	}

	// Get tools for this agent and add them to the request
	adkTools, err := r.getAgentTools(ctx, agent.ID)
	if err == nil && len(adkTools) > 0 {
		// Initialize tools map and config if needed
		if llmReq.Tools == nil {
			llmReq.Tools = make(map[string]interface{})
		}
		if llmReq.Config == nil {
			llmReq.Config = &genai.GenerateContentConfig{}
		}

		// Add each tool to the request
		for _, t := range adkTools {
			llmReq.Tools[t.Name()] = t

			// Add function declaration to config
			if funcTool, ok := t.(interface{ Declaration() *genai.FunctionDeclaration }); ok {
				decl := funcTool.Declaration()
				if decl != nil {
					// Find or create the function declarations tool
					var funcTool *genai.Tool
					for _, tool := range llmReq.Config.Tools {
						if tool != nil && tool.FunctionDeclarations != nil {
							funcTool = tool
							break
						}
					}
					if funcTool == nil {
						llmReq.Config.Tools = append(llmReq.Config.Tools, &genai.Tool{
							FunctionDeclarations: []*genai.FunctionDeclaration{decl},
						})
					} else {
						funcTool.FunctionDeclarations = append(funcTool.FunctionDeclarations, decl)
					}
				}
			}
		}
		fmt.Printf("Added %d tools to custom LLM request\n", len(adkTools))
	} else {
		fmt.Printf("No tools added to custom LLM request (err: %v, tools: %d)\n", err, len(adkTools))
	}

	// Generate content
	var resp *model.LLMResponse
	seq := customProvider.GenerateContent(ctx, llmReq, false)
	for resp, err = range seq {
		if err != nil {
			return nil, fmt.Errorf("failed to generate content: %w", err)
		}

		if resp.ErrorCode != "" {
			return nil, fmt.Errorf("LLM error [%s]: %s", resp.ErrorCode, resp.ErrorMessage)
		}
	}

	// Handle tool execution loop using ADK's built-in tool handling
	maxIterations := 10 // Prevent infinite loops
	iteration := 0

	for iteration < maxIterations {
		iteration++

		// Check if response contains function calls
		hasFunctionCalls := false
		if resp.Content != nil {
			for _, part := range resp.Content.Parts {
				if part.FunctionCall != nil {
					hasFunctionCalls = true
					break
				}
			}
		}

		if !hasFunctionCalls {
			// No function calls, we have a final response
			break
		}

		fmt.Printf("Executing %d tool calls (iteration %d)\n", len(resp.Content.Parts), iteration)

		// Execute function calls using ADK's tool execution
		toolResults := make([]*genai.Part, 0)

		for _, part := range resp.Content.Parts {
			if part.FunctionCall != nil {
				funcCall := part.FunctionCall
				fmt.Printf("Executing tool: %s with args: %v\n", funcCall.Name, funcCall.Args)

				// Find and execute the tool using the FunctionTool interface
				var result map[string]any
				var err error

				for _, t := range adkTools {
					if t.Name() == funcCall.Name {
						if funcTool, ok := t.(interface {
							Run(ctx tool.Context, args any) (map[string]any, error)
						}); ok {
							// Create a simple tool context adapter
							toolCtx := &toolContextAdapter{ctx: ctx, functionCallID: funcCall.ID}
							result, err = funcTool.Run(toolCtx, funcCall.Args)
						} else {
							err = fmt.Errorf("tool %s does not implement Run method", funcCall.Name)
						}
						break
					}
				}

				if err != nil {
					fmt.Printf("Tool execution failed: %v\n", err)
					result = map[string]any{
						"error": err.Error(),
					}
				}

				// Create tool result part with matching ID
				toolResult := genai.NewPartFromFunctionResponse(funcCall.Name, result)
				toolResult.FunctionResponse.ID = funcCall.ID
				toolResults = append(toolResults, toolResult)
				fmt.Printf("Tool result for %s (ID: %s): %+v\n", funcCall.Name, funcCall.ID, result)
			}
		}

		// Add both the tool call request and tool call response to the conversation
		llmReq.Contents = append(llmReq.Contents, resp.Content)  // Model message with FunctionCall parts
		llmReq.Contents = append(llmReq.Contents, &genai.Content{
			Role:  "tool",  // Role should be "tool" for tool responses
			Parts: toolResults,
		})

		// Generate next response with tool results
		seq = customProvider.GenerateContent(ctx, llmReq, false)
		for resp, err = range seq {
			if err != nil {
				return nil, fmt.Errorf("failed to generate content after tool execution: %w", err)
			}

			if resp.ErrorCode != "" {
				return nil, fmt.Errorf("LLM error after tool execution [%s]: %s", resp.ErrorCode, resp.ErrorMessage)
			}
		}
	}

	if iteration >= maxIterations {
		fmt.Printf("Warning: Reached maximum tool execution iterations (%d)\n", maxIterations)
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

// getAgentTools retrieves tools for an agent
func (r *Runtime) getAgentTools(ctx context.Context, agentID string) ([]tool.Tool, error) {
	// Use the store interface method to get agent tools
	tools, err := r.store.GetAgentTools(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent tools: %w", err)
	}

	var adkTools []tool.Tool
	for _, t := range tools {
		// Check if it's a built-in tool by checking metadata for tool_type
		if toolType, ok := t.Metadata["tool_type"].(string); ok {
			if builtinTool, err := r.toolRegistry.Get(toolType); err == nil {
				adkTools = append(adkTools, builtinTool.ToTool())
			}
		}
	}

	return adkTools, nil
}

// ExecuteWorkflow submits a workflow for execution and returns the job
func (r *Runtime) ExecuteWorkflow(ctx context.Context, req *ChatCompletionRequest) (*job.Job, error) {
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

	// Submit job to workflow engine
	return r.workflowEngine.SubmitJob(ctx, targetWorkflow.ID, inputData)
}

// toolContextAdapter adapts context.Context to tool.Context
type toolContextAdapter struct {
	ctx            context.Context
	functionCallID string
}

func (a *toolContextAdapter) FunctionCallID() string {
	return a.functionCallID
}

func (a *toolContextAdapter) Actions() *session.EventActions {
	// Return nil as we don't need actions for now
	return nil
}

func (a *toolContextAdapter) SearchMemory(ctx context.Context, query string) (*memory.SearchResponse, error) {
	// Return nil as we don't have memory search implemented yet
	return nil, nil
}

func (a *toolContextAdapter) Artifacts() agent.Artifacts {
	// Return nil as we don't have artifacts implemented yet
	return nil
}

func (a *toolContextAdapter) State() session.State {
	// Return nil as we don't have state implemented yet
	return nil
}

// Implement agent.ReadonlyContext methods
func (a *toolContextAdapter) UserContent() *genai.Content {
	return nil
}

func (a *toolContextAdapter) InvocationID() string {
	return ""
}

func (a *toolContextAdapter) AgentName() string {
	return ""
}

func (a *toolContextAdapter) ReadonlyState() session.ReadonlyState {
	return nil
}

func (a *toolContextAdapter) UserID() string {
	return ""
}

func (a *toolContextAdapter) AppName() string {
	return ""
}

func (a *toolContextAdapter) SessionID() string {
	return ""
}

func (a *toolContextAdapter) Branch() string {
	return ""
}

// Implement context.Context methods by delegating to the wrapped context
func (a *toolContextAdapter) Deadline() (deadline time.Time, ok bool) {
	return a.ctx.Deadline()
}

func (a *toolContextAdapter) Done() <-chan struct{} {
	return a.ctx.Done()
}

func (a *toolContextAdapter) Err() error {
	return a.ctx.Err()
}

func (a *toolContextAdapter) Value(key any) any {
	return a.ctx.Value(key)
}
