# Discord RSS Feed Integration

This example demonstrates how to use Mule's RSS integration with Discord to create a live RSS feed of Discord messages.

## Features

- **Captures ALL Discord messages** from configured channels (not just mentions)
- **RSS feed server** accessible via HTTP
- **Real-time updates** as messages are posted
- **Web interface** for viewing the RSS feed
- **Configurable feed settings** (title, description, max items)

## Setup

### 1. Discord Bot Setup

1. Create a Discord application at https://discord.com/developers/applications
2. Create a bot for your application
3. Copy the bot token
4. Invite the bot to your Discord server with the following permissions:
   - Read Messages
   - Read Message History
   - View Channels

### 2. Environment Variables

Set the following environment variables:

```bash
export DISCORD_BOT_TOKEN="your_bot_token_here"
export DISCORD_CHANNEL_ID="123456789012345678"  # Channel ID to monitor
export DISCORD_GUILD_ID="123456789012345678"    # Optional: Server/Guild ID
```

To get Channel ID and Guild ID:
1. Enable Developer Mode in Discord settings
2. Right-click on the channel/server and select "Copy ID"

### 3. Running the Example

```bash
# From the mule root directory
go run ./examples/discord_rss_workflow
```

Or build and run:

```bash
go build ./examples/discord_rss_workflow
./discord_rss_workflow
```

## Usage

Once running, the workflow will:

1. **Connect to Discord** using the provided bot token
2. **Register RSS endpoints** with the main web server (port 8083)
3. **Monitor all messages** in the configured Discord channel
4. **Add messages to RSS feed** in real-time

### Access the RSS Feed

- **RSS XML**: http://localhost:8083/rss
- **Web interface**: http://localhost:8083/rss-index

### RSS Feed Structure

Each Discord message becomes an RSS item with:
- **Title**: "Message from [username] in [channel]"
- **Description**: The message content
- **Author**: The Discord username
- **Link**: Discord message URL (https://discord.com/channels/guild/channel)
- **Date**: When the message was posted

## Configuration

You can customize the RSS feed by modifying the RSS config in `main.go`:

```go
RSS: &rss.Config{
    Enabled:     true,
    Title:       "Discord Messages RSS Feed",      // RSS feed title
    Description: "Live RSS feed of Discord messages", // RSS description
    Link:        "http://localhost:8083/rss",      // RSS feed link
    Author:      "Mule Discord Bot",               // RSS author
    MaxItems:    50,                               // Max items to keep in feed
    Path:        "/rss",                            // URL path for RSS feed
},
```

## Integration with Mule Workflows

This example can be extended to:

1. **Filter messages** based on content or author
2. **Transform messages** before adding to RSS (e.g., remove mentions, format text)
3. **Add AI processing** to summarize or categorize messages
4. **Integrate with other systems** (Slack, email notifications, etc.)

## Troubleshooting

### Bot Not Receiving Messages

1. Ensure the bot has "Read Messages" and "Message Content Intent" permissions
2. Verify the bot is in the correct Discord server/channel
3. Check that the Channel ID is correct
4. For newer Discord bots, you may need to enable "Message Content Intent" in the Discord Developer Portal

### RSS Feed Empty

1. Send a test message in the Discord channel
2. Check the console logs for any errors
3. Verify the RSS server is running on http://localhost:8080

### Permission Issues

The Discord bot needs the following intents:
- `GUILD_MESSAGES` - to receive message events
- `MESSAGE_CONTENT` - to read message content (privileged intent)

For bots in 100+ servers, `MESSAGE_CONTENT` requires verification.

## Security Note

- Never commit your Discord bot token to version control
- Use environment variables or secure configuration files
- Consider implementing rate limiting for the RSS endpoint in production