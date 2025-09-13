package memory

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/philippgille/chromem-go"
)

// MemoryAgent handles intelligent memory operations
type MemoryAgent struct {
	store      *ChromeMStore
	config     *Config
	maxContext int // Maximum number of messages to include in context
}

// NewMemoryAgent creates a new memory agent with ChromeM backend
func NewMemoryAgent(dbPath string, config *Config) (*MemoryAgent, error) {
	if config == nil {
		config = DefaultConfig()
	}

	store, err := NewChromeMStore(dbPath, config.MaxMessages)
	if err != nil {
		return nil, fmt.Errorf("failed to create ChromeM store: %w", err)
	}

	return &MemoryAgent{
		store:      store,
		config:     config,
		maxContext: 10, // Default context size
	}, nil
}

// NewMemoryAgentWithEmbedding creates a new memory agent with custom embedding function
func NewMemoryAgentWithEmbedding(dbPath string, config *Config, embeddingFunc any) (*MemoryAgent, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Type assert to chromem.EmbeddingFunc
	var embedFunc chromem.EmbeddingFunc
	if embeddingFunc != nil {
		var ok bool
		embedFunc, ok = embeddingFunc.(chromem.EmbeddingFunc)
		if !ok {
			return nil, fmt.Errorf("invalid embedding function type")
		}
	}

	store, err := NewChromeMStoreWithEmbedding(dbPath, config.MaxMessages, embedFunc)
	if err != nil {
		return nil, fmt.Errorf("failed to create ChromeM store: %w", err)
	}

	return &MemoryAgent{
		store:      store,
		config:     config,
		maxContext: 10, // Default context size
	}, nil
}

// SearchMemory searches memory for relevant information based on a query
func (m *MemoryAgent) SearchMemory(ctx context.Context, query string, integrationID, channelID string) (string, error) {
	// Search for relevant messages
	messages, err := m.store.SearchMessages(query, integrationID, channelID, m.maxContext)
	if err != nil {
		return "", fmt.Errorf("failed to search memory: %w", err)
	}

	if len(messages) == 0 {
		return "", nil
	}

	// Format messages as context
	return formatMessagesAsMemoryContext(messages), nil
}

// ExtractAndStore extracts important information from a conversation and stores it
func (m *MemoryAgent) ExtractAndStore(ctx context.Context, content string, integrationID, channelID, userID, username string, isBot bool) error {
	// For now, we'll store the entire message
	// In a more sophisticated implementation, you could:
	// 1. Extract key entities and facts
	// 2. Summarize long messages
	// 3. Identify important topics

	msg := Message{
		ID:            GenerateID(),
		IntegrationID: integrationID,
		ChannelID:     channelID,
		UserID:        userID,
		Username:      username,
		Content:       content,
		Timestamp:     time.Now(),
		IsBot:         isBot,
	}

	return m.store.SaveMessage(msg)
}

// AddContextToPrompt adds relevant memory context to a prompt
func (m *MemoryAgent) AddContextToPrompt(ctx context.Context, prompt string, integrationID, channelID string) (string, error) {
	// Search for relevant context
	context, err := m.SearchMemory(ctx, prompt, integrationID, channelID)
	if err != nil {
		return prompt, err // Return original prompt on error
	}

	if context == "" {
		return prompt, nil // No relevant context found
	}

	// Add context only, don't include the original prompt since the workflow will handle that
	enhancedPrompt := fmt.Sprintf("<memory>\n%s\n</memory>", context)
	return enhancedPrompt, nil
}

// GetConversationSummary gets a summary of recent conversation
func (m *MemoryAgent) GetConversationSummary(integrationID, channelID string, limit int) (string, error) {
	messages, err := m.store.GetRecentMessages(integrationID, channelID, limit)
	if err != nil {
		return "", fmt.Errorf("failed to get recent messages: %w", err)
	}

	if len(messages) == 0 {
		return "No recent conversation history.", nil
	}

	return FormatMessagesForLLM(messages), nil
}

// formatMessagesAsMemoryContext formats messages for use as context (internal helper)
func formatMessagesAsMemoryContext(messages []Message) string {
	if len(messages) == 0 {
		return ""
	}

	var builder strings.Builder

	for _, msg := range messages {
		role := "User"
		if msg.IsBot {
			role = "Assistant"
		}
		builder.WriteString(fmt.Sprintf("[%s] %s (%s): %s\n",
			msg.Timestamp.Format("2006-01-02 15:04"),
			msg.Username,
			role,
			msg.Content))
	}

	return builder.String()
}

// Close closes the memory agent and its underlying store
func (m *MemoryAgent) Close() error {
	return m.store.Close()
}

// SetMaxContext sets the maximum number of messages to include in context
func (m *MemoryAgent) SetMaxContext(max int) {
	if max > 0 {
		m.maxContext = max
	}
}
