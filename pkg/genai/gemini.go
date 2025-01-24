package genai

import (
	"context"
	"fmt"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

func GetGeminiModels(apiKey string) ([]string, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %v", err)
	}
	defer client.Close()

	iter := client.ListModels(ctx)
	var geminiModels []string

	for {
		model, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error listing models: %v", err)
		}

		// Include all available models
		geminiModels = append(geminiModels, model.Name)
	}

	if len(geminiModels) == 0 {
		return []string{"gemini-pro", "gemini-pro-vision"}, nil
	}

	return geminiModels, nil
}

func geminiChat(prompt string, aiService AIService) (string, error) {
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey(aiService.APIKey))
	if err != nil {
		return "", fmt.Errorf("failed to create Gemini client: %v", err)
	}
	defer client.Close()

	geminiModel := client.GenerativeModel(aiService.Model)

	resp, err := geminiModel.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return "", fmt.Errorf("failed to generate content: %v", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("no response from Gemini API")
	}

	text, ok := resp.Candidates[0].Content.Parts[0].(genai.Text)
	if !ok {
		return "", fmt.Errorf("unexpected response type from Gemini API")
	}

	return string(text), nil
}
