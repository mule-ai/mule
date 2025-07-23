package mcp

import (
	"testing"

	"github.com/go-logr/logr"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		want   bool
	}{
		{
			name:   "nil config returns nil",
			config: nil,
			want:   false,
		},
		{
			name: "disabled config returns nil",
			config: &Config{
				Enabled: false,
			},
			want: false,
		},
		{
			name: "enabled config returns instance",
			config: &Config{
				Enabled: true,
				Servers: map[string]MCPServer{
					"test": {
						Command: "echo",
						Args:    []string{"test"},
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logr.Discard()
			got := New(tt.config, logger)
			if tt.want {
				if got == nil {
					t.Errorf("New() returned nil, want non-nil")
				}
				if got != nil && got.Name() != "mcp" {
					t.Errorf("Name() = %v, want %v", got.Name(), "mcp")
				}
			} else {
				if got != nil {
					t.Errorf("New() returned non-nil, want nil")
				}
			}
		})
	}
}

func TestMCP_Call(t *testing.T) {
	logger := logr.Discard()
	config := &Config{
		Enabled: true,
		Servers: map[string]MCPServer{
			"test": {
				Command: "echo",
				Args:    []string{"test"},
			},
		},
	}
	
	mcp := New(config, logger)
	if mcp == nil {
		t.Fatal("New() returned nil")
	}

	tests := []struct {
		name    string
		method  string
		data    any
		wantErr bool
	}{
		{
			name:    "start command",
			method:  "start",
			data:    nil,
			wantErr: false, // Will fail to start echo as MCP server, but shouldn't error
		},
		{
			name:    "stop command",
			method:  "stop",
			data:    nil,
			wantErr: false,
		},
		{
			name:    "list_tools command",
			method:  "list_tools",
			data:    nil,
			wantErr: false,
		},
		{
			name:    "call_tool without data",
			method:  "call_tool",
			data:    nil,
			wantErr: true,
		},
		{
			name:    "call_tool with invalid data",
			method:  "call_tool",
			data:    "invalid",
			wantErr: true,
		},
		{
			name: "call_tool with valid data",
			method: "call_tool",
			data: map[string]interface{}{
				"server": "test",
				"tool":   "test_tool",
				"params": map[string]string{"key": "value"},
			},
			wantErr: true, // Will error because server not actually running
		},
		{
			name:    "unknown method",
			method:  "unknown",
			data:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mcp.Call(tt.method, tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("Call() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestMCP_ChatHistory(t *testing.T) {
	logger := logr.Discard()
	config := &Config{
		Enabled: true,
	}
	
	mcp := New(config, logger)
	if mcp == nil {
		t.Fatal("New() returned nil")
	}

	// Test GetChatHistory
	_, err := mcp.GetChatHistory("test", 10)
	if err == nil {
		t.Error("GetChatHistory() expected error, got nil")
	}

	// Test ClearChatHistory
	err = mcp.ClearChatHistory("test")
	if err == nil {
		t.Error("ClearChatHistory() expected error, got nil")
	}
}

func TestMCPToolAdapter(t *testing.T) {
	logger := logr.Discard()
	config := &Config{
		Enabled: true,
	}
	
	mcp := New(config, logger)
	if mcp == nil {
		t.Fatal("New() returned nil")
	}

	tool := Tool{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"input": map[string]interface{}{
					"type": "string",
				},
			},
		},
	}

	adapter := NewMCPToolAdapter(mcp, "test_server", "test_tool", tool)
	
	// Test Name
	expectedName := "mcp_test_server_test_tool"
	if name := adapter.Name(); name != expectedName {
		t.Errorf("Name() = %v, want %v", name, expectedName)
	}
	
	// Test Description
	if desc := adapter.Description(); desc != "A test tool" {
		t.Errorf("Description() = %v, want %v", desc, "A test tool")
	}
	
	// Test Attributes
	attrs := adapter.Attributes()
	if attrs["server"] != "test_server" {
		t.Errorf("Attributes()[server] = %v, want %v", attrs["server"], "test_server")
	}
	if attrs["mcp_tool"] != "test_tool" {
		t.Errorf("Attributes()[mcp_tool] = %v, want %v", attrs["mcp_tool"], "test_tool")
	}
}

func TestToolRegistry(t *testing.T) {
	// Create a new registry for testing
	registry := &ToolRegistry{
		tools: make(map[string]interface{}),
	}

	// Mock tool
	mockTool := &MCPToolAdapter{
		serverName: "test",
		toolName:   "mock",
		tool: Tool{
			Name:        "mock",
			Description: "Mock tool",
		},
	}

	// Test Register
	registry.Register(mockTool)
	
	// Test Get
	tool, err := registry.Get("mcp_test_mock")
	if err != nil {
		t.Errorf("Get() unexpected error: %v", err)
	}
	if tool == nil {
		t.Error("Get() returned nil tool")
	}
	
	// Test Get non-existent
	_, err = registry.Get("non_existent")
	if err == nil {
		t.Error("Get() expected error for non-existent tool")
	}
	
	// Test List
	names := registry.List()
	found := false
	for _, name := range names {
		if name == "mcp_test_mock" {
			found = true
			break
		}
	}
	if !found {
		t.Error("List() did not contain registered tool")
	}
	
	// Test Clear
	registry.Clear()
	names = registry.List()
	if len(names) != 0 {
		t.Errorf("Clear() failed, List() returned %d tools", len(names))
	}
}