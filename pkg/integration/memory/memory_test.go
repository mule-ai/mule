package memory

import (
	"strings"
	"testing"
	"time"

	"github.com/mule-ai/mule/pkg/integration/types"
)

func TestInMemoryStore(t *testing.T) {
	// Create a new store with a max of 5 messages per channel
	store := NewInMemoryStore(5)

	// Test saving and retrieving messages
	t.Run("SaveAndRetrieveMessages", func(t *testing.T) {
		// Save some test messages
		for i := 0; i < 3; i++ {
			msg := Message{
				ID:            GenerateID(),
				IntegrationID: "test-integration",
				ChannelID:     "test-channel",
				UserID:        "user1",
				Username:      "Test User",
				Content:       "Test message " + GenerateID(),
				Timestamp:     time.Now().Add(time.Duration(-i) * time.Minute),
				IsBot:         i%2 == 0, // Even messages are from bot
			}

			err := store.SaveMessage(msg)
			if err != nil {
				t.Fatalf("Failed to save message: %v", err)
			}
		}

		// Retrieve messages
		messages, err := store.GetRecentMessages("test-integration", "test-channel", 10)
		if err != nil {
			t.Fatalf("Failed to retrieve messages: %v", err)
		}

		// Check if we have the right number of messages
		if len(messages) != 3 {
			t.Fatalf("Expected 3 messages, got %d", len(messages))
		}

		// Check if messages are in chronological order (oldest first)
		for i := 0; i < len(messages)-1; i++ {
			if messages[i].Timestamp.After(messages[i+1].Timestamp) {
				t.Errorf("Messages not in chronological order: %v is after %v",
					messages[i].Timestamp, messages[i+1].Timestamp)
			}
		}

		// Check if message properties are preserved
		for _, msg := range messages {
			if msg.IntegrationID != "test-integration" {
				t.Errorf("Integration ID mismatch: expected 'test-integration', got '%s'", msg.IntegrationID)
			}
			if msg.ChannelID != "test-channel" {
				t.Errorf("Channel ID mismatch: expected 'test-channel', got '%s'", msg.ChannelID)
			}
		}
	})

	// Test limit enforcement
	t.Run("EnforceMessageLimit", func(t *testing.T) {
		// Create a new store with a max of 3 messages
		limitedStore := NewInMemoryStore(3)

		// Save 5 messages (exceeding the limit)
		for i := 0; i < 5; i++ {
			msg := Message{
				ID:            GenerateID(),
				IntegrationID: "test-integration",
				ChannelID:     "test-channel",
				UserID:        "user1",
				Username:      "Test User",
				Content:       "Message " + GenerateID(),
				Timestamp:     time.Now().Add(time.Duration(i) * time.Second),
				IsBot:         false,
			}

			err := limitedStore.SaveMessage(msg)
			if err != nil {
				t.Fatalf("Failed to save message: %v", err)
			}
		}

		// Retrieve messages - should only get the most recent 3
		messages, err := limitedStore.GetRecentMessages("test-integration", "test-channel", 10)
		if err != nil {
			t.Fatalf("Failed to retrieve messages: %v", err)
		}

		// Check if we have only 3 messages (the limit)
		if len(messages) != 3 {
			t.Fatalf("Expected 3 messages after limit enforcement, got %d", len(messages))
		}
	})

	// Test clear functionality
	t.Run("ClearMessages", func(t *testing.T) {
		// Create a new store
		clearStore := NewInMemoryStore(10)

		// Save a few messages
		for i := 0; i < 3; i++ {
			msg := Message{
				ID:            GenerateID(),
				IntegrationID: "test-integration",
				ChannelID:     "test-channel",
				UserID:        "user1",
				Username:      "Test User",
				Content:       "Clear test message " + GenerateID(),
				Timestamp:     time.Now(),
				IsBot:         false,
			}

			err := clearStore.SaveMessage(msg)
			if err != nil {
				t.Fatalf("Failed to save message: %v", err)
			}
		}

		// Clear messages
		err := clearStore.ClearMessages("test-integration", "test-channel")
		if err != nil {
			t.Fatalf("Failed to clear messages: %v", err)
		}

		// Try to retrieve messages - should be empty
		messages, err := clearStore.GetRecentMessages("test-integration", "test-channel", 10)
		if err != nil {
			t.Fatalf("Failed to retrieve messages after clearing: %v", err)
		}

		// Check if the channel is empty
		if len(messages) != 0 {
			t.Fatalf("Expected 0 messages after clearing, got %d", len(messages))
		}
	})
}

func TestMemory(t *testing.T) {
	// Create a memory manager with default config
	config := DefaultConfig()
	store := NewInMemoryStore(config.MaxMessages)
	memory := New(config, store)

	// Test saving and retrieving messages through the Memory manager
	t.Run("SaveAndRetrieveMessages", func(t *testing.T) {
		// Register an integration
		memory.RegisterIntegration("matrix", "matrix")

		// Save some messages
		for i := 0; i < 3; i++ {
			err := memory.SaveMessage("matrix", "room123", "user1", "User One",
				"Test message "+GenerateID(), i%2 == 0)
			if err != nil {
				t.Fatalf("Failed to save message: %v", err)
			}
		}

		// Get formatted history
		history, err := memory.GetFormattedHistory("matrix", "room123", 10)
		if err != nil {
			t.Fatalf("Failed to get formatted history: %v", err)
		}

		// Check if we got any history
		if history == "" {
			t.Fatal("Expected non-empty history, got empty string")
		}
	})

	// Test adding history to trigger settings
	t.Run("AddHistoryToTriggerSettings", func(t *testing.T) {
		// Create a trigger setting with string data
		ts := &types.TriggerSettings{
			Integration: "matrix",
			Event:       "newMessage",
			Data:        "Hello world",
		}

		// Add history to the trigger settings
		err := memory.AddTriggerSettings(ts, "matrix", "room123", 10)
		if err != nil {
			t.Fatalf("Failed to add history to trigger settings: %v", err)
		}

		// Check if data was modified
		data, ok := ts.Data.(string)
		if !ok {
			t.Fatalf("Expected data to be string, got %T", ts.Data)
		}

		// Should start with our history header
		if len(data) < 10 || data[:10] != "=== Previo" {
			t.Fatalf("Expected data to start with history header, got: %s", data[:10])
		}
	})
}

func TestFormatter(t *testing.T) {
	// Create some test messages
	now := time.Now()
	messages := []Message{
		{
			ID:            "1",
			IntegrationID: "test",
			ChannelID:     "channel1",
			UserID:        "user1",
			Username:      "Alice",
			Content:       "Hello world",
			Timestamp:     now.Add(-5 * time.Minute),
			IsBot:         false,
		},
		{
			ID:            "2",
			IntegrationID: "test",
			ChannelID:     "channel1",
			UserID:        "bot1",
			Username:      "Bot",
			Content:       "How can I help you?",
			Timestamp:     now.Add(-4 * time.Minute),
			IsBot:         true,
		},
		{
			ID:            "3",
			IntegrationID: "test",
			ChannelID:     "channel1",
			UserID:        "user1",
			Username:      "Alice",
			Content:       "I need assistance",
			Timestamp:     now.Add(-3 * time.Minute),
			IsBot:         false,
		},
	}

	// Test different formatting options
	t.Run("FormatForLLM", func(t *testing.T) {
		formatted := FormatMessagesForLLM(messages)
		if formatted == "" {
			t.Fatal("Expected non-empty formatted messages")
		}

		// Quick check to make sure it contains expected content
		for _, needle := range []string{"User (Alice)", "Assistant", "Hello world", "How can I help you?"} {
			if !contains(formatted, needle) {
				t.Errorf("Expected formatted output to contain '%s', but didn't find it", needle)
			}
		}
	})

	t.Run("FormatForMarkdown", func(t *testing.T) {
		formatted := FormatMessagesForMarkdown(messages)
		if formatted == "" {
			t.Fatal("Expected non-empty formatted messages")
		}

		// Check for markdown formatting indicators
		for _, needle := range []string{"###", "**User", "**Assistant"} {
			if !contains(formatted, needle) {
				t.Errorf("Expected markdown to contain '%s', but didn't find it", needle)
			}
		}
	})

	t.Run("FormatAsContext", func(t *testing.T) {
		formatted := FormatMessagesAsContext(messages)
		if formatted == "" {
			t.Fatal("Expected non-empty formatted messages")
		}

		// Check for expected format
		if !contains(formatted, "Chat History:") {
			t.Error("Expected context to start with 'Chat History:'")
		}
	})
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	if s == "" || substr == "" {
		return false
	}
	return strings.Contains(s, substr)
}
