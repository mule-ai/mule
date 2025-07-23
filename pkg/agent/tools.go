package agent

import (
	"fmt"
	"strings"

	"github.com/jbutlerdev/genai/tools"
	"github.com/mule-ai/mule/pkg/integration/mcp"
)

// GetToolWithMCP tries to get a tool from standard tools first, then from MCP registry
func GetToolWithMCP(name string) (*tools.Tool, error) {
	// First try standard tools
	tool, err := tools.GetTool(name)
	if err == nil {
		return tool, nil
	}

	// If not found and starts with "mcp_", try MCP registry
	if strings.HasPrefix(name, "mcp_") {
		mcpTool, mcpErr := mcp.GetMCPTool(name)
		if mcpErr == nil {
			// Convert to *tools.Tool if needed
			if t, ok := mcpTool.(*tools.Tool); ok {
				return t, nil
			}
			// Try to create a wrapper if it implements the right methods
			if toolInterface, ok := mcpTool.(interface {
				Name() string
				Description() string
				Call(string) (string, error)
				Attributes() map[string]string
			}); ok {
				return &tools.Tool{
					Name:        toolInterface.Name(),
					Description: toolInterface.Description(),
					Call:        toolInterface.Call,
					Attributes:  toolInterface.Attributes(),
				}, nil
			}
		}
	}

	// Return original error if not found
	return nil, fmt.Errorf("tool %s not found in standard or MCP tools", name)
}