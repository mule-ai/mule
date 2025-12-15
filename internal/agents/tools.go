package agents

import (
	"context"
	"fmt"

	"github.com/mark3labs/mule/internal/tools"
	"github.com/mark3labs/mule/internal/tools/mcp"
)

// ExecuteTool executes a tool call for an agent
func (s *Service) ExecuteTool(ctx context.Context, agentID string, toolCall ToolCall) (*ToolResponse, error) {
	// Get the agent to find the tool
	agent, err := s.repo.GetAgentByID(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}

	// Find the tool in the agent's tool list
	var tool *tools.Tool
	for _, t := range agent.Tools {
		if t.Name == toolCall.Name {
			tool = &t
			break
		}
	}

	if tool == nil {
		return nil, fmt.Errorf("tool %s not found for agent %s", toolCall.Name, agentID)
	}

	// Execute the tool based on its type
	switch tool.Type {
	case tools.ToolTypeHTTP:
		return s.executeHTTPTool(ctx, tool, toolCall)
	case tools.ToolTypeShell:
		return s.executeShellTool(ctx, tool, toolCall)
	case tools.ToolTypeMCP:
		return s.executeMCPTool(ctx, tool, toolCall)
	default:
		return nil, fmt.Errorf("unsupported tool type: %s", tool.Type)
	}
}

func (s *Service) executeMCPTool(ctx context.Context, tool *tools.Tool, toolCall ToolCall) (*ToolResponse, error) {
	// Parse MCP config
	mcpConfig, err := s.parseMCPConfig(tool.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MCP config: %w", err)
	}

	// Create MCP client
	client := mcp.NewClient(mcp.MCPToolConfig{
		EndpointURL:        mcpConfig.EndpointURL,
		AuthToken:          mcpConfig.AuthToken,
		ConnectionSettings: mcpConfig.ConnectionSettings,
		Timeout:            30 * 1000000000, // 30 seconds default
	})

	// Connect to MCP server
	if err := client.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to MCP server: %w", err)
	}
	defer client.Close()

	// Execute the tool call
	mcpToolCall := mcp.MCPToolCall{
		Name:      toolCall.Arguments["name"].(string),
		Arguments: toolCall.Arguments,
	}

	mcpResponse, err := client.CallTool(ctx, mcpToolCall)
	if err != nil {
		return nil, fmt.Errorf("failed to call MCP tool: %w", err)
	}

	// Convert MCP response to tool response
	response := &ToolResponse{
		Content: mcpResponse.Content,
		IsError: mcpResponse.IsError,
	}

	return response, nil
}

func (s *Service) parseMCPConfig(config map[string]interface{}) (*tools.MCPConfig, error) {
	// Convert map to JSON and then to struct
	jsonData, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var mcpConfig tools.MCPConfig
	if err := json.Unmarshal(jsonData, &mcpConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &mcpConfig, nil
}