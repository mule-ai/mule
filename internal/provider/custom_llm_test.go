package provider

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

func TestCustomLLMProvider_Name(t *testing.T) {
	config := ProviderConfig{
		Name:    "test-provider",
		APIKey:  "test-key",
		BaseURL: "https://api.test.com",
		Model:   "test-model",
	}

	provider := NewCustomLLMProvider(config)
	assert.Equal(t, "test-provider", provider.Name())
}

func TestCustomLLMProvider_ConvertToOpenAIRequest(t *testing.T) {
	config := ProviderConfig{
		Name:    "test-provider",
		APIKey:  "test-key",
		BaseURL: "https://api.test.com",
		Model:   "test-model",
	}

	provider := NewCustomLLMProvider(config).(*CustomLLMProvider)

	// Create test content
	content := &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			{Text: "Hello, world!"},
		},
	}

	// Create LLM request
	req := &model.LLMRequest{
		Model:    "test-model",
		Contents: []*genai.Content{content},
		Config:   &genai.GenerateContentConfig{},
	}

	// Convert request
	openaiReq := provider.convertToOpenAIRequest(req)

	// Verify conversion
	assert.Equal(t, "test-model", openaiReq.Model)
	assert.Len(t, openaiReq.Messages, 1)
	assert.Equal(t, "user", openaiReq.Messages[0].Role)
	assert.Equal(t, "Hello, world!", openaiReq.Messages[0].Content)
	assert.False(t, openaiReq.Stream)
}

func TestCustomLLMProvider_ExtractTextFromContent(t *testing.T) {
	config := ProviderConfig{
		Name:    "test-provider",
		APIKey:  "test-key",
		BaseURL: "https://api.test.com",
		Model:   "test-model",
	}

	provider := NewCustomLLMProvider(config).(*CustomLLMProvider)

	// Test single part content
	content := &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			{Text: "Hello, world!"},
		},
	}

	text := provider.extractTextFromContent(content)
	assert.Equal(t, "Hello, world!", text)

	// Test multiple parts content
	contentMulti := &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			{Text: "Hello, "},
			{Text: "world!"},
		},
	}

	textMulti := provider.extractTextFromContent(contentMulti)
	assert.Equal(t, "Hello, world!", textMulti)
}

func TestCustomLLMProvider_ConvertOpenAIResponseToADK(t *testing.T) {
	config := ProviderConfig{
		Name:    "test-provider",
		APIKey:  "test-key",
		BaseURL: "https://api.test.com",
		Model:   "test-model",
	}

	provider := NewCustomLLMProvider(config).(*CustomLLMProvider)

	// Test valid OpenAI response
	openaiRespBody := `{
		"choices": [
			{
				"message": {
					"role": "assistant",
					"content": "Hello! How can I help you?"
				},
				"finish_reason": "stop"
			}
		],
		"usage": {
			"prompt_tokens": 10,
			"completion_tokens": 8,
			"total_tokens": 18
		}
	}`

	resp, err := provider.convertOpenAIResponseToADK([]byte(openaiRespBody))
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	assert.Equal(t, "model", resp.Content.Role)
	assert.Equal(t, "Hello! How can I help you?", resp.Content.Parts[0].Text)
	assert.Equal(t, genai.FinishReasonStop, resp.FinishReason)
	assert.Equal(t, int32(10), resp.UsageMetadata.PromptTokenCount)
	assert.Equal(t, int32(8), resp.UsageMetadata.CandidatesTokenCount)
	assert.Equal(t, int32(18), resp.UsageMetadata.TotalTokenCount)

	// Test empty choices response
	emptyRespBody := `{
		"choices": [],
		"usage": {
			"prompt_tokens": 0,
			"completion_tokens": 0,
			"total_tokens": 0
		}
	}`

	respEmpty, err := provider.convertOpenAIResponseToADK([]byte(emptyRespBody))
	assert.NoError(t, err)
	assert.NotNil(t, respEmpty)
	assert.Equal(t, "NO_CHOICES", respEmpty.ErrorCode)

	// Test invalid JSON response
	invalidRespBody := `{invalid json}`

	respInvalid, err := provider.convertOpenAIResponseToADK([]byte(invalidRespBody))
	assert.NoError(t, err)
	assert.NotNil(t, respInvalid)
	assert.Equal(t, "PARSE_ERROR", respInvalid.ErrorCode)
}
