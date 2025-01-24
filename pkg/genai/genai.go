package genai

type AIService struct {
	Server string
	Model  string
	Type   string
	APIKey string
}

func Chat(prompt string, aiService AIService) (string, error) {
	if aiService.Type == "gemini" {
		return geminiChat(prompt, aiService)
	}
	return ollamaChat(prompt, aiService)
}
