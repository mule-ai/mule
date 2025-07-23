# MCP (Model Context Protocol) Integration

Mule now supports MCP servers, allowing you to extend its capabilities with additional tools and services through the Model Context Protocol.

## Configuration

Add MCP server configurations to your `~/.config/mule/config.yaml`:

```yaml
integration:
  mcp:
    enabled: true
    servers:
      # Example: Weather MCP server
      weather:
        command: "npx"
        args: ["-y", "@modelcontextprotocol/server-weather"]
        env:
          OPENWEATHER_API_KEY: "your-api-key"
      
      # Example: Filesystem MCP server
      filesystem:
        command: "npx"
        args: ["-y", "@modelcontextprotocol/server-filesystem", "/path/to/allowed/directory"]
      
      # Example: Custom MCP server
      custom:
        command: "/path/to/mcp-server"
        args: ["--port", "3000"]
        env:
          CUSTOM_CONFIG: "value"
```

## Using MCP Tools in Agents

Once configured, MCP tools are automatically discovered and can be used in agent configurations. MCP tools are prefixed with `mcp_<server>_<tool>`:

```yaml
agents:
  - id: 1
    name: "Weather Agent"
    providerName: "openai"
    model: "gpt-4"
    tools:
      - "mcp_weather_get_forecast"
      - "mcp_weather_get_current"
    promptTemplate: |
      Get weather information for the requested location.
```

## Available MCP Servers

Popular MCP servers that can be used with Mule:

1. **Weather Server** - Weather data and forecasts
2. **Filesystem Server** - File system operations
3. **GitHub Server** - GitHub API operations
4. **Database Servers** - SQL and NoSQL database access
5. **Search Servers** - Web and document search

## API Usage

The MCP integration can also be controlled programmatically:

```go
// Start MCP servers
integration.Call("mcp", "start", nil)

// List available tools
tools, _ := integration.Call("mcp", "list_tools", nil)

// Call a specific tool
result, _ := integration.Call("mcp", "call_tool", map[string]interface{}{
    "server": "weather",
    "tool": "get_forecast",
    "params": map[string]string{
        "location": "San Francisco",
    },
})

// Stop MCP servers
integration.Call("mcp", "stop", nil)
```

## Troubleshooting

1. **Server fails to start**: Check that the command is available in PATH
2. **Tools not appearing**: Ensure the server supports tool listing
3. **Tool calls failing**: Verify parameters match the tool's input schema

## Protocol Details

Mule implements the MCP protocol version 0.1.0 with support for:
- Tool discovery and invocation
- Server lifecycle management
- Dynamic tool registration with the agent system