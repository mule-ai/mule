package memory

import (
	"time"

	"github.com/mule-ai/mule/pkg/types"
)

// Message represents a chat message
type Message struct {
	ID            string    `json:"id"`
	IntegrationID string    `json:"integration_id"` // "matrix", "discord", etc.
	ChannelID     string    `json:"channel_id"`     // Room ID or Discord channel
	UserID        string    `json:"user_id"`        // Who sent the message
	Username      string    `json:"username"`       // Display name of the user
	Content       string    `json:"content"`        // Message content
	Timestamp     time.Time `json:"timestamp"`      // When the message was received
	IsBot         bool      `json:"is_bot"`         // Whether the message is from the bot
}

// MemoryStore interface defines methods for storing and retrieving messages
type MemoryStore interface {
	// Save a message to the store
	SaveMessage(msg Message) error

	// Retrieve recent messages for a specific channel
	GetRecentMessages(integrationID, channelID string, limit int) ([]Message, error)

	// Clear messages for a specific channel
	ClearMessages(integrationID, channelID string) error
}

// Config holds configuration for the memory store
type Config struct {
	Enabled      bool `json:"enabled,omitempty"`
	MaxMessages  int  `json:"maxMessages,omitempty"`  // Maximum number of messages to store per channel
	DefaultLimit int  `json:"defaultLimit,omitempty"` // Default number of messages to retrieve
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:      true,
		MaxMessages:  100,
		DefaultLimit: 10,
	}
}

// Memory manages chat history for integrations
type Memory struct {
	config     *Config
	store      MemoryStore
	integTypes map[string]string // Map of integration to its type
}

// New creates a new Memory manager
func New(config *Config, store MemoryStore) *Memory {
	if config == nil {
		config = DefaultConfig()
	}

	return &Memory{
		config:     config,
		store:      store,
		integTypes: make(map[string]string),
	}
}

// RegisterIntegration adds an integration type to the memory manager
func (m *Memory) RegisterIntegration(integID, integType string) {
	m.integTypes[integID] = integType
}

// SaveMessage stores a message in the memory store
func (m *Memory) SaveMessage(integID, channelID, userID, username, content string, isBot bool) error {
	if !m.config.Enabled {
		return nil
	}

	msg := Message{
		ID:            GenerateID(),
		IntegrationID: integID,
		ChannelID:     channelID,
		UserID:        userID,
		Username:      username,
		Content:       content,
		Timestamp:     time.Now(),
		IsBot:         isBot,
	}

	return m.store.SaveMessage(msg)
}

// GetRecentMessages retrieves recent messages for a channel
func (m *Memory) GetRecentMessages(integID, channelID string, limit int) ([]Message, error) {
	if !m.config.Enabled {
		return []Message{}, nil
	}

	if limit <= 0 {
		limit = m.config.DefaultLimit
	}

	return m.store.GetRecentMessages(integID, channelID, limit)
}

// ClearMessages removes all messages for a channel
func (m *Memory) ClearMessages(integID, channelID string) error {
	return m.store.ClearMessages(integID, channelID)
}

// GenerateID creates a unique ID for a message
func GenerateID() string {
	return time.Now().Format("20060102150405.000000000")
}

// GetFormattedHistory returns a formatted string of recent messages suitable for LLM context
func (m *Memory) GetFormattedHistory(integID, channelID string, limit int) (string, error) {
	messages, err := m.GetRecentMessages(integID, channelID, limit)
	if err != nil {
		return "", err
	}

	return FormatMessagesForLLM(messages), nil
}

// AddTriggerSettings adds a message history to trigger settings
func (m *Memory) AddTriggerSettings(ts *types.TriggerSettings, integID, channelID string, limit int) error {
	if !m.config.Enabled {
		return nil
	}

	history, err := m.GetFormattedHistory(integID, channelID, limit)
	if err != nil {
		return err
	}

	// Add the history to the trigger data
	switch data := ts.Data.(type) {
	case string:
		messageWithHistory := "=== Previous Messages ===\n" + history + "\n=== Current Message ===\n" + data
		ts.Data = messageWithHistory
	case map[string]interface{}:
		data["history"] = history
		ts.Data = data
	default:
		// If we can't directly add the history, we create a new map
		newData := map[string]interface{}{
			"message": ts.Data,
			"history": history,
		}
		ts.Data = newData
	}

	return nil
}
