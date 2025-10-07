package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/mule-ai/mule/pkg/integration/memory"
)

func main() {
	// Setup database path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	dbPath := filepath.Join(homeDir, ".mule", "memory", "conversation.db")

	// Create memory agent
	fmt.Println("Initializing memory agent with persistent ChromeM database...")
	agent, err := memory.NewMemoryAgent(dbPath, memory.DefaultConfig())
	if err != nil {
		log.Fatalf("Failed to create memory agent: %v", err)
	}
	defer func() {
		if err := agent.Close(); err != nil {
			log.Printf("Failed to close agent: %v", err)
		}
	}()

	ctx := context.Background()
	integrationID := "example_integration"
	channelID := "example_channel"

	fmt.Println("\n=== Memory Integration Workflow Demo ===")

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
		}
	}
	fmt.Println("✓ Stored conversation history")

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
		fmt.Println("✓ Found relevant context from memory:")
		fmt.Printf("%s\n", relevantContext)
	}

	// Step 4: Add context to the prompt
	fmt.Println("\nStep 4: Enhancing prompt with context...")
	enhancedPrompt, err := agent.AddContextToPrompt(ctx, userPrompt, integrationID, channelID)
	if err != nil {
		log.Printf("Failed to enhance prompt: %v", err)
	} else {
		fmt.Println("✓ Enhanced prompt created")
		fmt.Printf("Enhanced prompt preview (first 200 chars): %.200s...\n", enhancedPrompt)
	}

	// Step 5: Simulate agent processing and response
	fmt.Println("\nStep 5: Sending enhanced prompt to agent (simulated)...")
	// In a real scenario, you would send enhancedPrompt to your LLM agent
	agentResponse := `Based on our previous discussions:

**Go's Concurrency Features:**
- Goroutines: Lightweight threads managed by Go runtime, started with 'go' keyword
- Channels: Used for communication between goroutines
- Thousands of goroutines can run concurrently due to their efficiency

**Error Handling in Go:**
- Errors are values returned from functions
- Always check errors immediately after function calls
- Use custom error types for specific cases
- Wrap errors with context using fmt.Errorf() for better debugging

These features make Go excellent for building concurrent, reliable applications.`

	fmt.Println("✓ Agent response received")

	// Step 6: Extract and store relevant information from response
	fmt.Println("\nStep 6: Storing agent response in memory...")
	err = agent.ExtractAndStore(ctx, agentResponse, integrationID, channelID, "bot", "Assistant", true)
	if err != nil {
		log.Printf("Failed to store response: %v", err)
	} else {
		fmt.Println("✓ Response stored in memory")
	}

	// Step 7: Send response to user and demonstrate persistence
	fmt.Println("\nStep 7: Final response to user:")
	fmt.Printf("Assistant: %s\n", agentResponse)

	// Demonstrate memory persistence
	fmt.Println("\n=== Demonstrating Memory Persistence ===")
	fmt.Println("\nFetching conversation summary from memory...")
	summary, err := agent.GetConversationSummary(integrationID, channelID, 5)
	if err != nil {
		log.Printf("Failed to get summary: %v", err)
	} else {
		fmt.Printf("\nLast 5 messages from conversation:\n%s\n", summary)
	}

	// Test semantic search
	fmt.Println("\n=== Testing Semantic Search ===")
	testQueries := []string{
		"goroutines and concurrency",
		"error handling best practices",
		"Go language features",
	}

	for _, query := range testQueries {
		fmt.Printf("\nSearching for: '%s'\n", query)
		results, err := agent.SearchMemory(ctx, query, integrationID, channelID)
		if err != nil {
			log.Printf("Search failed: %v", err)
		} else if results != "" {
			fmt.Printf("Found: %.150s...\n", results)
		} else {
			fmt.Println("No results found")
		}
	}

	fmt.Println("\n=== Workflow Complete ===")
	fmt.Printf("Memory is persisted at: %s\n", dbPath)
	fmt.Println("Run this program again to see that the memory persists across sessions!")
}
