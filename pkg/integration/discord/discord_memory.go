package discord

import (
	"fmt"

	"github.com/mule-ai/mule/pkg/integration/memory"
)

// SetMemory assigns a memory manager to the Discord integration
func (d *Discord) SetMemory(mem *memory.Memory) {
	d.memory = mem
}

// GetChatHistory retrieves formatted chat history for the specified channel
func (d *Discord) GetChatHistory(channelID string, limit int) (string, error) {
	if d.memory == nil {
		return "", fmt.Errorf("memory manager not initialized")
	}

	// If no specific channel ID is provided, use the default channel ID
	if channelID == "" {
		channelID = d.config.ChannelID
	}

	return d.memory.GetFormattedHistory(d.Name(), channelID, limit)
}

// ClearChatHistory removes chat history for the specified channel
func (d *Discord) ClearChatHistory(channelID string) error {
	if d.memory == nil {
		return fmt.Errorf("memory manager not initialized")
	}

	// If no specific channel ID is provided, use the default channel ID
	if channelID == "" {
		channelID = d.config.ChannelID
	}

	return d.memory.ClearMessages(d.Name(), channelID)
}

// LogBotMessage logs a message sent by the bot to the memory system
func (d *Discord) LogBotMessage(channelID, message string) {
	if d.memory == nil {
		return
	}

	if channelID == "" {
		channelID = d.config.ChannelID
	}

	botID := "bot" // Discord bot ID
	botName := "Mule"

	// Check if we have a session to get the actual bot ID
	if d.session != nil && d.session.State != nil && d.session.State.User != nil {
		botID = d.session.State.User.ID
		botName = d.session.State.User.Username
	}

	if err := d.memory.SaveMessage(d.Name(), channelID, botID, botName, message, true); err != nil {
		d.l.Error(err, "Failed to log bot message")
	}
}

// addHistoryToMessage adds chat history context to a message
func (d *Discord) addHistoryToMessage(message, channelID string) any {
	if d.memory == nil {
		return message
	}

	if channelID == "" {
		channelID = d.config.ChannelID
	}

	// Get chat history and format it with the current message
	history, err := d.memory.GetFormattedHistory(d.Name(), channelID, 10)
	if err != nil {
		d.l.Error(err, "Failed to get chat history")
		return message
	}

	// If no history, just return the message
	if history == "" {
		return message
	}

	// Format with history
	messageWithHistory := fmt.Sprintf("=== Previous Messages ===\n%s\n=== Current Message ===\n%s",
		history, message)

	return messageWithHistory
}
