package memory

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/philippgille/chromem-go"
)

// ChromeMStore implements MemoryStore using ChromeM with proper v0.7.0 methods
type ChromeMStore struct {
	db         *chromem.DB
	collection *chromem.Collection
	mu         sync.RWMutex
	maxEntries int
	dbPath     string
}

// NewChromeMStore creates a ChromeM store using only methods available in v0.7.0
func NewChromeMStore(dbPath string, maxEntries int) (*ChromeMStore, error) {
	return NewChromeMStoreWithEmbedding(dbPath, maxEntries, nil)
}

// NewChromeMStoreWithEmbedding creates a ChromeM store with custom embedding function
func NewChromeMStoreWithEmbedding(dbPath string, maxEntries int, embeddingFunc chromem.EmbeddingFunc) (*ChromeMStore, error) {
	// Ensure the database directory exists
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Create or open the persistent database
	db, err := chromem.NewPersistentDB(dbPath, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create/open ChromeM database: %w", err)
	}

	// If no embedding function provided, use default
	if embeddingFunc == nil {
		embeddingFunc = chromem.NewEmbeddingFuncDefault()
	}

	// Get or create the memory collection
	collection := db.GetCollection("chat_memory", embeddingFunc)
	if collection == nil {
		// Collection doesn't exist, create it
		collection, err = db.CreateCollection("chat_memory", nil, embeddingFunc)
		if err != nil {
			return nil, fmt.Errorf("failed to create collection: %w", err)
		}
	}

	return &ChromeMStore{
		db:         db,
		collection: collection,
		mu:         sync.RWMutex{},
		maxEntries: maxEntries,
		dbPath:     dbPath,
	}, nil
}

// GetMessageCount returns the total number of messages in the store
func (c *ChromeMStore) GetMessageCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.collection.Count()
}

// ListAllMessages retrieves messages using the working approach from the original implementation
func (c *ChromeMStore) ListAllMessages(integrationID, channelID string, limit int) ([]Message, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ctx := context.Background()

	// Get total count first
	totalCount := c.collection.Count()
	if totalCount == 0 {
		return []Message{}, nil
	}

	// Build metadata filter
	where := make(map[string]string)
	if integrationID != "" {
		where["integration_id"] = integrationID
	}
	if channelID != "" {
		where["channel_id"] = channelID
	}

	// Use search terms that should match all documents
	searchTerms := []string{"User:", "Channel:", "Message:"}
	allMessages := make(map[string]Message)

	for _, searchTerm := range searchTerms {
		queryLimit := totalCount

		results, err := c.collection.Query(ctx, searchTerm, queryLimit, where, nil)
		if err != nil {
			if strings.Contains(err.Error(), "nResults must be <=") {
				// Try with progressively smaller limits
				for tryLimit := totalCount; tryLimit >= 1; tryLimit-- {
					results, err = c.collection.Query(ctx, searchTerm, tryLimit, where, nil)
					if err == nil {
						break
					}
					if !strings.Contains(err.Error(), "nResults must be <=") {
						break
					}
				}
			}
			if err != nil {
				continue
			}
		}

		// Process results
		for _, result := range results {
			if _, exists := allMessages[result.ID]; exists {
				continue
			}

			// Extract the actual message content
			content := result.Content
			lines := strings.Split(content, "\n")
			for _, line := range lines {
				if len(line) > 8 && line[:8] == "Message:" {
					content = strings.TrimSpace(line[9:])
					break
				}
			}

			timestamp, _ := time.Parse(time.RFC3339, result.Metadata["timestamp"])
			isBot := result.Metadata["is_bot"] == "true"

			msg := Message{
				ID:            result.ID,
				IntegrationID: result.Metadata["integration_id"],
				ChannelID:     result.Metadata["channel_id"],
				UserID:        result.Metadata["user_id"],
				Username:      result.Metadata["username"],
				Content:       content,
				Timestamp:     timestamp,
				IsBot:         isBot,
			}
			allMessages[result.ID] = msg
		}

		if limit > 0 && len(allMessages) >= limit {
			break
		}
	}

	// Convert to slice and sort
	messages := make([]Message, 0, len(allMessages))
	for _, msg := range allMessages {
		messages = append(messages, msg)
	}

	// Sort by timestamp (most recent first)
	for i := 0; i < len(messages)-1; i++ {
		for j := i + 1; j < len(messages); j++ {
			if messages[i].Timestamp.Before(messages[j].Timestamp) {
				messages[i], messages[j] = messages[j], messages[i]
			}
		}
	}

	// Apply final limit
	if limit > 0 && len(messages) > limit {
		messages = messages[:limit]
	}

	return messages, nil
}

// SaveMessage saves a message to the ChromeM vector store (same as original)
func (c *ChromeMStore) SaveMessage(msg Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	ctx := context.Background()

	doc := chromem.Document{
		ID: msg.ID,
		Content: fmt.Sprintf("User: %s\nChannel: %s\nMessage: %s",
			msg.Username,
			msg.ChannelID,
			msg.Content),
		Metadata: map[string]string{
			"integration_id": msg.IntegrationID,
			"channel_id":     msg.ChannelID,
			"user_id":        msg.UserID,
			"username":       msg.Username,
			"timestamp":      msg.Timestamp.Format(time.RFC3339),
			"is_bot":         fmt.Sprintf("%t", msg.IsBot),
		},
	}

	if err := c.collection.AddDocument(ctx, doc); err != nil {
		return fmt.Errorf("failed to add document to ChromeM: %w", err)
	}

	return nil
}

// SearchMessages performs semantic search (same as original)
func (c *ChromeMStore) SearchMessages(query string, integrationID, channelID string, limit int) ([]Message, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ctx := context.Background()

	where := make(map[string]string)
	if integrationID != "" {
		where["integration_id"] = integrationID
	}
	if channelID != "" {
		where["channel_id"] = channelID
	}

	results, err := c.collection.Query(ctx, query, limit, where, nil)
	if err != nil {
		if strings.Contains(err.Error(), "nResults must be <=") {
			for tryLimit := 1; tryLimit <= limit; tryLimit++ {
				results, err = c.collection.Query(ctx, query, tryLimit, where, nil)
				if err == nil {
					break
				}
				if !strings.Contains(err.Error(), "nResults must be <=") {
					return nil, fmt.Errorf("failed to search ChromeM: %w", err)
				}
			}
			if err != nil {
				return []Message{}, nil
			}
		} else {
			return nil, fmt.Errorf("failed to search ChromeM: %w", err)
		}
	}

	messages := make([]Message, 0, len(results))
	for _, result := range results {
		content := result.Content
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			if len(line) > 8 && line[:8] == "Message:" {
				content = strings.TrimSpace(line[9:])
				break
			}
		}

		timestamp, _ := time.Parse(time.RFC3339, result.Metadata["timestamp"])
		isBot := result.Metadata["is_bot"] == "true"

		msg := Message{
			ID:            result.ID,
			IntegrationID: result.Metadata["integration_id"],
			ChannelID:     result.Metadata["channel_id"],
			UserID:        result.Metadata["user_id"],
			Username:      result.Metadata["username"],
			Content:       content,
			Timestamp:     timestamp,
			IsBot:         isBot,
		}
		messages = append(messages, msg)
	}

	return messages, nil
}

// GetRecentMessages (placeholder - uses search)
func (c *ChromeMStore) GetRecentMessages(integrationID, channelID string, limit int) ([]Message, error) {
	return c.SearchMessages("Message:", integrationID, channelID, limit)
}

// ClearMessages (placeholder)
func (c *ChromeMStore) ClearMessages(integrationID, channelID string) error {
	return fmt.Errorf("clear messages not yet implemented for ChromeM store")
}

// Close closes the database
func (c *ChromeMStore) Close() error {
	return nil
}

// NewLocalEmbeddingFunc returns a local embedding function
func NewLocalEmbeddingFunc() chromem.EmbeddingFunc {
	return func(ctx context.Context, text string) ([]float32, error) {
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
			norm := 1.0 / float32(math.Sqrt(float64(sum)))
			for i := range embedding {
				embedding[i] *= norm
			}
		}

		return embedding, nil
	}
}
