package tasks

import (
	"fmt"
)

// GetChatHistory returns an error as the tasks integration doesn't support chat history
func (t *Tasks) GetChatHistory(channelID string, limit int) (string, error) {
	return "", fmt.Errorf("chat history not supported by tasks integration")
}

// ClearChatHistory returns an error as the tasks integration doesn't support chat history
func (t *Tasks) ClearChatHistory(channelID string) error {
	return fmt.Errorf("chat history not supported by tasks integration")
}
