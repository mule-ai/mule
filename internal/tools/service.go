package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mule/internal/tools/mcp"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) CreateTool(ctx context.Context, tool *Tool) error {
	// Validate tool configuration based on type
	switch tool.Type {
	case ToolTypeHTTP:
		if err := s.validateHTTPConfig(tool.Config); err != nil {
			return fmt.Errorf("invalid HTTP config: %w", err)
		}
	case ToolTypeShell:
		if err := s.validateShellConfig(tool.Config); err != nil {
			return fmt.Errorf("invalid shell config: %w", err)
		}
	case ToolTypeMCP:
		if err := s.validateMCPConfig(tool.Config); err != nil {
			return fmt.Errorf("invalid MCP config: %w", err)
		}
	default:
		return fmt.Errorf("unsupported tool type: %s", tool.Type)
	}

	tool.CreatedAt = time.Now()
	tool.UpdatedAt = time.Now()

	return s.repo.CreateTool(ctx, tool)
}

func (s *Service) GetToolByID(ctx context.Context, id string) (*Tool, error) {
	return s.repo.GetToolByID(ctx, id)
}

func (s *Service) UpdateTool(ctx context.Context, tool *Tool) error {
	// Validate tool configuration based on type
	switch tool.Type {
	case ToolTypeHTTP:
		if err := s.validateHTTPConfig(tool.Config); err != nil {
			return fmt.Errorf("invalid HTTP config: %w", err)
		}
	case ToolTypeShell:
		if err := s.validateShellConfig(tool.Config); err != nil {
			return fmt.Errorf("invalid shell config: %w", err)
		}
	case ToolTypeMCP:
		if err := s.validateMCPConfig(tool.Config); err != nil {
			return fmt.Errorf("invalid MCP config: %w", err)
		}
	default:
		return fmt.Errorf("unsupported tool type: %s", tool.Type)
	}

	tool.UpdatedAt = time.Now()
	return s.repo.UpdateTool(ctx, tool)
}

func (s *Service) DeleteTool(ctx context.Context, id string) error {
	return s.repo.DeleteTool(ctx, id)
}

func (s *Service) ListTools(ctx context.Context) ([]*Tool, error) {
	return s.repo.ListTools(ctx)
}

func (s *Service) ListToolsByType(ctx context.Context, toolType ToolType) ([]*Tool, error) {
	return s.repo.ListToolsByType(ctx, toolType)
}

func (s *Service) TestMCPConnection(ctx context.Context, config map[string]interface{}) error {
	mcpConfig, err := s.parseMCPConfig(config)
	if err != nil {
		return fmt.Errorf("failed to parse MCP config: %w", err)
	}

	client := mcp.NewClient(mcp.MCPToolConfig{
		EndpointURL:        mcpConfig.EndpointURL,
		AuthToken:          mcpConfig.AuthToken,
		ConnectionSettings: mcpConfig.ConnectionSettings,
		Timeout:            time.Duration(mcpConfig.Timeout) * time.Second,
	})

	defer client.Close()

	if err := client.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect to MCP server: %w", err)
	}

	// Try to list tools to verify the connection works
	_, err = client.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("failed to list tools from MCP server: %w", err)
	}

	return nil
}

func (s *Service) validateHTTPConfig(config map[string]interface{}) error {
	// Basic validation - in a real implementation, you'd want more thorough validation
	if config["url"] == nil || config["url"] == "" {
		return fmt.Errorf("URL is required for HTTP tools")
	}
	return nil
}

func (s *Service) validateShellConfig(config map[string]interface{}) error {
	// Basic validation - in a real implementation, you'd want more thorough validation
	if config["command"] == nil || config["command"] == "" {
		return fmt.Errorf("command is required for shell tools")
	}
	return nil
}

func (s *Service) validateMCPConfig(config map[string]interface{}) error {
	// Basic validation - in a real implementation, you'd want more thorough validation
	if config["endpoint_url"] == nil || config["endpoint_url"] == "" {
		return fmt.Errorf("endpoint_url is required for MCP tools")
	}
	return nil
}

func (s *Service) parseMCPConfig(config map[string]interface{}) (*MCPConfig, error) {
	// Convert map to JSON and then to struct
	jsonData, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var mcpConfig MCPConfig
	if err := json.Unmarshal(jsonData, &mcpConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return &mcpConfig, nil
}