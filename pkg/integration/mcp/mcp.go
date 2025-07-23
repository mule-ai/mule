package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/go-logr/logr"
	"github.com/mule-ai/mule/pkg/types"
)

// Config holds the configuration for MCP servers
type Config struct {
	Enabled bool                 `json:"enabled"`
	Servers map[string]MCPServer `json:"servers"`
}

// MCPServer represents a single MCP server configuration
type MCPServer struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// MCP implements the Integration interface for MCP support
type MCP struct {
	config   *Config
	logger   logr.Logger
	servers  map[string]*mcpServerInstance
	mu       sync.RWMutex
	channels map[string]chan any
}

// mcpServerInstance represents a running MCP server
type mcpServerInstance struct {
	name    string
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	scanner *bufio.Scanner
	ctx     context.Context
	cancel  context.CancelFunc
	tools   map[string]Tool
	mu      sync.RWMutex
}

// Tool represents an MCP tool
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// Message represents an MCP protocol message
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *ErrorResponse  `json:"error,omitempty"`
}

// ErrorResponse represents an MCP error
type ErrorResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// New creates a new MCP integration
func New(config *Config, logger logr.Logger) *MCP {
	if config == nil || !config.Enabled {
		return nil
	}

	return &MCP{
		config:   config,
		logger:   logger,
		servers:  make(map[string]*mcpServerInstance),
		channels: make(map[string]chan any),
	}
}

// Name returns the name of the integration
func (m *MCP) Name() string {
	return "mcp"
}

// GetChannel returns the channel for the integration
func (m *MCP) GetChannel() chan any {
	return nil // MCP doesn't use a global channel
}

// RegisterTrigger registers a trigger for the integration
func (m *MCP) RegisterTrigger(trigger string, data any, channel chan any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels[trigger] = channel
}

// GetChatHistory returns chat history (not applicable for MCP)
func (m *MCP) GetChatHistory(channelID string, limit int) (string, error) {
	return "", fmt.Errorf("chat history not supported for MCP integration")
}

// ClearChatHistory clears chat history (not applicable for MCP)
func (m *MCP) ClearChatHistory(channelID string) error {
	return fmt.Errorf("chat history not supported for MCP integration")
}

// Call executes an MCP call
func (m *MCP) Call(name string, data any) (any, error) {
	switch name {
	case "start":
		return nil, m.Start()
	case "stop":
		return nil, m.Stop()
	case "list_tools":
		return m.ListTools()
	case "call_tool":
		callData, ok := data.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid data for call_tool")
		}
		serverName, ok := callData["server"].(string)
		if !ok {
			return nil, fmt.Errorf("server name required")
		}
		toolName, ok := callData["tool"].(string)
		if !ok {
			return nil, fmt.Errorf("tool name required")
		}
		params := callData["params"]
		return m.CallTool(serverName, toolName, params)
	default:
		return nil, fmt.Errorf("unknown MCP call: %s", name)
	}
}

// Start initializes and starts all configured MCP servers
func (m *MCP) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, serverConfig := range m.config.Servers {
		if err := m.startServer(name, serverConfig); err != nil {
			m.logger.Error(err, "failed to start MCP server", "server", name)
			// Continue starting other servers
		}
	}

	return nil
}

// Stop stops all running MCP servers
func (m *MCP) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, server := range m.servers {
		m.logger.Info("stopping MCP server", "server", name)
		server.cancel()
		if server.cmd != nil && server.cmd.Process != nil {
			server.cmd.Process.Kill()
		}
	}

	m.servers = make(map[string]*mcpServerInstance)
	return nil
}

// ListTools returns all available tools from all servers
func (m *MCP) ListTools() (map[string][]Tool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tools := make(map[string][]Tool)
	for name, server := range m.servers {
		server.mu.RLock()
		serverTools := make([]Tool, 0, len(server.tools))
		for _, tool := range server.tools {
			serverTools = append(serverTools, tool)
		}
		server.mu.RUnlock()
		tools[name] = serverTools
	}

	return tools, nil
}

// CallTool calls a specific tool on a specific server
func (m *MCP) CallTool(serverName, toolName string, params interface{}) (interface{}, error) {
	m.mu.RLock()
	server, exists := m.servers[serverName]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("server %s not found", serverName)
	}

	// Check if tool exists
	server.mu.RLock()
	_, toolExists := server.tools[toolName]
	server.mu.RUnlock()

	if !toolExists {
		return nil, fmt.Errorf("tool %s not found on server %s", toolName, serverName)
	}

	// Create tool call request
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params: %w", err)
	}

	request := Message{
		JSONRPC: "2.0",
		ID:      1, // TODO: Use proper ID management
		Method:  "tools/call",
		Params: json.RawMessage(fmt.Sprintf(`{"name": "%s", "arguments": %s}`, toolName, string(paramsJSON))),
	}

	// Send request and wait for response
	response, err := server.sendRequest(request)
	if err != nil {
		return nil, err
	}

	if response.Error != nil {
		return nil, fmt.Errorf("MCP error: %s (code: %d)", response.Error.Message, response.Error.Code)
	}

	var result interface{}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return result, nil
}

// startServer starts a single MCP server
func (m *MCP) startServer(name string, config MCPServer) error {
	ctx, cancel := context.WithCancel(context.Background())

	// Set up the command
	cmd := exec.CommandContext(ctx, config.Command, config.Args...)
	
	// Set environment variables
	env := os.Environ()
	for k, v := range config.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	// Set up pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("failed to start command: %w", err)
	}

	server := &mcpServerInstance{
		name:    name,
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		scanner: bufio.NewScanner(stdout),
		ctx:     ctx,
		cancel:  cancel,
		tools:   make(map[string]Tool),
	}

	m.servers[name] = server

	// Start reading from the server
	go server.readLoop(m.logger.WithName(name))

	// Initialize the server
	if err := server.initialize(); err != nil {
		cancel()
		delete(m.servers, name)
		return fmt.Errorf("failed to initialize server: %w", err)
	}

	m.logger.Info("MCP server started", "server", name)
	return nil
}

// readLoop reads messages from the server
func (s *mcpServerInstance) readLoop(logger logr.Logger) {
	defer s.cancel()

	for s.scanner.Scan() {
		line := s.scanner.Text()
		
		var msg Message
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			logger.Error(err, "failed to unmarshal message", "line", line)
			continue
		}

		// Handle the message based on method
		if msg.Method != "" {
			s.handleNotification(msg, logger)
		}
	}

	if err := s.scanner.Err(); err != nil {
		logger.Error(err, "scanner error")
	}
}

// handleNotification handles notification messages from the server
func (s *mcpServerInstance) handleNotification(msg Message, logger logr.Logger) {
	switch msg.Method {
	case "tools/list":
		// Server is listing available tools
		var tools struct {
			Tools []Tool `json:"tools"`
		}
		if err := json.Unmarshal(msg.Params, &tools); err != nil {
			logger.Error(err, "failed to unmarshal tools list")
			return
		}

		s.mu.Lock()
		s.tools = make(map[string]Tool)
		for _, tool := range tools.Tools {
			s.tools[tool.Name] = tool
		}
		s.mu.Unlock()

		logger.Info("received tools list", "count", len(tools.Tools))
	default:
		logger.V(1).Info("received notification", "method", msg.Method)
	}
}

// sendRequest sends a request to the server and waits for response
func (s *mcpServerInstance) sendRequest(request Message) (*Message, error) {
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send the request
	if _, err := fmt.Fprintf(s.stdin, "%s\n", requestJSON); err != nil {
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// TODO: Implement proper response handling with ID matching
	// For now, this is a simplified version
	return nil, fmt.Errorf("response handling not yet implemented")
}

// initialize sends the initialization sequence to the server
func (s *mcpServerInstance) initialize() error {
	// Send initialize request
	initRequest := Message{
		JSONRPC: "2.0",
		ID:      "init",
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion": "0.1.0", "capabilities": {}}`),
	}

	initJSON, err := json.Marshal(initRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal init request: %w", err)
	}

	if _, err := fmt.Fprintf(s.stdin, "%s\n", initJSON); err != nil {
		return fmt.Errorf("failed to send init request: %w", err)
	}

	// Request tools list
	toolsRequest := Message{
		JSONRPC: "2.0",
		ID:      "tools",
		Method:  "tools/list",
		Params:  json.RawMessage(`{}`),
	}

	toolsJSON, err := json.Marshal(toolsRequest)
	if err != nil {
		return fmt.Errorf("failed to marshal tools request: %w", err)
	}

	if _, err := fmt.Fprintf(s.stdin, "%s\n", toolsJSON); err != nil {
		return fmt.Errorf("failed to send tools request: %w", err)
	}

	return nil
}