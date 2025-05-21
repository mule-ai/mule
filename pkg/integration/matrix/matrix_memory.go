package matrix

import (
	"fmt"

	"github.com/mule-ai/mule/pkg/integration/memory"
)

// SetMemory assigns a memory manager to the Matrix integration
func (m *Matrix) SetMemory(mem *memory.Memory) {
	m.memory = mem
}

// GetChatHistory retrieves formatted chat history for the default room
func (m *Matrix) GetChatHistory(channelID string, limit int) (string, error) {
	if m.memory == nil {
		return "", fmt.Errorf("memory manager not initialized")
	}

	// If no specific channel ID is provided, use the default room ID
	if channelID == "" {
		channelID = m.config.RoomID
	}

	return m.memory.GetFormattedHistory(m.Name(), channelID, limit)
}

// ClearChatHistory removes chat history for the specified channel
func (m *Matrix) ClearChatHistory(channelID string) error {
	if m.memory == nil {
		return fmt.Errorf("memory manager not initialized")
	}

	// If no specific channel ID is provided, use the default room ID
	if channelID == "" {
		channelID = m.config.RoomID
	}

	return m.memory.ClearMessages(m.Name(), channelID)
}

// LogBotMessage logs a message sent by the bot to the memory system
func (m *Matrix) LogBotMessage(message string) {
	if m.memory == nil {
		return
	}

	botID := m.config.UserID
	botName := "Mule"

	if err := m.memory.SaveMessage(m.Name(), m.config.RoomID, botID, botName, message, true); err != nil {
		m.l.Error(err, "Failed to log bot message")
	}
}
