package mcp

import (
	"encoding/json"
	"fmt"

	"github.com/jbutlerdev/genai/tools"
)

// MCPToolAdapter adapts MCP tools to the genai tools interface
type MCPToolAdapter struct {
	mcp        *MCP
	serverName string
	toolName   string
	tool       Tool
}

// NewMCPToolAdapter creates a new adapter for an MCP tool
func NewMCPToolAdapter(mcp *MCP, serverName, toolName string, tool Tool) *MCPToolAdapter {
	return &MCPToolAdapter{
		mcp:        mcp,
		serverName: serverName,
		toolName:   toolName,
		tool:       tool,
	}
}

// Name returns the name of the tool
func (m *MCPToolAdapter) Name() string {
	return fmt.Sprintf("mcp_%s_%s", m.serverName, m.toolName)
}

// Description returns the description of the tool
func (m *MCPToolAdapter) Description() string {
	return m.tool.Description
}

// Call executes the MCP tool
func (m *MCPToolAdapter) Call(input string) (string, error) {
	// Parse the input as JSON
	var params interface{}
	if input != "" {
		if err := json.Unmarshal([]byte(input), &params); err != nil {
			// If not valid JSON, treat as a simple string parameter
			params = input
		}
	}

	// Call the MCP tool
	result, err := m.mcp.CallTool(m.serverName, m.toolName, params)
	if err != nil {
		return "", fmt.Errorf("MCP tool call failed: %w", err)
	}

	// Convert result to string
	switch v := result.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		// Marshal complex results to JSON
		resultJSON, err := json.Marshal(result)
		if err != nil {
			return "", fmt.Errorf("failed to marshal result: %w", err)
		}
		return string(resultJSON), nil
	}
}

// Attributes returns the attributes of the tool (empty for MCP tools)
func (m *MCPToolAdapter) Attributes() map[string]string {
	// Convert input schema to attributes if needed
	attrs := make(map[string]string)
	attrs["server"] = m.serverName
	attrs["mcp_tool"] = m.toolName
	
	// Add input schema as JSON if present
	if len(m.tool.InputSchema) > 0 {
		if schemaJSON, err := json.Marshal(m.tool.InputSchema); err == nil {
			attrs["input_schema"] = string(schemaJSON)
		}
	}
	
	return attrs
}

// GetMCPTools returns all MCP tools as genai tool interfaces
func (m *MCP) GetMCPTools() ([]tools.Tool, error) {
	allTools, err := m.ListTools()
	if err != nil {
		return nil, err
	}

	var genaiTools []tools.Tool
	for serverName, serverTools := range allTools {
		for _, tool := range serverTools {
			adapter := NewMCPToolAdapter(m, serverName, tool.Name, tool)
			genaiTools = append(genaiTools, adapter)
		}
	}

	return genaiTools, nil
}

// RegisterWithToolStore registers all MCP tools with a genai tool store
func (m *MCP) RegisterWithToolStore(store *tools.Store) error {
	mcpTools, err := m.GetMCPTools()
	if err != nil {
		return fmt.Errorf("failed to get MCP tools: %w", err)
	}

	for _, tool := range mcpTools {
		store.Add(tool)
	}

	m.logger.Info("registered MCP tools with tool store", "count", len(mcpTools))
	return nil
}