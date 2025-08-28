package memory

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/philippgille/chromem-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEmbeddingFunc is a simple mock embedding function for testing
// It creates embeddings based on word overlap for basic semantic similarity
func mockEmbeddingFunc(ctx context.Context, text string) ([]float32, error) {
	// Create a simple hash-based embedding for testing
	embedding := make([]float32, 384) // Standard embedding size

	// Convert text to lowercase for better matching
	text = strings.ToLower(text)

	// Create simple word-based features
	words := strings.Fields(text)
	for _, word := range words {
		// Hash each word to a position in the embedding
		hash := 0
		for _, char := range word {
			hash = (hash*31 + int(char)) % 384
		}
		embedding[hash] += 1.0

		// Add some overlap to neighboring positions for similarity
		if hash > 0 {
			embedding[hash-1] += 0.5
		}
		if hash < 383 {
			embedding[hash+1] += 0.5
		}
	}

	// Add character n-grams for better matching
	for i := 0; i < len(text)-2; i++ {
		trigram := text[i:min(i+3, len(text))]
		hash := 0
		for _, char := range trigram {
			hash = (hash*17 + int(char)) % 384
		}
		embedding[hash] += 0.3
	}

	// Normalize
	var sum float32
	for _, v := range embedding {
		sum += v * v
	}
	norm := float32(1.0)
	if sum > 0 {
		norm = 1.0 / float32(math.Sqrt(float64(sum)))
	}
	for i := range embedding {
		embedding[i] *= norm
	}

	return embedding, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestMemoryWorkflow tests the complete memory workflow with agents
func TestMemoryWorkflow(t *testing.T) {
	// Create a temporary directory for the test database
	tempDir, err := os.MkdirTemp("", "memory_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test_memory.db")

	// Create a memory agent with mock embedding function
	agent, err := NewMemoryAgentWithEmbedding(dbPath, DefaultConfig(), chromem.EmbeddingFunc(mockEmbeddingFunc))
	require.NoError(t, err)
	defer agent.Close()

	ctx := context.Background()
	integrationID := "test_integration"
	channelID := "test_channel"

	// Test Scenario: Complete workflow as described
	t.Run("CompleteWorkflow", func(t *testing.T) {
		// Step 1: Simulate storing some initial context/conversation history
		t.Log("Step 1: Storing initial conversation history")

		// Store some historical messages
		historicalMessages := []struct {
			userID   string
			username string
			content  string
			isBot    bool
		}{
			{"user1", "Alice", "What's the weather forecast for tomorrow?", false},
			{"bot", "Assistant", "Tomorrow's weather will be partly cloudy with temperatures around 72°F (22°C). There's a 20% chance of rain in the afternoon.", true},
			{"user1", "Alice", "Should I bring an umbrella?", false},
			{"bot", "Assistant", "Given the low 20% chance of rain, an umbrella is optional. You might want to bring a light one just in case.", true},
			{"user2", "Bob", "What are the best practices for error handling in Go?", false},
			{"bot", "Assistant", "In Go, best practices for error handling include: always check errors, wrap errors with context, use custom error types for specific cases, and avoid panic except in initialization.", true},
		}

		for _, msg := range historicalMessages {
			err = agent.ExtractAndStore(ctx, msg.content, integrationID, channelID, msg.userID, msg.username, msg.isBot)
			assert.NoError(t, err)
			time.Sleep(10 * time.Millisecond) // Small delay to ensure different timestamps
		}

		// Step 2: Receive a new prompt from the user
		t.Log("Step 2: Receiving new user prompt")
		userPrompt := "Can you tell me more about Go error handling patterns and what we discussed about the weather?"

		// Step 3: Use agent to search memory for relevant information
		t.Log("Step 3: Searching memory for relevant context")
		relevantContext, err := agent.SearchMemory(ctx, userPrompt, integrationID, channelID)
		assert.NoError(t, err)
		assert.NotEmpty(t, relevantContext, "Should find relevant context")
		t.Logf("Found relevant context:\n%s", relevantContext)

		// Step 4: Add context to the prompt
		t.Log("Step 4: Adding context to prompt")
		enhancedPrompt, err := agent.AddContextToPrompt(ctx, userPrompt, integrationID, channelID)
		assert.NoError(t, err)
		// Enhanced prompt should only contain memory context, not the original user prompt
		assert.NotContains(t, enhancedPrompt, userPrompt)
		assert.Contains(t, enhancedPrompt, "<memory>")
		t.Logf("Enhanced prompt:\n%s", enhancedPrompt)

		// Step 5: Simulate sending to an agent and getting a response
		t.Log("Step 5: Simulating agent response")
		// In a real scenario, this would be sent to an LLM agent
		agentResponse := "Based on our previous discussion about Go error handling best practices, the key patterns include wrapping errors with fmt.Errorf or using the errors package for context. As for the weather, I mentioned earlier that tomorrow will be partly cloudy with temperatures around 72°F and a 20% chance of rain, so an umbrella is optional."

		// Step 6: Extract and store the relevant information from the response
		t.Log("Step 6: Storing agent response in memory")
		err = agent.ExtractAndStore(ctx, agentResponse, integrationID, channelID, "bot", "Assistant", true)
		assert.NoError(t, err)

		// Step 7: Verify the complete conversation is stored
		t.Log("Step 7: Verifying stored conversation")
		summary, err := agent.GetConversationSummary(integrationID, channelID, 10)
		assert.NoError(t, err)
		assert.NotEmpty(t, summary)
		t.Logf("Conversation summary:\n%s", summary)

		// Verify we can search for the new information
		searchResults, err := agent.SearchMemory(ctx, "error wrapping patterns", integrationID, channelID)
		assert.NoError(t, err)
		assert.NotEmpty(t, searchResults, "Should find the newly stored response")
	})
}

// TestMemoryAgentSearching tests the search functionality
func TestMemoryAgentSearching(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "memory_search_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test_search.db")
	agent, err := NewMemoryAgentWithEmbedding(dbPath, DefaultConfig(), chromem.EmbeddingFunc(mockEmbeddingFunc))
	require.NoError(t, err)
	defer agent.Close()

	ctx := context.Background()
	integrationID := "test_integration"
	channelID := "test_channel"

	// Store various messages on different topics
	topics := []struct {
		content string
		user    string
	}{
		{"Python decorators are functions that modify other functions", "Dev1"},
		{"Machine learning models need proper validation sets", "Dev2"},
		{"Docker containers provide isolation for applications", "Dev1"},
		{"Kubernetes orchestrates container deployments", "Dev2"},
		{"React hooks allow state management in functional components", "Dev1"},
	}

	for _, topic := range topics {
		err = agent.ExtractAndStore(ctx, topic.content, integrationID, channelID, topic.user, topic.user, false)
		require.NoError(t, err)
	}

	// Test searching for specific topics
	tests := []struct {
		query    string
		expected string
	}{
		{"Python decorators", "functions that modify"},
		{"containers", "Docker"},
		{"React state management", "hooks"},
		{"machine learning validation", "validation sets"},
	}

	for _, tc := range tests {
		t.Run(tc.query, func(t *testing.T) {
			results, err := agent.SearchMemory(ctx, tc.query, integrationID, channelID)
			assert.NoError(t, err)
			assert.Contains(t, results, tc.expected, "Search results should contain expected content")
		})
	}
}

// TestMemoryPersistence tests that memory persists across agent instances
func TestMemoryPersistence(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "memory_persist_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "persist.db")
	ctx := context.Background()
	integrationID := "persist_test"
	channelID := "channel1"

	// First agent instance - store data
	{
		agent1, err := NewMemoryAgentWithEmbedding(dbPath, DefaultConfig(), chromem.EmbeddingFunc(mockEmbeddingFunc))
		require.NoError(t, err)

		err = agent1.ExtractAndStore(ctx, "Important information to remember", integrationID, channelID, "user1", "User", false)
		require.NoError(t, err)

		err = agent1.Close()
		require.NoError(t, err)
	}

	// Second agent instance - verify data persists
	{
		agent2, err := NewMemoryAgentWithEmbedding(dbPath, DefaultConfig(), chromem.EmbeddingFunc(mockEmbeddingFunc))
		require.NoError(t, err)
		defer agent2.Close()

		results, err := agent2.SearchMemory(ctx, "important information", integrationID, channelID)
		assert.NoError(t, err)
		assert.Contains(t, results, "Important information to remember")
	}
}

// TestConcurrentAccess tests concurrent read/write operations
func TestConcurrentAccess(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "memory_concurrent_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "concurrent.db")
	agent, err := NewMemoryAgentWithEmbedding(dbPath, DefaultConfig(), chromem.EmbeddingFunc(mockEmbeddingFunc))
	require.NoError(t, err)
	defer agent.Close()

	ctx := context.Background()
	integrationID := "concurrent_test"
	channelID := "channel1"

	// Run concurrent operations
	done := make(chan bool, 10)

	// Writers
	for i := 0; i < 5; i++ {
		go func(id int) {
			content := fmt.Sprintf("Message %d from goroutine", id)
			err := agent.ExtractAndStore(ctx, content, integrationID, channelID, fmt.Sprintf("user%d", id), fmt.Sprintf("User%d", id), false)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Readers
	for i := 0; i < 5; i++ {
		go func(id int) {
			_, err := agent.SearchMemory(ctx, fmt.Sprintf("Message %d", id), integrationID, channelID)
			assert.NoError(t, err)
			done <- true
		}(i)
	}

	// Wait for all operations to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify all messages were stored
	summary, err := agent.GetConversationSummary(integrationID, channelID, 10)
	assert.NoError(t, err)
	assert.NotEmpty(t, summary)
}
