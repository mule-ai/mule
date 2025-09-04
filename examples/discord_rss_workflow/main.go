package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-logr/zapr"
	"github.com/jbutlerdev/genai"
	"github.com/mule-ai/mule/pkg/agent"
	"github.com/mule-ai/mule/pkg/integration"
	"github.com/mule-ai/mule/pkg/integration/api"
	"github.com/mule-ai/mule/pkg/integration/discord"
	"github.com/mule-ai/mule/pkg/integration/memory"
	"github.com/mule-ai/mule/pkg/integration/rss"
	"github.com/mule-ai/mule/pkg/types"
	"go.uber.org/zap"
)

func main() {
	// Set up logging
	zapLogger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatal(err)
	}
	logger := zapr.NewLogger(zapLogger)

	fmt.Println("=== Discord RSS Feed Workflow Demo ===")
	fmt.Println("This workflow captures ALL Discord messages and adds them to an RSS feed")

	// Check for required environment variables
	botToken := os.Getenv("DISCORD_BOT_TOKEN")
	channelID := os.Getenv("DISCORD_CHANNEL_ID")
	guildID := os.Getenv("DISCORD_GUILD_ID")

	if botToken == "" {
		log.Fatal("DISCORD_BOT_TOKEN environment variable is required")
	}
	if channelID == "" {
		log.Fatal("DISCORD_CHANNEL_ID environment variable is required")
	}

	fmt.Printf("Bot Token: %s...\n", botToken[:10])
	fmt.Printf("Channel ID: %s\n", channelID)
	if guildID != "" {
		fmt.Printf("Guild ID: %s\n", guildID)
	}

	// Create integration settings
	settings := &integration.Settings{
		Discord: &discord.Config{
			Enabled:          true,
			MessageOnConnect: true,
			BotToken:         botToken,
			GuildID:          guildID,
			ChannelID:        channelID,
		},
		RSS: map[string]*rss.Config{
			"discord": {
				Enabled:     true,
				Title:       "Discord Messages RSS Feed",
				Description: "Live RSS feed of Discord messages from the configured channel",
				Link:        "http://localhost:8083/rss",
				Author:      "Mule Discord Bot",
				MaxItems:    50,
				Path:        "/rss",
			},
		},
		API: &api.Config{
			Enabled: true,
			Path:    "/integration-api",
		},
		Memory: &memory.Config{
			Enabled:     true,
			MaxMessages: 100,
		},
	}

	// Initialize integrations
	integrationInput := integration.IntegrationInput{
		Settings:  settings,
		Providers: make(map[string]*genai.Provider),
		Agents:    make(map[int]*agent.Agent),
		Workflows: make(map[string]*agent.Workflow),
		Logger:    logger,
	}

	integrations := integration.LoadIntegrations(integrationInput)

	// Verify integrations loaded
	if _, exists := integrations["discord"]; !exists {
		log.Fatal("Failed to load Discord integration")
	}
	if _, exists := integrations["rss"]; !exists {
		log.Fatal("Failed to load RSS integration")
	}

	fmt.Println("✓ Discord integration loaded")
	fmt.Println("✓ RSS integration loaded")
	fmt.Println("✓ Integrations connected")

	// Set up RSS message handler
	rssIntegration := integrations["rss"]
	discordIntegration := integrations["discord"]

	// Create a channel to receive Discord messages for RSS
	rssChannel := make(chan any, 100)

	// Register trigger for all Discord messages
	discordIntegration.RegisterTrigger("allMessages", nil, rssChannel)

	// Start RSS message processor
	go func() {
		logger.Info("Starting RSS message processor...")
		for messageData := range rssChannel {
			// Convert message data to RSS item
			itemData, ok := messageData.(map[string]string)
			if !ok {
				logger.Error(fmt.Errorf("invalid message data format"), "Invalid message data")
				continue
			}

			// Add to RSS feed via trigger
			rssIntegration.GetChannel() <- &types.TriggerSettings{
				Integration: "rss",
				Event:       "addItem",
				Data:        itemData,
			}

			logger.Info("Added message to RSS feed",
				"author", itemData["author"],
				"title", itemData["title"])
		}
	}()

	fmt.Println("\n🚀 Discord RSS Workflow is now running!")
	fmt.Println("📡 RSS feed available at: http://localhost:8083/rss")
	fmt.Println("🌐 Web interface at: http://localhost:8083/rss-index")
	fmt.Println("💬 All Discord messages from the configured channel will be added to the RSS feed")
	fmt.Println("\nPress Ctrl+C to stop...")

	// Wait for interrupt signal to gracefully shut down
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	<-c

	fmt.Println("\n🛑 Shutting down...")
	fmt.Println("✓ Shutdown complete")
}
