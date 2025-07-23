package mcp

import (
	"fmt"
	"sync"

	"github.com/jbutlerdev/genai/tools"
)

// ToolRegistry manages MCP tool registration
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]interface{} // Store as interface{} to avoid import cycles
}

// GlobalRegistry is the global MCP tool registry
var GlobalRegistry = &ToolRegistry{
	tools: make(map[string]interface{}),
}

// Register adds a tool to the registry
func (r *ToolRegistry) Register(tool interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// Get name from tool interface
	if t, ok := tool.(interface{ Name() string }); ok {
		r.tools[t.Name()] = tool
	}
}

// Get retrieves a tool from the registry
func (r *ToolRegistry) Get(name string) (interface{}, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	tool, exists := r.tools[name]
	if !exists {
		return nil, fmt.Errorf("MCP tool %s not found", name)
	}
	return tool, nil
}

// List returns all registered tool names
func (r *ToolRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// Clear removes all tools from the registry
func (r *ToolRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools = make(map[string]interface{})
}

// RegisterMCPTools registers all available MCP tools
func (m *MCP) RegisterMCPTools() error {
	// Clear existing MCP tools
	for _, name := range GlobalRegistry.List() {
		// Only clear tools that start with "mcp_"
		if len(name) > 4 && name[:4] == "mcp_" {
			GlobalRegistry.mu.Lock()
			delete(GlobalRegistry.tools, name)
			GlobalRegistry.mu.Unlock()
		}
	}

	// Get all MCP tools
	mcpTools, err := m.GetMCPTools()
	if err != nil {
		return fmt.Errorf("failed to get MCP tools: %w", err)
	}

	// Register each tool
	for _, tool := range mcpTools {
		GlobalRegistry.Register(tool)
	}

	m.logger.Info("registered MCP tools", "count", len(mcpTools))
	return nil
}

// GetMCPTool is a helper function that can be used by agents
func GetMCPTool(name string) (interface{}, error) {
	return GlobalRegistry.Get(name)
}