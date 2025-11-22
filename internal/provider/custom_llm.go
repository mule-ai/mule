package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	iter "iter"
	"net/http"
	"strings"

	adkTool "google.golang.org/adk/tool"
	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// CustomLLMProvider implements the model.LLM interface to support custom providers
type CustomLLMProvider struct {
	name       string
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
}

// ProviderConfig holds the configuration for a custom provider
type ProviderConfig struct {
	Name    string `json:"name"`
	APIKey  string `json:"api_key"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

// NewCustomLLMProvider creates a new custom LLM provider
func NewCustomLLMProvider(config ProviderConfig) model.LLM {
	return &CustomLLMProvider{
		name:       config.Name,
		apiKey:     config.APIKey,
		baseURL:    config.BaseURL,
		model:      config.Model,
		httpClient: &http.Client{},
	}
}

// Name returns the provider name
func (p *CustomLLMProvider) Name() string {
	return p.name
}

// GenerateContent generates content using the custom provider
func (p *CustomLLMProvider) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	if stream {
		return p.generateStream(ctx, req)
	}

	return func(yield func(*model.LLMResponse, error) bool) {
		resp, err := p.generate(ctx, req)
		yield(resp, err)
	}
}

// generate handles non-streaming content generation
func (p *CustomLLMProvider) generate(ctx context.Context, req *model.LLMRequest) (*model.LLMResponse, error) {
	// Convert ADK request to OpenAI-compatible format
	openaiReq := p.convertToOpenAIRequest(req)

	// Make HTTP request to custom provider
	resp, err := p.makeHTTPRequest(ctx, openaiReq)
	if err != nil {
		return &model.LLMResponse{
			ErrorCode:    "HTTP_ERROR",
			ErrorMessage: fmt.Sprintf("HTTP request failed: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &model.LLMResponse{
			ErrorCode:    "READ_ERROR",
			ErrorMessage: fmt.Sprintf("Failed to read response: %v", err),
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return &model.LLMResponse{
			ErrorCode:    "API_ERROR",
			ErrorMessage: fmt.Sprintf("API returned status %d: %s", resp.StatusCode, string(body)),
		}, nil
	}

	// Convert OpenAI response to ADK format
	return p.convertOpenAIResponseToADK(body)
}

// generateStream handles streaming content generation
func (p *CustomLLMProvider) generateStream(ctx context.Context, req *model.LLMRequest) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		// For now, implement streaming as non-streaming
		// In a real implementation, you would handle Server-Sent Events
		resp, err := p.generate(ctx, req)
		if !yield(resp, err) {
			return
		}
	}
}

// OpenAIRequest represents an OpenAI-compatible request
type OpenAIRequest struct {
	Model       string                    `json:"model"`
	Messages    []OpenAIMessage           `json:"messages"`
	Stream      bool                      `json:"stream"`
	Temperature float64                   `json:"temperature,omitempty"`
	MaxTokens   int                       `json:"max_tokens,omitempty"`
	Tools       []OpenAITool              `json:"tools,omitempty"`
	ToolChoice  interface{}               `json:"tool_choice,omitempty"`
}

// OpenAITool represents a tool in OpenAI format
type OpenAITool struct {
	Type     string                    `json:"type"`
	Function OpenAIFunctionDeclaration `json:"function"`
}

// OpenAIFunctionDeclaration represents a function declaration in OpenAI format
type OpenAIFunctionDeclaration struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}

// OpenAIMessage represents a message in OpenAI format
type OpenAIMessage struct {
	Role       string          `json:"role"`
	Content    string          `json:"content,omitempty"`
	ToolCalls  []OpenAIToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Name       string          `json:"name,omitempty"`
}

// OpenAIToolCall represents a tool call in OpenAI format
type OpenAIToolCall struct {
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function OpenAIFunctionCall  `json:"function"`
	Index    int                  `json:"index,omitempty"`
}

// OpenAIFunctionCall represents a function call in OpenAI format
type OpenAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// OpenAIResponse represents an OpenAI-compatible response
type OpenAIResponse struct {
	Choices []OpenAIChoice `json:"choices"`
	Usage   OpenAIUsage    `json:"usage"`
}

// OpenAIChoice represents a choice in OpenAI response
type OpenAIChoice struct {
	Message      OpenAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
	ToolCalls    []OpenAIToolCall `json:"tool_calls,omitempty"`
}

// OpenAIUsage represents token usage in OpenAI response
type OpenAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// convertToOpenAIRequest converts ADK LLMRequest to OpenAI format
func (p *CustomLLMProvider) convertToOpenAIRequest(req *model.LLMRequest) OpenAIRequest {
	messages := make([]OpenAIMessage, 0, len(req.Contents))

	for _, content := range req.Contents {
		message := p.convertContentToOpenAIMessage(content)
		if message != nil {
			messages = append(messages, *message)
		}
	}

	openaiReq := OpenAIRequest{
		Model:    p.model,
		Messages: messages,
		Stream:   false,
	}

	// Extract temperature and max tokens from config if available
	if req.Config != nil {
		if req.Config.Temperature != nil {
			openaiReq.Temperature = float64(*req.Config.Temperature)
		}
		if req.Config.MaxOutputTokens > 0 {
			openaiReq.MaxTokens = int(req.Config.MaxOutputTokens)
		}
	}

	// Convert tools if present
	if req.Tools != nil && len(req.Tools) > 0 {
		tools := make([]OpenAITool, 0, len(req.Tools))
		for name, tool := range req.Tools {
			// Extract tool information
			if t, ok := tool.(adkTool.Tool); ok {
				var parameters map[string]interface{}

				// First try to get underlying tool via GetTool() method (for our adapters)
				if toolWithGetTool, ok := tool.(interface{ GetTool() interface{} }); ok {
					underlyingTool := toolWithGetTool.GetTool()
					// Now check if the underlying tool has GetSchema
					if toolWithSchema, ok := underlyingTool.(interface{ GetSchema() map[string]interface{} }); ok {
						parameters = toolWithSchema.GetSchema()
						fmt.Printf("Tool %s: Using schema from underlying tool\n", t.Name())
					} else {
						parameters = map[string]interface{}{"type": "object"}
						fmt.Printf("Tool %s: Underlying tool has no schema\n", t.Name())
					}
				} else {
					// Fallback to empty parameters
					parameters = map[string]interface{}{"type": "object"}
					fmt.Printf("Tool %s: No GetTool method, using empty parameters\n", t.Name())
				}

				toolDecl := OpenAITool{
					Type: "function",
					Function: OpenAIFunctionDeclaration{
						Name:        t.Name(),
						Description: t.Description(),
						Parameters:  parameters,
					},
				}
				tools = append(tools, toolDecl)
			} else {
				// Fallback for other tool types
				toolDecl := OpenAITool{
					Type: "function",
					Function: OpenAIFunctionDeclaration{
						Name: name,
					},
				}
				tools = append(tools, toolDecl)
			}
		}
		openaiReq.Tools = tools
		fmt.Printf("Converted %d tools to OpenAI format\n", len(tools))
	}

	return openaiReq
}

// convertContentToOpenAIMessage converts a genai.Content to OpenAIMessage
func (p *CustomLLMProvider) convertContentToOpenAIMessage(content *genai.Content) *OpenAIMessage {
	// Check if this is a tool response (role="tool")
	if content.Role == "tool" {
		// Extract tool response content from FunctionResponse parts
		var toolResponse strings.Builder
		toolCallID := ""

		for _, part := range content.Parts {
			if part.FunctionResponse != nil {
				// Marshal the response to JSON string
				if part.FunctionResponse.Response != nil {
					responseJSON, err := json.Marshal(part.FunctionResponse.Response)
					if err == nil {
						toolResponse.WriteString(string(responseJSON))
					} else {
						toolResponse.WriteString(fmt.Sprintf("{\"error\": \"failed to marshal response\"}"))
					}
				}
				// Get the tool call ID from the FunctionResponse
				if part.FunctionResponse.ID != "" {
					toolCallID = part.FunctionResponse.ID
				}
			}
		}

		// If we still don't have content, use a default message
		contentStr := toolResponse.String()
		if contentStr == "" {
			contentStr = "Tool execution completed"
		}

		// If we still don't have a tool call ID, generate one
		if toolCallID == "" {
			toolCallID = "call_" + content.Role
		}

		return &OpenAIMessage{
			Role:       "tool",
			Content:    contentStr,
			ToolCallID: toolCallID,
		}
	}

	// Check for tool calls in the parts
	var toolCalls []OpenAIToolCall
	var textContent strings.Builder

	for _, part := range content.Parts {
		if part.Text != "" {
			textContent.WriteString(part.Text)
		}

		// Check if this is a function call
		if part.FunctionCall != nil {
			// Convert Args map to JSON string
			argsJSON, err := json.Marshal(part.FunctionCall.Args)
			if err != nil {
				// If marshaling fails, use empty object
				argsJSON = []byte("{}")
			}

			toolCall := OpenAIToolCall{
				ID:   "call_" + part.FunctionCall.Name,
				Type: "function",
				Function: OpenAIFunctionCall{
					Name:      part.FunctionCall.Name,
					Arguments: string(argsJSON),
				},
			}
			toolCalls = append(toolCalls, toolCall)
		}
	}

	// If we have tool calls, return a message with tool calls
	if len(toolCalls) > 0 {
		// Map "model" role to "assistant" for OpenAI compatibility
		role := content.Role
		if role == "model" {
			role = "assistant"
		}
		return &OpenAIMessage{
			Role:      role,
			Content:   "", // Empty content is required even with tool calls
			ToolCalls: toolCalls,
		}
	}

	// Otherwise, return a regular text message
	if textContent.Len() > 0 {
		// Map "model" role to "assistant" for OpenAI compatibility
		role := content.Role
		if role == "model" {
			role = "assistant"
		}
		return &OpenAIMessage{
			Role:    role,
			Content: textContent.String(),
		}
	}

	return nil
}

// extractTextFromContent extracts text content from genai.Content
func (p *CustomLLMProvider) extractTextFromContent(content *genai.Content) string {
	var text strings.Builder
	for _, part := range content.Parts {
		if part.Text != "" {
			text.WriteString(part.Text)
		}
	}
	return text.String()
}

// makeHTTPRequest makes an HTTP request to the custom provider
func (p *CustomLLMProvider) makeHTTPRequest(ctx context.Context, req OpenAIRequest) (*http.Response, error) {
	// Marshal request to JSON
	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Debug: Print the request being sent
	fmt.Printf("DEBUG: Sending OpenAI request with %d messages\n", len(req.Messages))
	for i, msg := range req.Messages {
		fmt.Printf("DEBUG: Message %d - Role: %s, Content: '%s', ToolCalls: %d, ToolCallID: '%s'\n",
			i, msg.Role, msg.Content, len(msg.ToolCalls), msg.ToolCallID)
		for j, tc := range msg.ToolCalls {
			fmt.Printf("DEBUG:   ToolCall %d - ID: %s, Name: %s\n", j, tc.ID, tc.Function.Name)
		}
	}

	// Create HTTP request
	url := fmt.Sprintf("%s/chat/completions", strings.TrimSuffix(p.baseURL, "/"))
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiKey))

	// Make request
	return p.httpClient.Do(httpReq)
}

// convertOpenAIResponseToADK converts OpenAI response to ADK LLMResponse format
func (p *CustomLLMProvider) convertOpenAIResponseToADK(body []byte) (*model.LLMResponse, error) {
	var openaiResp OpenAIResponse
	if err := json.Unmarshal(body, &openaiResp); err != nil {
		return &model.LLMResponse{
			ErrorCode:    "PARSE_ERROR",
			ErrorMessage: fmt.Sprintf("Failed to parse response: %v", err),
		}, nil
	}

	if len(openaiResp.Choices) == 0 {
		return &model.LLMResponse{
			ErrorCode:    "NO_CHOICES",
			ErrorMessage: "No choices returned from provider",
		}, nil
	}

	choice := openaiResp.Choices[0]

	// Convert to ADK format
	adkResp := &model.LLMResponse{
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     int32(openaiResp.Usage.PromptTokens),
			CandidatesTokenCount: int32(openaiResp.Usage.CompletionTokens),
			TotalTokenCount:      int32(openaiResp.Usage.TotalTokens),
		},
		FinishReason: genai.FinishReasonStop,
	}

	// Handle content and tool calls
	if len(choice.Message.ToolCalls) > 0 {
		// This is a tool call response
		adkResp.Content = &genai.Content{
			Role: "model",
		}

		// Convert tool calls to genai.Parts
		for _, toolCall := range choice.Message.ToolCalls {
			// Parse the arguments JSON string into a map
			var args map[string]interface{}
			if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
				// If parsing fails, use empty args
				args = make(map[string]interface{})
			}

			part := &genai.Part{
				FunctionCall: &genai.FunctionCall{
					ID:   toolCall.ID,
					Name: toolCall.Function.Name,
					Args: args,
				},
			}
			adkResp.Content.Parts = append(adkResp.Content.Parts, part)
		}
		fmt.Printf("Converted %d tool calls to ADK format\n", len(choice.Message.ToolCalls))
	} else {
		// This is a regular text response
		adkResp.Content = &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: choice.Message.Content}},
		}
	}

	// Map finish reason
	switch choice.FinishReason {
	case "stop":
		adkResp.FinishReason = genai.FinishReasonStop
	case "length":
		adkResp.FinishReason = genai.FinishReasonMaxTokens
	case "tool_calls":
		adkResp.FinishReason = genai.FinishReasonStop
	default:
		adkResp.FinishReason = genai.FinishReasonOther
	}

	return adkResp, nil
}
