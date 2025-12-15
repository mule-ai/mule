package mcp

import (
	"time"
)

// MCPToolConfig represents the configuration for an MCP tool
type MCPToolConfig struct {
	EndpointURL        string                 `json:"endpoint_url"`
	AuthToken          string                 `json:"auth_token,omitempty"`
	ConnectionSettings map[string]interface{} `json:"connection_settings,omitempty"`
	Timeout            time.Duration          `json:"timeout,omitempty"`
}

// MCPToolCall represents a request to call an MCP tool
type MCPToolCall struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// MCPToolResponse represents the response from an MCP tool call
type MCPToolResponse struct {
	Content []interface{} `json:"content"`
	IsError bool          `json:"is_error,omitempty"`
}