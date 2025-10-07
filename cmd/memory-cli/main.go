package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/mule-ai/mule/pkg/integration/memory"
	"github.com/spf13/cobra"
)

var (
	dbPath      string
	maxMessages int
)

func main() {
	var rootCmd = &cobra.Command{
		Use:   "memory-cli",
		Short: "CLI tool for managing ChromeM memory store",
		Long: `A command line interface for performing operations on the ChromeM memory store.
Supports listing, adding, deleting, and querying memories.`,
	}

	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "/tmp/mule_memory.db", "Path to ChromeM database")
	rootCmd.PersistentFlags().IntVar(&maxMessages, "max", 1000, "Maximum number of messages to store")

	// Add subcommands
	rootCmd.AddCommand(
		createListCmd(),
		createAddCmd(),
		createDeleteCmd(),
		createQueryCmd(),
		createStatsCmd(),
		createClearCmd(),
	)

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}

func createListCmd() *cobra.Command {
	var (
		limit         int
		integrationID string
		channelID     string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List memories from the store",
		Long:  "List memories from the ChromeM store with optional filtering",
		Run: func(cmd *cobra.Command, args []string) {
			store, err := memory.NewChromeMStoreWithEmbedding(dbPath, maxMessages, memory.NewLocalEmbeddingFunc())
			if err != nil {
				log.Fatalf("Failed to open store: %v", err)
			}
			defer func() {
				if err := store.Close(); err != nil {
					log.Printf("Failed to close store: %v", err)
				}
			}()

			// Use the proper ListAllMessages method for listing
			messages, err := store.ListAllMessages(integrationID, channelID, limit)
			if err != nil {
				log.Fatalf("Failed to list messages: %v", err)
			}

			if len(messages) == 0 {
				fmt.Println("No memories found")
				return
			}

			fmt.Printf("Found %d memories:\n\n", len(messages))
			for i, msg := range messages {
				role := "User"
				if msg.IsBot {
					role = "Bot"
				}
				fmt.Printf("%d. [%s] %s (%s) - %s\n",
					i+1,
					msg.Timestamp.Format("2006-01-02 15:04:05"),
					msg.Username,
					role,
					msg.ChannelID,
				)
				fmt.Printf("   ID: %s\n", msg.ID)
				fmt.Printf("   Integration: %s\n", msg.IntegrationID)

				// Truncate long content for list view
				content := msg.Content
				if len(content) > 100 {
					content = content[:100] + "..."
				}
				fmt.Printf("   Content: %s\n\n", content)
			}
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "l", 10, "Maximum number of memories to list")
	cmd.Flags().StringVar(&integrationID, "integration", "", "Filter by integration ID")
	cmd.Flags().StringVar(&channelID, "channel", "", "Filter by channel ID")

	return cmd
}

func createAddCmd() *cobra.Command {
	var (
		content       string
		integrationID string
		channelID     string
		userID        string
		username      string
		isBot         bool
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a memory to the store",
		Long:  "Add a new memory entry to the ChromeM store",
		Run: func(cmd *cobra.Command, args []string) {
			if content == "" {
				log.Fatal("Content is required. Use --content flag.")
			}

			store, err := memory.NewChromeMStoreWithEmbedding(dbPath, maxMessages, memory.NewLocalEmbeddingFunc())
			if err != nil {
				log.Fatalf("Failed to open store: %v", err)
			}
			defer func() {
				if err := store.Close(); err != nil {
					log.Printf("Failed to close store: %v", err)
				}
			}()

			msg := memory.Message{
				ID:            memory.GenerateID(),
				IntegrationID: integrationID,
				ChannelID:     channelID,
				UserID:        userID,
				Username:      username,
				Content:       content,
				Timestamp:     time.Now(),
				IsBot:         isBot,
			}

			if err := store.SaveMessage(msg); err != nil {
				log.Fatalf("Failed to save memory: %v", err)
			}

			fmt.Printf("Memory added successfully!\n")
			fmt.Printf("ID: %s\n", msg.ID)
			fmt.Printf("Timestamp: %s\n", msg.Timestamp.Format("2006-01-02 15:04:05"))
		},
	}

	cmd.Flags().StringVarP(&content, "content", "c", "", "Memory content (required)")
	cmd.Flags().StringVar(&integrationID, "integration", "cli", "Integration ID")
	cmd.Flags().StringVar(&channelID, "channel", "default", "Channel ID")
	cmd.Flags().StringVar(&userID, "user-id", "user", "User ID")
	cmd.Flags().StringVar(&username, "username", "User", "Username")
	cmd.Flags().BoolVar(&isBot, "bot", false, "Mark as bot message")

	return cmd
}

func createDeleteCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete [message-id]",
		Short: "Delete a memory from the store",
		Long:  "Delete a specific memory by its ID",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			messageID := args[0]

			// ChromeM doesn't have direct delete by ID functionality
			// This is a limitation we need to acknowledge
			fmt.Printf("Delete functionality is not yet implemented for ChromeM store.\n")
			fmt.Printf("ChromeM doesn't support direct deletion of individual documents by ID.\n")
			fmt.Printf("Message ID requested for deletion: %s\n", messageID)
			fmt.Printf("\nAlternative options:\n")
			fmt.Printf("1. Use 'clear' command to clear all memories for a specific channel\n")
			fmt.Printf("2. Recreate the database by deleting the file: %s\n", dbPath)
		},
	}

	return cmd
}

func createQueryCmd() *cobra.Command {
	var (
		query         string
		limit         int
		integrationID string
		channelID     string
	)

	cmd := &cobra.Command{
		Use:   "query",
		Short: "Search memories using semantic search",
		Long:  "Search for memories using semantic similarity search",
		Run: func(cmd *cobra.Command, args []string) {
			if query == "" {
				log.Fatal("Query is required. Use --query flag.")
			}

			store, err := memory.NewChromeMStoreWithEmbedding(dbPath, maxMessages, memory.NewLocalEmbeddingFunc())
			if err != nil {
				log.Fatalf("Failed to open store: %v", err)
			}
			defer func() {
				if err := store.Close(); err != nil {
					log.Printf("Failed to close store: %v", err)
				}
			}()

			messages, err := store.SearchMessages(query, integrationID, channelID, limit)
			if err != nil {
				log.Fatalf("Failed to search memories: %v", err)
			}

			if len(messages) == 0 {
				fmt.Printf("No memories found for query: %s\n", query)
				return
			}

			fmt.Printf("Found %d memories for query: %s\n\n", len(messages), query)
			for i, msg := range messages {
				role := "User"
				if msg.IsBot {
					role = "Bot"
				}
				fmt.Printf("%d. [%s] %s (%s) - %s\n",
					i+1,
					msg.Timestamp.Format("2006-01-02 15:04:05"),
					msg.Username,
					role,
					msg.ChannelID,
				)
				fmt.Printf("   ID: %s\n", msg.ID)
				fmt.Printf("   Integration: %s\n", msg.IntegrationID)
				fmt.Printf("   Content: %s\n\n", msg.Content)
			}
		},
	}

	cmd.Flags().StringVarP(&query, "query", "q", "", "Search query (required)")
	cmd.Flags().IntVarP(&limit, "limit", "l", 10, "Maximum number of results")
	cmd.Flags().StringVar(&integrationID, "integration", "", "Filter by integration ID")
	cmd.Flags().StringVar(&channelID, "channel", "", "Filter by channel ID")

	return cmd
}

func createStatsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "stats",
		Short: "Show database statistics",
		Long:  "Display statistics about the ChromeM memory database",
		Run: func(cmd *cobra.Command, args []string) {
			store, err := memory.NewChromeMStoreWithEmbedding(dbPath, maxMessages, memory.NewLocalEmbeddingFunc())
			if err != nil {
				log.Fatalf("Failed to open store: %v", err)
			}
			defer func() {
				if err := store.Close(); err != nil {
					log.Printf("Failed to close store: %v", err)
				}
			}()

			fmt.Printf("ChromeM Memory Database Statistics\n")
			fmt.Printf("==================================\n")
			fmt.Printf("Database Path: %s\n", dbPath)
			fmt.Printf("Max Messages: %d\n", maxMessages)

			// Check if database file exists and get its size
			if info, err := os.Stat(dbPath); err == nil {
				fmt.Printf("Database Size: %.2f KB\n", float64(info.Size())/1024)
				fmt.Printf("Last Modified: %s\n", info.ModTime().Format("2006-01-02 15:04:05"))
			} else {
				fmt.Printf("Database Size: Database not found or inaccessible\n")
			}

			// Get actual document count
			count := store.GetMessageCount()
			fmt.Printf("Total Messages: %d\n", count)

			// Try to get some sample data to show activity
			if count > 0 {
				messages, err := store.ListAllMessages("", "", 1)
				if err == nil && len(messages) > 0 {
					fmt.Printf("Most Recent Memory: %s\n", messages[0].Timestamp.Format("2006-01-02 15:04:05"))
				} else {
					fmt.Printf("Most Recent Memory: Unable to retrieve\n")
				}
			} else {
				fmt.Printf("Most Recent Memory: No memories found\n")
			}
		},
	}

	return cmd
}

func createClearCmd() *cobra.Command {
	var (
		integrationID string
		channelID     string
		confirm       bool
	)

	cmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear memories from the store",
		Long:  "Clear all memories for a specific integration and channel",
		Run: func(cmd *cobra.Command, args []string) {
			if !confirm {
				fmt.Println("This will permanently delete memories. Use --confirm to proceed.")
				return
			}

			store, err := memory.NewChromeMStoreWithEmbedding(dbPath, maxMessages, memory.NewLocalEmbeddingFunc())
			if err != nil {
				log.Fatalf("Failed to open store: %v", err)
			}
			defer func() {
				if err := store.Close(); err != nil {
					log.Printf("Failed to close store: %v", err)
				}
			}()

			err = store.ClearMessages(integrationID, channelID)
			if err != nil {
				log.Fatalf("Failed to clear messages: %v", err)
			}

			fmt.Printf("Successfully cleared memories for integration: %s, channel: %s\n", integrationID, channelID)
		},
	}

	cmd.Flags().StringVar(&integrationID, "integration", "", "Integration ID (required)")
	cmd.Flags().StringVar(&channelID, "channel", "", "Channel ID (required)")
	cmd.Flags().BoolVar(&confirm, "confirm", false, "Confirm the clear operation")

	if err := cmd.MarkFlagRequired("integration"); err != nil {
		log.Fatal(err)
	}
	if err := cmd.MarkFlagRequired("channel"); err != nil {
		log.Fatal(err)
	}

	return cmd
}
