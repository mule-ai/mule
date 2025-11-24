package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

// InMemoryTool provides in-memory key-value storage for agents (legacy implementation)
type InMemoryTool struct {
	name string
	desc string
	data map[string]interface{}
	mu   sync.RWMutex
}

// NewInMemoryTool creates a new in-memory tool (legacy implementation)
func NewInMemoryTool() *InMemoryTool {
	return &InMemoryTool{
		name: "memory",
		desc: "Store and retrieve key-value pairs in memory",
		data: make(map[string]interface{}),
	}
}

// Name returns the tool name
func (m *InMemoryTool) Name() string {
	return m.name
}

// Description returns the tool description
func (m *InMemoryTool) Description() string {
	return m.desc
}

// IsLongRunning indicates if this is a long-running operation
func (m *InMemoryTool) IsLongRunning() bool {
	return false
}

// Execute executes the memory tool with the given parameters
func (m *InMemoryTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	action, ok := params["action"].(string)
	if !ok {
		return nil, fmt.Errorf("action parameter is required")
	}

	switch action {
	case "get":
		key, ok := params["key"].(string)
		if !ok {
			return nil, fmt.Errorf("key parameter is required for get action")
		}
		return m.Get(key)
	case "set":
		key, ok := params["key"].(string)
		if !ok {
			return nil, fmt.Errorf("key parameter is required for set action")
		}
		value, ok := params["value"]
		if !ok {
			return nil, fmt.Errorf("value parameter is required for set action")
		}
		return m.Set(key, value)
	case "delete":
		key, ok := params["key"].(string)
		if !ok {
			return nil, fmt.Errorf("key parameter is required for delete action")
		}
		return m.Delete(key)
	case "list":
		return m.List()
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// Get retrieves a value from memory
func (m *InMemoryTool) Get(key string) (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	value, exists := m.data[key]
	if !exists {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	return value, nil
}

// Set stores a value in memory
func (m *InMemoryTool) Set(key string, value interface{}) (interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.data[key] = value
	return map[string]interface{}{
		"success": true,
		"key":     key,
	}, nil
}

// Delete removes a value from memory
func (m *InMemoryTool) Delete(key string) (interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.data, key)
	return map[string]interface{}{
		"success": true,
		"key":     key,
	}, nil
}

// List returns all keys in memory
func (m *InMemoryTool) List() (interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	keys := make([]string, 0, len(m.data))
	for key := range m.data {
		keys = append(keys, key)
	}

	return map[string]interface{}{
		"keys":  keys,
		"count": len(keys),
	}, nil
}

// GetSchema returns the JSON schema for this tool
func (m *InMemoryTool) GetSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "The action to perform: get, set, delete, or list",
				"enum":        []string{"get", "set", "delete", "list"},
			},
			"key": map[string]interface{}{
				"type":        "string",
				"description": "The key to operate on (required for get, set, delete)",
			},
			"value": map[string]interface{}{
				"type":        "string",
				"description": "The value to store as JSON string (required for set)",
			},
		},
		"required": []string{"action"},
	}
}

// ToTool converts this to an ADK tool
func (m *InMemoryTool) ToTool() tool.Tool {
	return &inMemoryToolAdapter{tool: m}
}

// inMemoryToolAdapter adapts InMemoryTool to the ADK tool interface
type inMemoryToolAdapter struct {
	tool *InMemoryTool
}

func (a *inMemoryToolAdapter) Name() string {
	return a.tool.Name()
}

func (a *inMemoryToolAdapter) Description() string {
	return a.tool.Description()
}

func (a *inMemoryToolAdapter) IsLongRunning() bool {
	return a.tool.IsLongRunning()
}

func (a *inMemoryToolAdapter) GetTool() interface{} {
	return a.tool
}

func (a *inMemoryToolAdapter) Declaration() *genai.FunctionDeclaration {
	schema := a.tool.GetSchema()
	paramsJSON, _ := json.Marshal(schema)

	return &genai.FunctionDeclaration{
		Name:                 a.tool.Name(),
		Description:          a.tool.Description(),
		ParametersJsonSchema: string(paramsJSON),
	}
}

func (a *inMemoryToolAdapter) Run(ctx tool.Context, args any) (map[string]any, error) {
	// Convert args to map[string]interface{}
	argsMap, ok := args.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected map[string]any, got %T", args)
	}

	result, err := a.tool.Execute(context.Background(), argsMap)
	if err != nil {
		return nil, err
	}

	// Convert result to map[string]any
	resultMap, ok := result.(map[string]any)
	if !ok {
		return map[string]any{"result": result}, nil
	}

	return resultMap, nil
}
