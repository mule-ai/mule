package memory

import (
	"fmt"
	"sort"
	"sync"
)

// InMemoryStore implements a simple in-memory store for chat messages
type InMemoryStore struct {
	messages      map[string]map[string][]Message // integrationID -> channelID -> []Message
	maxPerChannel int
	mu            sync.RWMutex
}

// NewInMemoryStore creates a new in-memory message store
func NewInMemoryStore(maxPerChannel int) *InMemoryStore {
	if maxPerChannel <= 0 {
		maxPerChannel = 100 // reasonable default
	}

	return &InMemoryStore{
		messages:      make(map[string]map[string][]Message),
		maxPerChannel: maxPerChannel,
		mu:            sync.RWMutex{},
	}
}

// channelKey returns the key for a specific channel in an integration
func (s *InMemoryStore) getChannelMessages(integID, channelID string) []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()

	channels, ok := s.messages[integID]
	if !ok {
		return []Message{}
	}

	messages, ok := channels[channelID]
	if !ok {
		return []Message{}
	}

	return messages
}

// SaveMessage stores a message in memory
func (s *InMemoryStore) SaveMessage(msg Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Initialize maps if needed
	if _, ok := s.messages[msg.IntegrationID]; !ok {
		s.messages[msg.IntegrationID] = make(map[string][]Message)
	}

	if _, ok := s.messages[msg.IntegrationID][msg.ChannelID]; !ok {
		s.messages[msg.IntegrationID][msg.ChannelID] = []Message{}
	}

	// Add the new message
	s.messages[msg.IntegrationID][msg.ChannelID] = append(
		s.messages[msg.IntegrationID][msg.ChannelID],
		msg,
	)

	// Trim if exceeding max messages per channel
	if len(s.messages[msg.IntegrationID][msg.ChannelID]) > s.maxPerChannel {
		s.messages[msg.IntegrationID][msg.ChannelID] = s.messages[msg.IntegrationID][msg.ChannelID][1:]
	}

	return nil
}

// GetRecentMessages retrieves the most recent messages for a channel
func (s *InMemoryStore) GetRecentMessages(integID, channelID string, limit int) ([]Message, error) {
	messages := s.getChannelMessages(integID, channelID)

	// Sort messages by timestamp (newest first)
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Timestamp.After(messages[j].Timestamp)
	})

	// Apply limit
	if limit > 0 && limit < len(messages) {
		messages = messages[:limit]
	}

	// Reverse the order so oldest messages come first
	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}

	return messages, nil
}

// ClearMessages removes all messages for a channel
func (s *InMemoryStore) ClearMessages(integID, channelID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if channels, ok := s.messages[integID]; ok {
		if _, ok := channels[channelID]; ok {
			channels[channelID] = []Message{}
			return nil
		}
		return fmt.Errorf("channel %s not found for integration %s", channelID, integID)
	}

	return fmt.Errorf("integration %s not found", integID)
}
