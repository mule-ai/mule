package tools

import (
	"time"
)

// ToolType represents the type of tool
type ToolType string

const (
	ToolTypeHTTP ToolType = "http"
	ToolTypeShell ToolType = "shell"
	ToolTypeMCP   ToolType = "mcp"
)

// Tool represents a tool that can be used by an agent
type Tool struct {
	ID          string            `json:"id" db:"id"`
	Name        string            `json:"name" db:"name"`
	Description string            `json:"description" db:"description"`
	Type        ToolType          `json:"type" db:"type"`
	Config      map[string]interface{} `json:"config" db:"config"`
	CreatedAt   time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at" db:"updated_at"`
}

// HTTPConfig represents the configuration for an HTTP tool
type HTTPConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers,omitempty"`
	Timeout int               `json:"timeout,omitempty"`
}

// ShellConfig represents the configuration for a shell tool
type ShellConfig struct {
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

// MCPConfig represents the configuration for an MCP tool
type MCPConfig struct {
	EndpointURL        string                 `json:"endpoint_url"`
	AuthToken          string                 `json:"auth_token,omitempty"`
	ConnectionSettings map[string]interface{} `json:"connection_settings,omitempty"`
	Timeout            int                    `json:"timeout,omitempty"`
}