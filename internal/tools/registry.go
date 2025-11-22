package tools

import (
	"context"
	"fmt"
	"sync"

	"google.golang.org/adk/tool"
)

// Registry manages built-in tools and provides them to agents
type Registry struct {
	tools map[string]Tool
	mu    sync.RWMutex
}

// Tool defines the interface for built-in tools
type Tool interface {
	Name() string
	Description() string
	IsLongRunning() bool
	Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
	GetSchema() map[string]interface{}
	ToTool() tool.Tool
}

// NewRegistry creates a new tool registry with built-in tools
func NewRegistry() *Registry {
	registry := &Registry{
		tools: make(map[string]Tool),
	}

	// Register built-in tools
	registry.Register(NewMemoryTool())
	registry.Register(NewFilesystemTool("."))
	registry.Register(NewHTTPTool())
	registry.Register(NewDatabaseTool())

	return registry
}

// Register registers a tool in the registry
func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tools[tool.Name()] = tool
}

// Get retrieves a tool by name
func (r *Registry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	if !exists {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	return tool, nil
}

// List returns all registered tools
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}

	return tools
}

// GetADKTools returns all tools as ADK tool interfaces
func (r *Registry) GetADKTools() []tool.Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	adkTools := make([]tool.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		adkTools = append(adkTools, t.ToTool())
	}

	return adkTools
}

// GetToolNames returns a list of all registered tool names
func (r *Registry) GetToolNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}

	return names
}

// BuiltInTools returns a list of built-in tool names
func BuiltInTools() []string {
	return []string{
		"memory",
		"filesystem",
		"http",
		"database",
	}
}
