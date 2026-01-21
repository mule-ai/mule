package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// Client wraps the MCP client functionality
type Client struct {
	mcpClient  *client.StdioMCPClient
	config     MCPToolConfig
	initialized bool
}

// NewClient creates a new MCP client
func NewClient(config MCPToolConfig) *Client {
	return &Client{
		config: config,
	}
}

// Connect initializes the connection to the MCP server
func (c *Client) Connect(ctx context.Context) error {
	if c.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.config.Timeout)
		defer cancel()
	}

	// For now, we'll assume stdio-based client
	// In the future, we might need to support other transport types
	var err error
	c.mcpClient, err = client.NewStdioMCPClient(
		"npx",
		[]string{}, // Empty ENV
		"-y",
		c.config.EndpointURL,
		"/tmp",
	)
	if err != nil {
		return fmt.Errorf("failed to create MCP client: %w", err)
	}

	// Initialize the client
	initRequest := mcp.InitializeRequest{}
	initRequest.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initRequest.Params.ClientInfo = mcp.Implementation{
		Name:    "mule-mcp-client",
		Version: "1.0.0",
	}

	_, err = c.mcpClient.Initialize(ctx, initRequest)
	if err != nil {
		c.mcpClient.Close()
		c.mcpClient = nil
		return fmt.Errorf("failed to initialize MCP client: %w", err)
	}

	c.initialized = true
	return nil
}

// ListTools returns the list of available tools from the MCP server
func (c *Client) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	if !c.initialized || c.mcpClient == nil {
		return nil, fmt.Errorf("MCP client not initialized")
	}

	if c.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.config.Timeout)
		defer cancel()
	}

	toolsRequest := mcp.ListToolsRequest{}
	result, err := c.mcpClient.ListTools(ctx, toolsRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	return result.Tools, nil
}

// CallTool executes a tool call on the MCP server
func (c *Client) CallTool(ctx context.Context, toolCall MCPToolCall) (*MCPToolResponse, error) {
	if !c.initialized || c.mcpClient == nil {
		return nil, fmt.Errorf("MCP client not initialized")
	}

	if c.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.config.Timeout)
		defer cancel()
	}

	callRequest := mcp.CallToolRequest{}
	callRequest.Params.Name = toolCall.Name
	callRequest.Params.Arguments = toolCall.Arguments

	result, err := c.mcpClient.CallTool(ctx, callRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to call tool: %w", err)
	}

	response := &MCPToolResponse{
		Content: result.Content,
		IsError: result.IsError,
	}

	return response, nil
}

// Close closes the connection to the MCP server
func (c *Client) Close() error {
	if c.mcpClient != nil {
		err := c.mcpClient.Close()
		c.mcpClient = nil
		c.initialized = false
		return err
	}
	return nil
}