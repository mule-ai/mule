package pirc

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Config holds configuration for the PI RPC bridge
type Config struct {
	Provider         string
	ModelID          string
	APIKey           string
	SystemPrompt     string
	ThinkingLevel    string // off, minimal, low, medium, high, xhigh
	SessionDir       string
	NoSession        bool
	Skills           []string // paths to skill directories
	Tools            string   // comma-separated list of tools
	NoTools          bool
	Extensions       []string
	NoExtensions     bool
	WorkingDirectory string
	Timeout          time.Duration
}

// ImageContent represents an image for PI RPC
type ImageContent struct {
	Type     string `json:"type"`
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

// PromptMessage represents a prompt command for PI RPC
type PromptMessage struct {
	Type              string         `json:"type"`
	Message           string         `json:"message"`
	Images            []ImageContent `json:"images,omitempty"`
	StreamingBehavior string         `json:"streamingBehavior,omitempty"`
	ID                string         `json:"id,omitempty"`
}

// SteerMessage represents a steer command for PI RPC
type SteerMessage struct {
	Type    string         `json:"type"`
	Message string         `json:"message"`
	Images  []ImageContent `json:"images,omitempty"`
	ID      string         `json:"id,omitempty"`
}

// FollowUpMessage represents a follow_up command for PI RPC
type FollowUpMessage struct {
	Type    string         `json:"type"`
	Message string         `json:"message"`
	Images  []ImageContent `json:"images,omitempty"`
	ID      string         `json:"id,omitempty"`
}

// AbortMessage represents an abort command for PI RPC
type AbortMessage struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
}

// NewSessionMessage represents a new_session command for PI RPC
type NewSessionMessage struct {
	Type          string `json:"type"`
	ParentSession string `json:"parentSession,omitempty"`
	ID            string `json:"id,omitempty"`
}

// SetModelMessage represents a set_model command for PI RPC
type SetModelMessage struct {
	Type     string `json:"type"`
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`
	ID       string `json:"id,omitempty"`
}

// SetThinkingLevelMessage represents a set_thinking_level command for PI RPC
type SetThinkingLevelMessage struct {
	Type  string `json:"type"`
	Level string `json:"level"`
	ID    string `json:"id,omitempty"`
}

// BashMessage represents a bash command for PI RPC
type BashMessage struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	ID      string `json:"id,omitempty"`
}

// RPCCommand is a generic interface for RPC commands
type RPCCommand interface{}

// RPCResponse represents a response from PI RPC
type RPCResponse struct {
	Type    string          `json:"type"`
	Command string          `json:"command,omitempty"`
	Success bool            `json:"success"`
	Error   string          `json:"error,omitempty"`
	ID      string          `json:"id,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// AgentEvent represents an event from PI
type AgentEvent struct {
	Type                  string          `json:"type"`
	Message               json.RawMessage `json:"message,omitempty"`
	Messages              json.RawMessage `json:"messages,omitempty"`
	AssistantMessageEvent json.RawMessage `json:"assistantMessageEvent,omitempty"`
	ToolCallID            string          `json:"toolCallId,omitempty"`
	ToolName              string          `json:"toolName,omitempty"`
	Args                  json.RawMessage `json:"args,omitempty"`
	PartialResult         json.RawMessage `json:"partialResult,omitempty"`
	Result                json.RawMessage `json:"result,omitempty"`
	IsError               bool            `json:"isError,omitempty"`
}

// ExtensionUIRequest represents an extension UI request
type ExtensionUIRequest struct {
	Type    string   `json:"type"`
	ID      string   `json:"id"`
	Method  string   `json:"method"`
	Title   string   `json:"title,omitempty"`
	Message string   `json:"message,omitempty"`
	Options []string `json:"options,omitempty"`
	Timeout int64    `json:"timeout,omitempty"`
}

// ExtensionUIResponse represents a response to an extension UI request
type ExtensionUIResponse struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Value     string `json:"value,omitempty"`
	Confirmed bool   `json:"confirmed,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`
}

// EventHandler is a function type for handling events
type EventHandler func(event AgentEvent)

// Bridge represents a PI RPC bridge
type Bridge struct {
	cfg         Config
	cmd         *exec.Cmd
	stdin       *bufio.Writer
	stdout      *bufio.Scanner
	stderr      io.Reader
	eventChan   chan AgentEvent
	errChan     chan error
	mu          sync.Mutex
	closed      bool
	processDone chan struct{}
}

// NewBridge creates a new PI RPC bridge
func NewBridge(cfg Config) *Bridge {
	return &Bridge{
		cfg:         cfg,
		eventChan:   make(chan AgentEvent, 100),
		errChan:     make(chan error, 10),
		processDone: make(chan struct{}),
	}
}

// Start starts the PI process
func (b *Bridge) Start() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return fmt.Errorf("bridge is closed")
	}

	args := b.buildArgs()
	args = append(args, "--mode", "rpc")

	// Always use --no-session for Mule integration (ephemeral mode)
	args = append(args, "--no-session")

	fmt.Printf("Starting PI with args: %v\n", args)

	b.cmd = exec.Command("pi", args...)
	b.cmd.Env = os.Environ()

	// Set up environment variables for API key if provided
	if b.cfg.APIKey != "" {
		b.cmd.Env = append(b.cmd.Env, "ANTHROPIC_API_KEY="+b.cfg.APIKey)
	}

	stdin, err := b.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	b.stdin = bufio.NewWriter(stdin)

	stdout, err := b.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	b.stdout = bufio.NewScanner(stdout)

	stderr, err := b.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}
	b.stderr = stderr

	// Set working directory if specified
	if b.cfg.WorkingDirectory != "" {
		b.cmd.Dir = b.cfg.WorkingDirectory
	}

	if err := b.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start pi: %w", err)
	}

	// Start goroutine to read stdout events
	go b.readEvents()

	// Start goroutine to read stderr (for logging)
	go b.readStderr()

	// Start goroutine to wait for process exit
	go b.waitForExit()

	return nil
}

// GetArgs returns the command line arguments built from config
func (b *Bridge) GetArgs() []string {
	return b.buildArgs()
}

// buildArgs builds command line arguments from config
func (b *Bridge) buildArgs() []string {
	var args []string

	if b.cfg.Provider != "" {
		args = append(args, "--provider", b.cfg.Provider)
	}

	if b.cfg.ModelID != "" {
		args = append(args, "--model", b.cfg.ModelID)
	}

	if b.cfg.SystemPrompt != "" {
		args = append(args, "--system-prompt", b.cfg.SystemPrompt)
	}

	if b.cfg.ThinkingLevel != "" {
		args = append(args, "--thinking", b.cfg.ThinkingLevel)
	}

	if b.cfg.SessionDir != "" {
		args = append(args, "--session-dir", b.cfg.SessionDir)
	}

	for _, skill := range b.cfg.Skills {
		args = append(args, "--skill", skill)
	}

	if b.cfg.NoTools {
		args = append(args, "--no-tools")
	} else if b.cfg.Tools != "" {
		args = append(args, "--tools", b.cfg.Tools)
	}

	if b.cfg.NoExtensions {
		args = append(args, "--no-extensions")
	}

	for _, ext := range b.cfg.Extensions {
		args = append(args, "--extension", ext)
	}

	return args
}

// readEvents reads events from stdout
func (b *Bridge) readEvents() {
	for b.stdout.Scan() {
		line := b.stdout.Text()
		if line == "" {
			continue
		}

		var event AgentEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			fmt.Printf("Failed to parse event: %v, line: %s\n", err, line)
			continue
		}

		// Check for extension UI requests - pass them through to the event channel
		// instead of auto-cancelling so the client can respond
		if event.Type == "extension_ui_request" {
			select {
			case b.eventChan <- event:
			default:
				fmt.Printf("Event channel full, dropping extension UI request: %s\n", event.Type)
			}
			continue
		}

		select {
		case b.eventChan <- event:
		default:
			fmt.Printf("Event channel full, dropping event: %s\n", event.Type)
		}
	}

	if err := b.stdout.Err(); err != nil {
		b.errChan <- fmt.Errorf("stdout scanner error: %w", err)
	}
}

// readStderr reads stderr for logging
func (b *Bridge) readStderr() {
	buf := make([]byte, 1024)
	for {
		n, err := b.stderr.Read(buf)
		if n > 0 {
			fmt.Printf("PI stderr: %s\n", string(buf[:n]))
		}
		if err != nil {
			break
		}
	}
}

// waitForExit waits for the process to exit
func (b *Bridge) waitForExit() {
	err := b.cmd.Wait()
	if err != nil {
		b.errChan <- fmt.Errorf("pi process exited with error: %w", err)
	}
	close(b.processDone)
}

// sendCommand sends a command to PI
func (b *Bridge) sendCommand(cmd RPCCommand) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return fmt.Errorf("bridge is closed")
	}

	if b.stdin == nil {
		return fmt.Errorf("bridge is not running")
	}

	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("failed to marshal command: %w", err)
	}

	_, err = b.stdin.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write command: %w", err)
	}

	_, err = b.stdin.Write([]byte("\n"))
	if err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	err = b.stdin.Flush()
	if err != nil {
		return fmt.Errorf("failed to flush stdin: %w", err)
	}

	return nil
}

// SendExtensionUICancel sends a cancellation response for extension UI requests
// This is exported for use by external packages that need to cancel extension UI requests
func (b *Bridge) SendExtensionUICancel(id string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return fmt.Errorf("bridge is closed")
	}

	if b.stdin == nil {
		return fmt.Errorf("bridge is not running")
	}

	resp := ExtensionUIResponse{
		Type:      "extension_ui_response",
		ID:        id,
		Cancelled: true,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal UI response: %w", err)
	}

	_, err = b.stdin.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write UI response: %w", err)
	}

	_, err = b.stdin.Write([]byte("\n"))
	if err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	return b.stdin.Flush()
}

// SendExtensionUIResponse sends a response to an extension UI request
// This is a public method that can be called by the WebSocket handler when
// the client responds to a UI request (select, confirm, input, etc.)
func (b *Bridge) SendExtensionUIResponse(id, value string, confirmed bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return fmt.Errorf("bridge is closed")
	}

	if b.stdin == nil {
		return fmt.Errorf("bridge is not running")
	}

	resp := ExtensionUIResponse{
		Type:      "extension_ui_response",
		ID:        id,
		Value:     value,
		Confirmed: confirmed,
		Cancelled: false,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("failed to marshal UI response: %w", err)
	}

	_, err = b.stdin.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write UI response: %w", err)
	}

	_, err = b.stdin.Write([]byte("\n"))
	if err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	return b.stdin.Flush()
}

// Prompt sends a prompt to PI and returns immediately
func (b *Bridge) Prompt(ctx context.Context, message string) error {
	msg := PromptMessage{
		Type:    "prompt",
		Message: message,
		ID:      uuid.New().String(),
	}
	return b.sendCommand(msg)
}

// PromptWithImages sends a prompt with images to PI
func (b *Bridge) PromptWithImages(ctx context.Context, message string, images []ImageContent) error {
	msg := PromptMessage{
		Type:    "prompt",
		Message: message,
		Images:  images,
		ID:      uuid.New().String(),
	}
	return b.sendCommand(msg)
}

// Steer sends a steering message to interrupt the agent
func (b *Bridge) Steer(ctx context.Context, message string) error {
	msg := SteerMessage{
		Type:    "steer",
		Message: message,
		ID:      uuid.New().String(),
	}
	return b.sendCommand(msg)
}

// FollowUp queues a follow-up message
func (b *Bridge) FollowUp(ctx context.Context, message string) error {
	msg := FollowUpMessage{
		Type:    "follow_up",
		Message: message,
		ID:      uuid.New().String(),
	}
	return b.sendCommand(msg)
}

// Abort aborts the current operation
func (b *Bridge) Abort(ctx context.Context) error {
	msg := AbortMessage{
		Type: "abort",
	}
	return b.sendCommand(msg)
}

// NewSession starts a new session
func (b *Bridge) NewSession(ctx context.Context) error {
	msg := NewSessionMessage{
		Type: "new_session",
	}
	return b.sendCommand(msg)
}

// SetModel switches to a specific model
func (b *Bridge) SetModel(ctx context.Context, provider, modelID string) error {
	msg := SetModelMessage{
		Type:     "set_model",
		Provider: provider,
		ModelID:  modelID,
	}
	return b.sendCommand(msg)
}

// SetThinkingLevel sets the thinking level
func (b *Bridge) SetThinkingLevel(ctx context.Context, level string) error {
	msg := SetThinkingLevelMessage{
		Type:  "set_thinking_level",
		Level: level,
	}
	return b.sendCommand(msg)
}

// Bash executes a bash command
func (b *Bridge) Bash(ctx context.Context, command string) error {
	msg := BashMessage{
		Type:    "bash",
		Command: command,
	}
	return b.sendCommand(msg)
}

// Events returns a channel of events
func (b *Bridge) Events() <-chan AgentEvent {
	return b.eventChan
}

// Errors returns a channel of errors
func (b *Bridge) Errors() <-chan error {
	return b.errChan
}

// ProcessDone returns a channel that is closed when the process exits
func (b *Bridge) ProcessDone() <-chan struct{} {
	return b.processDone
}

// Stop stops the PI process
func (b *Bridge) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true

	if b.cmd != nil && b.cmd.Process != nil {
		// Try graceful shutdown first
		err := b.cmd.Process.Signal(os.Interrupt)
		if err != nil {
			return fmt.Errorf("failed to signal process: %w", err)
		}

		// Wait for graceful shutdown with timeout
		select {
		case <-b.processDone:
			return nil
		case <-time.After(5 * time.Second):
			// Force kill if graceful shutdown fails
			err := b.cmd.Process.Kill()
			if err != nil {
				return fmt.Errorf("failed to kill process: %w", err)
			}
			<-b.processDone
		}
	}

	close(b.eventChan)
	close(b.errChan)

	return nil
}

// IsRunning returns true if the process is running
func (b *Bridge) IsRunning() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed || b.cmd == nil || b.cmd.Process == nil {
		return false
	}

	select {
	case <-b.processDone:
		return false
	default:
		return true
	}
}
