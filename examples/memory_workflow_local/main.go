package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/mule-ai/mule/pkg/integration/memory"
	"github.com/philippgille/chromem-go"
)

// localEmbeddingFunc creates embeddings locally without API calls
func localEmbeddingFunc(ctx context.Context, text string) ([]float32, error) {
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

func main() {
	// Setup database path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	dbPath := filepath.Join(homeDir, ".mule", "memory", "conversation_local.db")

	// Create memory agent with local embedding
	fmt.Println("Initializing memory agent with local embeddings (no API required)...")
	agent, err := memory.NewMemoryAgentWithEmbedding(dbPath, memory.DefaultConfig(), chromem.EmbeddingFunc(localEmbeddingFunc))
	if err != nil {
		log.Fatalf("Failed to create memory agent: %v", err)
	}
	defer agent.Close()

	ctx := context.Background()
	integrationID := "example_integration"
	channelID := "example_channel"

	fmt.Println("\n=== Memory Integration Workflow Demo (Local) ===")

	// Step 1: Store some initial conversation history
	fmt.Println("Step 1: Storing initial conversation history...")
	conversations := []struct {
		user    string
		content string
		isBot   bool
	}{
		{"Alice", "What are the key features of Go language?", false},
		{"Assistant", "Go's key features include: goroutines for concurrency, channels for communication, fast compilation, garbage collection, and a simple syntax.", true},
		{"Bob", "How do I handle errors in Go?", false},
		{"Assistant", "In Go, errors are values returned from functions. Check errors immediately after function calls, use custom error types when needed, and wrap errors with context using fmt.Errorf.", true},
		{"Alice", "Can you explain goroutines?", false},
		{"Assistant", "Goroutines are lightweight threads managed by the Go runtime. Start them with the 'go' keyword. They're more efficient than OS threads, allowing thousands to run concurrently.", true},
	}

	for _, conv := range conversations {
		userID := conv.user
		if conv.isBot {
			userID = "bot"
		}
		err = agent.ExtractAndStore(ctx, conv.content, integrationID, channelID, userID, conv.user, conv.isBot)
		if err != nil {
			log.Printf("Failed to store message: %v", err)
		} else {
			fmt.Printf("  ✓ Stored: %.50s...\n", conv.content)
		}
	}

	// Step 2: Receive a new prompt from user
	fmt.Println("\nStep 2: New user prompt received:")
	userPrompt := "Can you summarize what we've discussed about Go's concurrency features and error handling?"
	fmt.Printf("User: %s\n", userPrompt)

	// Step 3: Search memory for relevant information
	fmt.Println("\nStep 3: Searching memory for relevant context...")
	relevantContext, err := agent.SearchMemory(ctx, userPrompt, integrationID, channelID)
	if err != nil {
		log.Printf("Failed to search memory: %v", err)
	} else if relevantContext != "" {
		fmt.Println("✓ Found relevant context from memory")
		fmt.Printf("Context preview (first 200 chars): %.200s...\n", relevantContext)
	} else {
		fmt.Println("No relevant context found in memory")
	}

	// Step 4: Add context to the prompt
	fmt.Println("\nStep 4: Enhancing prompt with context...")
	enhancedPrompt, err := agent.AddContextToPrompt(ctx, userPrompt, integrationID, channelID)
	if err != nil {
		log.Printf("Failed to enhance prompt: %v", err)
	} else {
		fmt.Println("✓ Enhanced prompt created")
		if enhancedPrompt != userPrompt {
			fmt.Println("Prompt was enhanced with memory context")
		}
	}

	// Step 5: Simulate agent processing and response
	fmt.Println("\nStep 5: Generating response (simulated)...")
	agentResponse := `Based on our previous discussions:

**Go's Concurrency Features:**
- Goroutines: Lightweight threads managed by Go runtime, started with 'go' keyword
- Channels: Used for communication between goroutines
- Thousands of goroutines can run concurrently due to their efficiency

**Error Handling in Go:**
- Errors are values returned from functions
- Always check errors immediately after function calls
- Use custom error types for specific cases
- Wrap errors with context using fmt.Errorf() for better debugging`

	fmt.Println("✓ Response generated")

	// Step 6: Store the response in memory
	fmt.Println("\nStep 6: Storing response in memory...")
	err = agent.ExtractAndStore(ctx, agentResponse, integrationID, channelID, "bot", "Assistant", true)
	if err != nil {
		log.Printf("Failed to store response: %v", err)
	} else {
		fmt.Println("✓ Response stored in memory")
	}

	// Step 7: Demonstrate memory retrieval
	fmt.Println("\n=== Demonstrating Memory Retrieval ===")
	fmt.Println("\nFetching conversation summary...")
	summary, err := agent.GetConversationSummary(integrationID, channelID, 3)
	if err != nil {
		log.Printf("Failed to get summary: %v", err)
	} else if summary != "" {
		fmt.Printf("\nRecent conversation (last 3 messages):\n%s\n", summary)
	} else {
		fmt.Println("No conversation history found")
	}

	// Test semantic search
	fmt.Println("\n=== Testing Semantic Search ===")
	testQueries := []string{
		"goroutines",
		"error handling",
		"concurrency",
	}

	for _, query := range testQueries {
		fmt.Printf("\nSearching for: '%s'\n", query)
		results, err := agent.SearchMemory(ctx, query, integrationID, channelID)
		if err != nil {
			log.Printf("Search failed: %v", err)
		} else if results != "" {
			// Show just first line of results
			lines := strings.Split(results, "\n")
			if len(lines) > 0 {
				fmt.Printf("Found match: %.100s...\n", lines[0])
			}
		} else {
			fmt.Println("No results found")
		}
	}

	fmt.Println("\n=== Workflow Complete ===")
	fmt.Printf("Memory is persisted at: %s\n", dbPath)
	fmt.Println("This example uses local embeddings - no API key required!")
	fmt.Println("Run this program again to see that the memory persists across sessions.")
}
