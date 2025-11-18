package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	iter "iter"
	"net/http"
	"strings"

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
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	Stream      bool            `json:"stream"`
	Temperature float64         `json:"temperature,omitempty"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
}

// OpenAIMessage represents a message in OpenAI format
type OpenAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
		message := OpenAIMessage{
			Role:    content.Role,
			Content: p.extractTextFromContent(content),
		}
		messages = append(messages, message)
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

	return openaiReq
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
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{{Text: choice.Message.Content}},
		},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     int32(openaiResp.Usage.PromptTokens),
			CandidatesTokenCount: int32(openaiResp.Usage.CompletionTokens),
			TotalTokenCount:      int32(openaiResp.Usage.TotalTokens),
		},
		FinishReason: genai.FinishReasonStop,
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
