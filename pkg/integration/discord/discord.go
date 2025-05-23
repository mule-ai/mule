package discord

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/go-logr/logr"
	"github.com/mule-ai/mule/pkg/integration/memory"
	"github.com/mule-ai/mule/pkg/integration/types"
)

// Config holds the configuration for the Discord integration.
type Config struct {
	Enabled          bool   `json:"enabled,omitempty"`
	MessageOnConnect bool   `json:"messageOnConnect,omitempty"`
	BotToken         string `json:"botToken,omitempty"`  // Discord Bot Token
	GuildID          string `json:"guildId,omitempty"`   // Optional: Server ID for slash commands
	ChannelID        string `json:"channelId,omitempty"` // Default channel to send messages to and listen from
}

// Discord represents the Discord integration.
type Discord struct {
	config             *Config
	l                  logr.Logger
	session            *discordgo.Session
	channel            chan any // Internal channel for triggers
	slashCommandRegex  *regexp.Regexp
	triggers           map[string]chan any
	registeredCommands []*discordgo.ApplicationCommand
	memory             *memory.Memory
}

var (
	events = map[string]struct{}{
		"newMessage":   {},
		"slashCommand": {},
	}
)

// New creates a new Discord integration instance.
func New(config *Config, l logr.Logger) *Discord {
	d := &Discord{
		config:             config,
		l:                  l,
		channel:            make(chan any),
		triggers:           make(map[string]chan any),
		registeredCommands: make([]*discordgo.ApplicationCommand, 0),
	}
	// Regex to identify mentions (e.g., @username) - Discord handles this differently,
	// but we might want to parse mentions if the bot is mentioned.
	// For simplicity, this example will check if the bot's User ID is in the message mentions.
	// d.mentionRegex = regexp.MustCompile(`@(\w+)`) // Simplified, Discord provides structured mention data

	// Regex to identify slash commands (e.g., /command)
	d.slashCommandRegex = regexp.MustCompile(`\/([a-zA-Z0-9_]+)`) //

	d.init()
	go d.receiveTriggers()
	return d
}

// init initializes the Discord integration.
func (d *Discord) init() {
	if !d.config.Enabled {
		d.l.Info("Discord integration is disabled")
		return
	}

	if err := d.validateConfig(); err != nil {
		d.l.Error(err, "Invalid Discord config")
		return
	}

	if err := d.connect(); err != nil {
		d.l.Error(err, "Failed to connect to Discord")
		return
	}
	if d.config.MessageOnConnect {
		if err := d.sendMessage(d.config.ChannelID, "Mule is online"); err != nil {
			d.l.Error(err, "Failed to send online message")
		}
	}

	// Optionally register slash commands here
	// This is a more complex topic with discordgo, often involving command definitions
	// and registration with Discord's API. For simplicity, basic slash command
	// detection from message content is implemented in messageReceived.
	// Proper slash command handling would use InteractionCreate events.
	d.l.Info("Discord integration initialized")
}

// validateConfig checks if the necessary configuration is provided.
func (d *Discord) validateConfig() error {
	if d.config.BotToken == "" {
		return fmt.Errorf("BotToken is not set")
	}
	if d.config.ChannelID == "" {
		return fmt.Errorf("ChannelID is not set (default channel for messages)")
	}
	// GuildID might be optional if commands are global or not used extensively
	return nil
}

// connect establishes a connection to Discord.
func (d *Discord) connect() error {
	var err error
	d.session, err = discordgo.New("Bot " + d.config.BotToken) //
	if err != nil {
		return fmt.Errorf("failed to create Discord session: %w", err)
	}

	d.session.AddHandler(d.messageCreate)     // Handles regular messages
	d.session.AddHandler(d.interactionCreate) // Handles slash commands and other interactions

	// We need information about guilds, messages, and potentially message content.
	// MESSAGE_CONTENT is a privileged intent. Your bot must be approved for it if it's not verified.
	d.session.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages | discordgo.IntentsMessageContent

	err = d.session.Open()
	if err != nil {
		return fmt.Errorf("failed to open Discord connection: %w", err)
	}

	d.l.Info("Successfully connected to Discord", "user", d.session.State.User.Username)

	return nil
}

// Call is a generic method for future extensions (not directly used in this example).
func (d *Discord) Call(name string, data any) (any, error) {
	d.l.Info("Call method invoked", "name", name)
	// Example: if name == "registerSlashCommand"
	// This could be used to dynamically register commands if needed.
	return nil, fmt.Errorf("method '%s' not implemented", name)
}

// Name returns the name of the integration.
func (d *Discord) Name() string {
	return "discord"
}

// sendMessage is an internal helper to send a message to a specific channel.
func (d *Discord) sendMessage(channelID, message string) error { //
	if d.session == nil {
		return fmt.Errorf("discord session not initialized")
	}
	_, err := d.session.ChannelMessageSend(channelID, message)
	if err != nil {
		d.l.Error(err, "Error sending message to Discord", "channelID", channelID)
		return err
	}

	// Log the bot's message to chat history
	if d.memory != nil {
		d.LogBotMessage(channelID, message)
	}

	return nil
}

func (d *Discord) sendMessageAsFile(channelID, message string) error {
	if d.session == nil {
		return fmt.Errorf("discord session not initialized")
	}
	// write message to temp file
	tempFile, err := os.CreateTemp("", "report.md")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tempFile.Name())
	_, err = tempFile.WriteString(message)
	if err != nil {
		return fmt.Errorf("failed to write message to temp file: %w", err)
	}
	file, err := os.Open(tempFile.Name())
	if err != nil {
		return fmt.Errorf("failed to open temp file: %w", err)
	}
	defer file.Close()

	// You might want to determine ContentType more robustly, e.g., using http.DetectContentType
	contentType := "application/octet-stream" // Or a more specific one

	messageSend := &discordgo.MessageSend{
		Content: "Here is your file", // Optional: text content to go with the file
		Files: []*discordgo.File{
			{
				Name:        "report.md",
				ContentType: contentType,
				Reader:      file,
			},
		},
	}

	_, err = d.session.ChannelMessageSendComplex(channelID, messageSend)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}

	// Log the bot's file message to chat history
	if d.memory != nil {
		d.LogBotMessage(channelID, "File: "+messageSend.Content)
	}

	return nil
}

// GetChannel returns the channel for internal triggers.
func (d *Discord) GetChannel() chan any { //
	return d.channel
}

// RegisterTrigger registers a channel for a specific trigger.
func (d *Discord) RegisterTrigger(trigger string, data any, channel chan any) { //
	_, ok := events[trigger]
	if !ok {
		d.l.Error(fmt.Errorf("trigger not found: %s", trigger), "Trigger not found")
		return
	}
	// The Matrix integration combines trigger and data for the key.
	// For Discord, data might represent a specific command name for "slashCommand".
	dataStr, ok := data.(string)
	if !ok && data != nil { // Allow nil data for general newMessage triggers
		d.l.Error(fmt.Errorf("trigger data is not a string: %v", data), "Data is not a string")
		return
	}

	triggerKey := trigger
	if dataStr != "" {
		triggerKey = trigger + dataStr
	}

	d.triggers[triggerKey] = channel
	d.l.Info("Registered trigger", "key", triggerKey)
}

// messageCreate is called when a new message is created on a channel the bot has access to.
func (d *Discord) messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	mentionsMe := false
	for _, user := range m.Mentions {
		if user.ID == s.State.User.ID {
			mentionsMe = true
			break
		}
	}

	if m.ChannelID != d.config.ChannelID || !mentionsMe {
		return
	}

	d.l.Info("Discord message received", "sender", m.Author.Username, "channel_id", m.ChannelID, "content", m.Content)
	d.messageReceived(m.Content, m.Author.ID, m.ChannelID, false, nil) // false for not an interaction
}

// interactionCreate is called when an interaction (e.g., slash command) is used.
func (d *Discord) interactionCreate(s *discordgo.Session, i *discordgo.InteractionCreate) {
	if i.Type == discordgo.InteractionApplicationCommand {
		commandName := i.ApplicationCommandData().Name
		d.l.Info("Discord interaction (slash command) received", "command", commandName, "user", i.Member.User.Username)

		// Respond to the interaction to acknowledge it.
		// This is important for Discord; otherwise, the command will show as "failed."
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource, // Or InteractionResponseDeferredChannelMessageWithSource if processing takes time
			Data: &discordgo.InteractionResponseData{
				Content: "Processing your command...",    // Optional initial response
				Flags:   discordgo.MessageFlagsEphemeral, // Makes the response visible only to the user who triggered the command
			},
		})
		if err != nil {
			d.l.Error(err, "Failed to send interaction response")
			// Attempt to send a follow-up if the initial response failed for some reason (though less likely for ephemeral)
			_, followupErr := s.FollowupMessageCreate(i.Interaction, true, &discordgo.WebhookParams{
				Content: "There was an error acknowledging your command.",
				Flags:   discordgo.MessageFlagsEphemeral,
			})
			if followupErr != nil {
				d.l.Error(followupErr, "Failed to send followup error message for interaction")
			}
			return
		}

		d.messageReceived("/"+commandName, i.Member.User.ID, i.ChannelID, true, i.Interaction)
	}
}

// messageReceived processes incoming messages (from messageCreate or interactionCreate).
// The `isInteraction` flag and `interaction` object are for slash commands that come via Interactions API.
func (d *Discord) messageReceived(message, userID, channelID string, isInteraction bool, interaction *discordgo.Interaction) { //
	// Get username from the session if possible
	username := "User-" + userID
	if d.session != nil && d.session.State != nil {
		if member, err := d.session.State.Member("", userID); err == nil && member != nil {
			if member.Nick != "" {
				username = member.Nick
			} else if member.User != nil && member.User.Username != "" {
				username = member.User.Username
			}
		}
	}

	// Check for slash commands first
	slashCommandMatch := d.slashCommandRegex.FindStringSubmatch(message) //
	if len(slashCommandMatch) > 1 {
		for key := range d.triggers {
			cmd := strings.TrimPrefix(key, "slashCommand")
			if strings.Contains(message, cmd) {
				d.l.Info("Slash command received", "command", cmd)
				// Store slash command in memory
				if d.memory != nil {
					if err := d.memory.SaveMessage(d.Name(), channelID, userID, username, message, false); err != nil {
						d.l.Error(err, "Failed to save slash command to memory")
					}
				}
				// For slash commands, pass the original message (no chat history)
				d.triggers[key] <- message
				return
			}
		}
		d.l.Info("Slash command recognized with no trigger, processing as chat message")
	}

	// Store the message in memory
	if d.memory != nil {
		if err := d.memory.SaveMessage(d.Name(), channelID, userID, username, message, false); err != nil {
			d.l.Error(err, "Failed to save message to memory")
		}
	}

	// Check if this is a slash command (even without a specific trigger)
	isSlashCommand := len(d.slashCommandRegex.FindStringSubmatch(message)) > 1

	// Process through triggers
	select {
	case d.triggers["newMessage"] <- func() any {
		// For slash commands without specific triggers, don't add history
		if isSlashCommand {
			return message
		}
		return d.addHistoryToMessage(message, channelID)
	}():
	default:
		d.l.Info("Channel full or not ready for newMessage, discarding message", "message", message)
		err := d.sendMessage(channelID, "Mule is busy, please try again later.")
		if err != nil {
			d.l.Error(err, "Failed to send busy message")
		}
	}
}

// receiveTriggers listens on the internal channel for actions to perform (e.g., send a message).
func (d *Discord) receiveTriggers() {
	for trigger := range d.channel {
		triggerSettings, ok := trigger.(*types.TriggerSettings)
		if !ok {
			d.l.Error(fmt.Errorf("trigger is not a Trigger"), "Trigger is not a Trigger")
			continue
		}
		if triggerSettings.Integration != "discord" {
			d.l.Error(fmt.Errorf("trigger integration is not discord"), "Trigger integration is not discord")
			continue
		}
		switch triggerSettings.Event {
		case "sendMessage":
			message, ok := triggerSettings.Data.(string)
			if !ok {
				d.l.Error(fmt.Errorf("trigger data is not a string"), "Trigger data is not a string")
				continue
			}
			err := d.sendMessage(d.config.ChannelID, message)
			if err != nil {
				d.l.Error(err, "Failed to send message from trigger")
			}
		case "sendFile":
			message, ok := triggerSettings.Data.(string)
			if !ok {
				d.l.Error(fmt.Errorf("trigger data is not a string"), "Trigger data is not a string")
				continue
			}
			err := d.sendMessageAsFile(d.config.ChannelID, message)
			if err != nil {
				d.l.Error(err, "Failed to send file from trigger")
			}
		default:
			d.l.Error(fmt.Errorf("trigger event not supported: %s", triggerSettings.Event), "Unsupported trigger event")
		}
	}
}
