package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"github.com/philippgille/chromem-go"
)

// WorkflowMemoryIntegration provides memory operations for workflows
type WorkflowMemoryIntegration struct {
	agent   *MemoryAgent
	logger  logr.Logger
	name    string
	channel chan any
	dbPath  string
}

// WorkflowMemoryConfig holds configuration for workflow memory integration
type WorkflowMemoryConfig struct {
	Enabled           bool   `json:"enabled,omitempty"`
	DBPath            string `json:"dbPath,omitempty"`
	MaxMessages       int    `json:"maxMessages,omitempty"`
	UseLocalEmbedding bool   `json:"useLocalEmbedding,omitempty"`
}

// NewWorkflowMemoryIntegration creates a new memory integration for workflows
func NewWorkflowMemoryIntegration(name string, config *WorkflowMemoryConfig, logger logr.Logger) (*WorkflowMemoryIntegration, error) {
	if config == nil {
		config = &WorkflowMemoryConfig{
			Enabled:           true,
			DBPath:            "/tmp/mule_workflow_memory.db",
			MaxMessages:       100,
			UseLocalEmbedding: true,
		}
	}

	if config.DBPath == "" {
		config.DBPath = "/tmp/mule_workflow_memory.db"
	}

	// Note: Directory creation is handled by ChromeM store itself

	var agent *MemoryAgent
	var err error

	if config.UseLocalEmbedding {
		// Use local embedding for workflow memory
		agent, err = NewMemoryAgentWithEmbedding(config.DBPath, &Config{
			Enabled:      config.Enabled,
			MaxMessages:  config.MaxMessages,
			DefaultLimit: 10,
		}, chromem.EmbeddingFunc(localWorkflowEmbedding))
	} else {
		// Use default embedding (requires API key)
		agent, err = NewMemoryAgent(config.DBPath, &Config{
			Enabled:      config.Enabled,
			MaxMessages:  config.MaxMessages,
			DefaultLimit: 10,
		})
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create memory agent: %w", err)
	}

	return &WorkflowMemoryIntegration{
		agent:   agent,
		logger:  logger,
		name:    name,
		channel: make(chan any),
		dbPath:  config.DBPath,
	}, nil
}

// Call handles memory operations from workflows
func (w *WorkflowMemoryIntegration) Call(event string, data any) (any, error) {
	ctx := context.Background()

	switch event {
	case "search", "searchMemory":
		return w.handleSearch(ctx, data)
	case "save", "saveMemory":
		return w.handleSave(ctx, data)
	case "saveUserMessage":
		return w.handleSaveUserMessage(ctx, data)
	case "saveBotResponse":
		return w.handleSaveBotResponse(ctx, data)
	case "saveConversation":
		return w.handleSaveConversation(ctx, data)
	case "getSummary":
		return w.handleGetSummary(ctx, data)
	case "addContext":
		return w.handleAddContext(ctx, data)
	default:
		return nil, fmt.Errorf("unknown event: %s", event)
	}
}

// handleSearch searches memory for relevant information
func (w *WorkflowMemoryIntegration) handleSearch(ctx context.Context, data any) (string, error) {
	// Parse input
	searchInput, err := parseSearchInput(data)
	if err != nil {
		return "", err
	}

	result, err := w.agent.SearchMemory(ctx, searchInput.Query, searchInput.IntegrationID, searchInput.ChannelID)
	if err != nil {
		return "", fmt.Errorf("failed to search memory: %w", err)
	}

	return result, nil
}

// handleSave saves information to memory
func (w *WorkflowMemoryIntegration) handleSave(ctx context.Context, data any) (string, error) {
	// Parse input
	saveInput, err := parseSaveInput(data)
	if err != nil {
		return "", err
	}

	err = w.agent.ExtractAndStore(ctx, saveInput.Content, saveInput.IntegrationID,
		saveInput.ChannelID, saveInput.UserID, saveInput.Username, saveInput.IsBot)
	if err != nil {
		return "", fmt.Errorf("failed to save to memory: %w", err)
	}

	return "Memory saved successfully", nil
}

// handleGetSummary gets conversation summary
func (w *WorkflowMemoryIntegration) handleGetSummary(ctx context.Context, data any) (string, error) {
	summaryInput, err := parseSummaryInput(data)
	if err != nil {
		return "", err
	}

	summary, err := w.agent.GetConversationSummary(summaryInput.IntegrationID,
		summaryInput.ChannelID, summaryInput.Limit)
	if err != nil {
		return "", fmt.Errorf("failed to get summary: %w", err)
	}

	return summary, nil
}

// handleAddContext adds memory context to a prompt
func (w *WorkflowMemoryIntegration) handleAddContext(ctx context.Context, data any) (string, error) {
	contextInput, err := parseContextInput(data)
	if err != nil {
		return "", err
	}

	enhanced, err := w.agent.AddContextToPrompt(ctx, contextInput.Prompt,
		contextInput.IntegrationID, contextInput.ChannelID)
	if err != nil {
		return "", fmt.Errorf("failed to add context: %w", err)
	}

	return enhanced, nil
}

// handleSaveUserMessage saves user message and returns original message (passthrough)
func (w *WorkflowMemoryIntegration) handleSaveUserMessage(ctx context.Context, data any) (string, error) {
	// Parse input
	dataStr := fmt.Sprintf("%v", data)

	// Save to memory as user message
	err := w.agent.ExtractAndStore(ctx, dataStr, "workflow", "matrix-chat", "user", "User", false)
	if err != nil {
		w.logger.Error(err, "Failed to save user message to memory")
		// Don't fail the workflow for memory errors, just log and continue
	}

	// Return original message for passthrough
	return dataStr, nil
}

// handleSaveBotResponse saves bot response and returns original response (passthrough)
func (w *WorkflowMemoryIntegration) handleSaveBotResponse(ctx context.Context, data any) (string, error) {
	// Parse input - this will be the bot's response from previous step
	dataStr := fmt.Sprintf("%v", data)

	// Save to memory as bot message
	err := w.agent.ExtractAndStore(ctx, dataStr, "workflow", "matrix-chat", "bot", "Assistant", true)
	if err != nil {
		w.logger.Error(err, "Failed to save bot response to memory")
		// Don't fail the workflow for memory errors, just log and continue
	}

	// Return original response for passthrough - this becomes the final workflow output
	return dataStr, nil
}

// handleSaveConversation saves the conversation and returns the agent response
func (w *WorkflowMemoryIntegration) handleSaveConversation(ctx context.Context, data any) (string, error) {
	// The data here should be the bot's response from the previous step
	botResponse := fmt.Sprintf("%v", data)

	// For now, we'll need the user message from the workflow context
	// Since we can't easily access the original user message, we'll just save the bot response
	// and let the Matrix integration handle saving user messages to its own memory

	// Save bot response to memory
	err := w.agent.ExtractAndStore(ctx, botResponse, "workflow", "matrix-chat", "bot", "Assistant", true)
	if err != nil {
		w.logger.Error(err, "Failed to save bot response to memory")
		// Don't fail the workflow for memory errors, just log and continue
	}

	// Return the bot response as the final workflow output
	return botResponse, nil
}

// GetChannel returns the integration's channel
func (w *WorkflowMemoryIntegration) GetChannel() chan any {
	return w.channel
}

// Name returns the integration name
func (w *WorkflowMemoryIntegration) Name() string {
	return w.name
}

// RegisterTrigger is not used for memory integration
func (w *WorkflowMemoryIntegration) RegisterTrigger(trigger string, data any, channel chan any) {
	// Memory integration doesn't use triggers
}

// GetChatHistory retrieves chat history
func (w *WorkflowMemoryIntegration) GetChatHistory(channelID string, limit int) (string, error) {
	return w.agent.GetConversationSummary("workflow", channelID, limit)
}

// ClearChatHistory clears chat history
func (w *WorkflowMemoryIntegration) ClearChatHistory(channelID string) error {
	// ChromeMStore doesn't support clear yet
	return fmt.Errorf("clear not implemented for ChromeM store")
}

// Close closes the memory integration
func (w *WorkflowMemoryIntegration) Close() error {
	if w.agent != nil {
		return w.agent.Close()
	}
	return nil
}

// Input structures for different operations

type SearchInput struct {
	Query         string `json:"query"`
	IntegrationID string `json:"integrationId"`
	ChannelID     string `json:"channelId"`
}

type SaveInput struct {
	Content       string `json:"content"`
	IntegrationID string `json:"integrationId"`
	ChannelID     string `json:"channelId"`
	UserID        string `json:"userId"`
	Username      string `json:"username"`
	IsBot         bool   `json:"isBot"`
}

type SummaryInput struct {
	IntegrationID string `json:"integrationId"`
	ChannelID     string `json:"channelId"`
	Limit         int    `json:"limit"`
}

type ContextInput struct {
	Prompt        string `json:"prompt"`
	IntegrationID string `json:"integrationId"`
	ChannelID     string `json:"channelId"`
}

// Helper functions to parse input

func parseSearchInput(data any) (*SearchInput, error) {
	switch v := data.(type) {
	case string:
		return &SearchInput{Query: v, IntegrationID: "workflow", ChannelID: "default"}, nil
	case map[string]interface{}:
		input := &SearchInput{IntegrationID: "workflow", ChannelID: "default"}
		if q, ok := v["query"].(string); ok {
			input.Query = q
		} else if q, ok := v["message"].(string); ok {
			input.Query = q
		}
		if id, ok := v["integrationId"].(string); ok {
			input.IntegrationID = id
		}
		if ch, ok := v["channelId"].(string); ok {
			input.ChannelID = ch
		}
		return input, nil
	default:
		jsonBytes, _ := json.Marshal(data)
		input := &SearchInput{}
		if err := json.Unmarshal(jsonBytes, input); err == nil {
			if input.IntegrationID == "" {
				input.IntegrationID = "workflow"
			}
			if input.ChannelID == "" {
				input.ChannelID = "default"
			}
			return input, nil
		}
		return &SearchInput{Query: fmt.Sprintf("%v", data), IntegrationID: "workflow", ChannelID: "default"}, nil
	}
}

func parseSaveInput(data any) (*SaveInput, error) {
	switch v := data.(type) {
	case string:
		return &SaveInput{
			Content:       v,
			IntegrationID: "workflow",
			ChannelID:     "default",
			UserID:        "user",
			Username:      "User",
			IsBot:         false,
		}, nil
	case map[string]interface{}:
		input := &SaveInput{
			IntegrationID: "workflow",
			ChannelID:     "default",
			UserID:        "user",
			Username:      "User",
			IsBot:         false,
		}
		if c, ok := v["content"].(string); ok {
			input.Content = c
		} else if c, ok := v["message"].(string); ok {
			input.Content = c
		}
		if id, ok := v["integrationId"].(string); ok {
			input.IntegrationID = id
		}
		if ch, ok := v["channelId"].(string); ok {
			input.ChannelID = ch
		}
		if uid, ok := v["userId"].(string); ok {
			input.UserID = uid
		}
		if un, ok := v["username"].(string); ok {
			input.Username = un
		}
		if ib, ok := v["isBot"].(bool); ok {
			input.IsBot = ib
		}
		return input, nil
	default:
		return nil, fmt.Errorf("invalid save input format")
	}
}

func parseSummaryInput(data any) (*SummaryInput, error) {
	switch v := data.(type) {
	case map[string]interface{}:
		input := &SummaryInput{
			IntegrationID: "workflow",
			ChannelID:     "default",
			Limit:         10,
		}
		if id, ok := v["integrationId"].(string); ok {
			input.IntegrationID = id
		}
		if ch, ok := v["channelId"].(string); ok {
			input.ChannelID = ch
		}
		if l, ok := v["limit"].(int); ok {
			input.Limit = l
		}
		return input, nil
	default:
		return &SummaryInput{
			IntegrationID: "workflow",
			ChannelID:     "default",
			Limit:         10,
		}, nil
	}
}

func parseContextInput(data any) (*ContextInput, error) {
	switch v := data.(type) {
	case string:
		return &ContextInput{
			Prompt:        v,
			IntegrationID: "workflow",
			ChannelID:     "default",
		}, nil
	case map[string]interface{}:
		input := &ContextInput{
			IntegrationID: "workflow",
			ChannelID:     "default",
		}
		if p, ok := v["prompt"].(string); ok {
			input.Prompt = p
		} else if p, ok := v["message"].(string); ok {
			input.Prompt = p
		}
		if id, ok := v["integrationId"].(string); ok {
			input.IntegrationID = id
		}
		if ch, ok := v["channelId"].(string); ok {
			input.ChannelID = ch
		}
		return input, nil
	default:
		return nil, fmt.Errorf("invalid context input format")
	}
}

// localWorkflowEmbedding provides local embedding for workflows
func localWorkflowEmbedding(ctx context.Context, text string) ([]float32, error) {
	embedding := make([]float32, 384)
	text = strings.ToLower(text)
	words := strings.Fields(text)

	for _, word := range words {
		hash := 0
		for _, char := range word {
			hash = (hash*31 + int(char)) % 384
		}
		embedding[hash] += 1.0
		if hash > 0 {
			embedding[hash-1] += 0.5
		}
		if hash < 383 {
			embedding[hash+1] += 0.5
		}
	}

	// Normalize
	var sum float32
	for _, v := range embedding {
		sum += v * v
	}
	if sum > 0 {
		norm := float32(1.0)
		for i := 1; i <= 100; i++ {
			temp := float32(i) / 10.0
			tempSum := sum * temp * temp
			if tempSum >= 0.9 && tempSum <= 1.1 {
				norm = temp
				break
			}
		}
		for i := range embedding {
			embedding[i] /= norm
		}
	}

	return embedding, nil
}
