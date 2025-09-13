package discord

import (
	"encoding/json"
	"fmt"

	"github.com/bwmarrin/discordgo"
)

var (
	rssEvents = map[string]struct{}{
		"allMessages": {},
	}
)

// SetRSSMode enables RSS feed mode which captures ALL messages in the configured channel
func (d *Discord) SetRSSMode(enabled bool) {
	if enabled {
		// Add RSS-specific events to the events map
		for event := range rssEvents {
			events[event] = struct{}{}
		}
		d.l.Info("RSS mode enabled - will capture all Discord messages")
	}
}

// messageCreateRSS is called when a new message is created - captures ALL messages for RSS feed
func (d *Discord) messageCreateRSS(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Only process messages from the configured channel (if specified)
	if d.config.ChannelID != "" && m.ChannelID != d.config.ChannelID {
		return
	}

	d.l.Info("Discord RSS message received", "sender", m.Author.Username, "channel_id", m.ChannelID, "content", m.Content)

	// Process ALL messages for RSS feed (not just mentions)
	d.messageReceivedRSS(m.Content, m.Author.ID, m.ChannelID, m.Author.Username)
}

// messageReceivedRSS processes incoming messages specifically for RSS feed
func (d *Discord) messageReceivedRSS(message, userID, channelID, username string) {
	// Get better username if available
	if d.session != nil && d.session.State != nil {
		if member, err := d.session.State.Member("", userID); err == nil && member != nil {
			if member.Nick != "" {
				username = member.Nick
			} else if member.User != nil && member.User.Username != "" {
				username = member.User.Username
			}
		}
	}

	// Store the message in memory if available
	if d.memory != nil {
		if err := d.memory.SaveMessage(d.Name(), channelID, userID, username, message, false); err != nil {
			d.l.Error(err, "Failed to save RSS message to memory")
		}
	}

	// Create RSS item data
	rssItemData := map[string]string{
		"title":       fmt.Sprintf("Message from %s in %s", username, d.GetChannelName(channelID)),
		"description": message,
		"link":        fmt.Sprintf("https://discord.com/channels/%s/%s", d.config.GuildID, channelID),
		"author":      username,
	}

	// Convert to JSON string for workflow compatibility
	jsonData, err := json.Marshal(rssItemData)
	if err != nil {
		d.l.Error(err, "Failed to marshal RSS item data")
		return
	}

	// Trigger RSS feed addition with JSON string
	if rssChannel, exists := d.triggers["allMessages"]; exists {
		select {
		case rssChannel <- string(jsonData):
			d.l.Info("Added message to RSS feed", "username", username, "content", truncateString(message, 50))
		default:
			d.l.Info("RSS trigger channel full or not ready, discarding message", "message", truncateString(message, 50))
		}
	} else {
		d.l.Info("RSS trigger not registered, skipping message", "message", truncateString(message, 50))
	}
}

// EnableRSSHandler adds the RSS message handler alongside the existing handler
func (d *Discord) EnableRSSHandler() {
	if d.session != nil {
		d.session.AddHandler(d.messageCreateRSS)
		d.l.Info("RSS message handler enabled")
	}
}

// truncateString truncates a string to a maximum length
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// RegisterRSSTrigger registers a trigger specifically for RSS messages
func (d *Discord) RegisterRSSTrigger(trigger string, data any, channel chan any) {
	_, ok := rssEvents[trigger]
	if !ok {
		d.l.Error(fmt.Errorf("RSS trigger not found: %s", trigger), "RSS Trigger not found")
		return
	}

	dataStr, ok := data.(string)
	if !ok && data != nil {
		d.l.Error(fmt.Errorf("RSS trigger data is not a string: %v", data), "RSS Data is not a string")
		return
	}

	triggerKey := trigger
	if dataStr != "" {
		triggerKey = trigger + dataStr
	}

	d.triggers[triggerKey] = channel
	d.l.Info("Registered RSS trigger", "key", triggerKey)
}

// GetChannelName gets the channel name for better RSS item titles
func (d *Discord) GetChannelName(channelID string) string {
	if d.session == nil {
		return channelID
	}

	channel, err := d.session.State.Channel(channelID)
	if err != nil {
		// Try to fetch from API if not in state
		channel, err = d.session.Channel(channelID)
		if err != nil {
			return channelID
		}
	}

	if channel.Name != "" {
		return "#" + channel.Name
	}
	return channelID
}

// SetRSSIntegration sets up the Discord integration to work with RSS
func (d *Discord) SetRSSIntegration(rssChannel chan any) {
	// Register the RSS trigger
	d.RegisterRSSTrigger("allMessages", nil, rssChannel)

	// Enable RSS mode
	d.SetRSSMode(true)

	// Enable RSS message handler
	d.EnableRSSHandler()

	d.l.Info("Discord RSS integration configured")
}
