package settings

import "dev-team/pkg/genai"

type Settings struct {
	OllamaServer string `json:"ollamaServer"`
	OllamaModel  string `json:"ollamaModel"`
	GitHubToken  string `json:"githubToken"`
	AIService    string `json:"aiService"`
	GeminiAPIKey string `json:"geminiAPIKey"`
	GeminiModel  string `json:"geminiModel"`
}

func (s *Settings) GetAIService() genai.AIService {
	if s.AIService == "gemini" {
		return genai.AIService{
			Server: "",
			Model:  s.GeminiModel,
			Type:   s.AIService,
			APIKey: s.GeminiAPIKey,
		}
	}
	return genai.AIService{
		Server: s.OllamaServer,
		Model:  s.OllamaModel,
		Type:   s.AIService,
		APIKey: "",
	}
}
