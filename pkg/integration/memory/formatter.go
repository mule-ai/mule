package memory

import (
	"fmt"
	"strings"
)

// FormatMessagesForLLM formats a list of messages for LLM context
func FormatMessagesForLLM(messages []Message) string {
	if len(messages) == 0 {
		return ""
	}

	var builder strings.Builder

	for i, msg := range messages {
		// Add a prefix based on whether it's a bot message
		prefix := "User"
		if msg.IsBot {
			prefix = "Assistant"
		}

		// Include username if available
		if msg.Username != "" && !msg.IsBot {
			prefix = fmt.Sprintf("%s (%s)", prefix, msg.Username)
		}

		// Format timestamp
		timestamp := msg.Timestamp.Format("2006-01-02 15:04:05")

		// Build the message line
		builder.WriteString(fmt.Sprintf("[%s] %s: %s\n", timestamp, prefix, msg.Content))

		// Add a separator line except for the last message
		if i < len(messages)-1 {
			builder.WriteString("---\n")
		}
	}

	return builder.String()
}

// FormatMessagesForMarkdown formats a list of messages for markdown display
func FormatMessagesForMarkdown(messages []Message) string {
	if len(messages) == 0 {
		return "*No previous messages*"
	}

	var builder strings.Builder
	builder.WriteString("### Previous Messages\n\n")

	for _, msg := range messages {
		// Add a prefix based on whether it's a bot message
		prefix := "**User"
		if msg.IsBot {
			prefix = "**Assistant"
		}

		// Include username if available
		if msg.Username != "" && !msg.IsBot {
			prefix = fmt.Sprintf("%s (%s)", prefix, msg.Username)
		}

		// Format timestamp
		timestamp := msg.Timestamp.Format("2006-01-02 15:04:05")

		// Build the message line
		builder.WriteString(fmt.Sprintf("*%s* %s**: %s\n\n", timestamp, prefix, msg.Content))
	}

	return builder.String()
}

// FormatMessagesAsContext formats messages as a compact context for an LLM prompt
func FormatMessagesAsContext(messages []Message) string {
	if len(messages) == 0 {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("Chat History:\n")

	for _, msg := range messages {
		role := "User"
		if msg.IsBot {
			role = "Assistant"
		}

		// Add username for non-bot messages if available
		if !msg.IsBot && msg.Username != "" {
			builder.WriteString(fmt.Sprintf("%s (%s): %s\n", role, msg.Username, msg.Content))
		} else {
			builder.WriteString(fmt.Sprintf("%s: %s\n", role, msg.Content))
		}
	}

	return builder.String()
}
