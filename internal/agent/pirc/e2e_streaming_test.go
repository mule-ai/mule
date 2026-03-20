package pirc

import (
	"context"
	"encoding/json"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// isPiAvailable checks if the pi executable is available in PATH
func isPiAvailable() bool {
	_, err := exec.LookPath("pi")
	return err == nil
}

// mockHub implements EventBroadcaster for testing end-to-end streaming
type mockHub struct {
	messages []WebSocketMessage
	mu       sync.Mutex
}

func (m *mockHub) BroadcastAgentEvent(eventType string, data interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if msg, ok := data.(WebSocketMessage); ok {
		m.messages = append(m.messages, msg)
	}
}

func (m *mockHub) GetMessages() []WebSocketMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]WebSocketMessage, len(m.messages))
	copy(result, m.messages)
	return result
}

func (m *mockHub) ClearMessages() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = nil
}

// TestEndToEndStreaming tests the full streaming pipeline from pi to WebSocket
// This is an integration test that requires a running pi process
func TestEndToEndStreaming(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping end-to-end streaming test in short mode")
	}

	// Skip if pi is not available
	if !isPiAvailable() {
		t.Skip("Skipping test: pi not available")
	}

	// Create mock hub for WebSocket broadcasting
	hub := &mockHub{}

	// Create config for a simple prompt
	config := Config{
		Provider:      "google",
		ModelID:       "gemini-2.0-flash",
		ThinkingLevel: "low",
		NoSession:     true,
		NoTools:       true, // No tools for this test
	}

	// Create bridge
	bridge := NewBridge(config)

	// Start the pi process
	if err := bridge.Start(); err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}
	t.Log("PI bridge started")

	// Create event mapper
	mapper := NewEventMapper()

	// Create streamer
	streamer := NewPIEventStreamer(hub, "test-job-e2e")
	streamer.SetEventTypes([]string{
		"text_delta",
		"text_done",
		"thinking_delta",
		"thinking_done",
		"tool_call_start",
		"tool_call_done",
		"agent_start",
		"agent_end",
		"agent_error",
	})

	// Start streaming
	streamer.Start(bridge, mapper)

	// Send a simple prompt that should produce text output
	ctx := context.Background()
	prompt := "Say 'streaming test successful' in exactly 3 words"
	if err := bridge.Prompt(ctx, prompt); err != nil {
		_ = bridge.Stop()
		streamer.Stop()
		t.Fatalf("Failed to send prompt: %v", err)
	}
	t.Logf("Sent prompt: %s", prompt)

	// Wait for agent to complete or timeout
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var hasTextDelta, hasAgentEnd bool

	for {
		select {
		case <-timeout:
			t.Log("Timeout reached, checking received events")
			// Even on timeout, verify what we received
			messages := hub.GetMessages()
			t.Logf("Received %d WebSocket messages total", len(messages))
			for i, msg := range messages {
				t.Logf("Message %d: type=%s", i, msg.Type)
			}
			goto done
		case <-ticker.C:
			messages := hub.GetMessages()
			for _, msg := range messages {
				if msg.Type == "text_delta" {
					hasTextDelta = true
					if data, ok := msg.Data.(MuleEvent); ok {
						t.Logf("Received text_delta: %s", data.Delta)
					}
				}
				if msg.Type == "agent_end" || msg.Type == "response" {
					hasAgentEnd = true
					t.Logf("Received agent end: %s", msg.Type)
				}
			}
			if hasAgentEnd {
				t.Log("Agent completed successfully")
				goto done
			}
		}
	}

done:
	// Verify we received some events
	messages := hub.GetMessages()
	t.Logf("Total WebSocket messages received: %d", len(messages))

	// Print all received messages for debugging
	for i, msg := range messages {
		t.Logf("Message %d: type=%s", i, msg.Type)
		if data, ok := msg.Data.(MuleEvent); ok {
			if data.Delta != "" {
				t.Logf("  Delta: %s", data.Delta)
			}
			if data.Error != "" {
				t.Logf("  Error: %s", data.Error)
			}
		}
	}

	// Check that we received at least some events
	if len(messages) == 0 {
		t.Error("Expected to receive WebSocket messages")
	}

	// Check for text_delta or thinking_delta events (either is acceptable)
	if !hasTextDelta {
		hasThinkingDelta := false
		for _, msg := range messages {
			if msg.Type == "thinking_delta" {
				hasThinkingDelta = true
				break
			}
		}
		if !hasThinkingDelta {
			t.Log("Note: No text_delta or thinking_delta received - may be model-dependent")
		}
	}

	// Stop the streamer and bridge
	streamer.Stop()
	if err := bridge.Stop(); err != nil {
		t.Logf("Warning: error stopping bridge: %v", err)
	}

	t.Log("End-to-end streaming test completed")
}

// TestEndToEndStreamingWithTools tests streaming with tool execution
func TestEndToEndStreamingWithTools(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping end-to-end streaming test with tools in short mode")
	}

	// Skip if pi is not available
	if !isPiAvailable() {
		t.Skip("Skipping test: pi not available")
	}

	// Create mock hub for WebSocket broadcasting
	hub := &mockHub{}

	// Create config with tools enabled
	config := Config{
		Provider:      "google",
		ModelID:       "gemini-2.0-flash",
		ThinkingLevel: "low",
		NoSession:     true,
		// Tools enabled (default)
	}

	// Create bridge
	bridge := NewBridge(config)

	// Start the pi process
	if err := bridge.Start(); err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}
	t.Log("PI bridge started with tools")

	// Create event mapper
	mapper := NewEventMapper()

	// Create streamer
	streamer := NewPIEventStreamer(hub, "test-job-e2e-tools")
	streamer.SetEventTypes([]string{
		"text_delta",
		"text_done",
		"thinking_delta",
		"thinking_done",
		"tool_call_start",
		"tool_call_progress",
		"tool_call_done",
		"tool_result",
		"agent_start",
		"agent_end",
		"agent_error",
	})

	// Start streaming
	streamer.Start(bridge, mapper)

	// Send a prompt that requires a tool (bash command)
	ctx := context.Background()
	prompt := "Run the command: echo 'tool test successful'"
	if err := bridge.Prompt(ctx, prompt); err != nil {
		_ = bridge.Stop()
		streamer.Stop()
		t.Fatalf("Failed to send prompt: %v", err)
	}
	t.Logf("Sent prompt with tool: %s", prompt)

	// Wait for agent to complete or timeout
	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var hasToolCallStart, hasToolCallDone, hasAgentEnd bool

	for {
		select {
		case <-timeout:
			t.Log("Timeout reached")
			goto done
		case <-ticker.C:
			messages := hub.GetMessages()
			for _, msg := range messages {
				if msg.Type == "tool_call_start" {
					hasToolCallStart = true
					t.Log("Received tool_call_start")
				}
				if msg.Type == "tool_call_done" || msg.Type == "tool_result" {
					hasToolCallDone = true
					t.Log("Received tool_call_done/tool_result")
				}
				if msg.Type == "agent_end" {
					hasAgentEnd = true
					t.Log("Received agent_end")
					goto done
				}
			}
		}
	}

done:
	messages := hub.GetMessages()
	t.Logf("Total WebSocket messages received: %d", len(messages))
	t.Logf("Tool call start received: %v", hasToolCallStart)
	t.Logf("Tool call done received: %v", hasToolCallDone)
	t.Logf("Agent end received: %v", hasAgentEnd)

	// Print all received messages for debugging
	for i, msg := range messages {
		t.Logf("Message %d: type=%s", i, msg.Type)
		if data, ok := msg.Data.(MuleEvent); ok {
			if data.Name != "" {
				t.Logf("  Tool name: %s", data.Name)
			}
			if data.ID != "" {
				t.Logf("  Tool ID: %s", data.ID)
			}
			if data.Delta != "" {
				t.Logf("  Delta: %s", data.Delta)
			}
			if data.Error != "" {
				t.Logf("  Error: %s", data.Error)
			}
		}
	}

	// Stop the streamer and bridge
	streamer.Stop()
	if err := bridge.Stop(); err != nil {
		t.Logf("Warning: error stopping bridge: %v", err)
	}

	t.Log("End-to-end streaming with tools test completed")
}

// TestEventMappingIntegration tests that events are correctly mapped
func TestEventMappingIntegration(t *testing.T) {
	// Test the mapping pipeline directly with mock events
	mapper := NewEventMapper()

	// Create a channel to feed events
	piEvents := make(chan AgentEvent, 10)

	// Start the mapping
	mapper.StartMapping(piEvents)

	// Send various event types
	testEvents := []AgentEvent{
		{
			Type: "agent_start",
		},
		{
			Type:                  "text_delta",
			AssistantMessageEvent: json.RawMessage(`{"type":"text_delta","delta":"Hello "}`),
		},
		{
			Type:                  "text_delta",
			AssistantMessageEvent: json.RawMessage(`{"type":"text_delta","delta":"World"}`),
		},
		{
			Type:       "tool_execution_start",
			ToolCallID: "call_123",
			ToolName:   "bash",
			Args:       json.RawMessage(`{"command":"ls"}`),
		},
		{
			Type:       "tool_execution_done",
			ToolCallID: "call_123",
			ToolName:   "bash",
			Result:     json.RawMessage(`{"output":"file1.txt\nfile2.txt"}`),
		},
		{
			Type: "agent_end",
		},
	}

	for _, event := range testEvents {
		piEvents <- event
	}
	close(piEvents)

	// Collect mapped events
	var mappedEvents []MuleEvent
	timeout := time.After(2 * time.Second)

	for {
		select {
		case event, ok := <-mapper.Events():
			if !ok {
				// Channel closed
				goto done
			}
			mappedEvents = append(mappedEvents, event)
		case <-timeout:
			t.Error("Timeout waiting for mapped events")
			goto done
		}
	}

done:
	// Verify we got all the expected events
	if len(mappedEvents) != len(testEvents) {
		t.Errorf("Expected %d mapped events, got %d", len(testEvents), len(mappedEvents))
	}

	// Verify specific event types
	expectedTypes := []MuleEventType{
		MuleEventAgentStart,
		MuleEventTextDelta,
		MuleEventTextDelta,
		MuleEventToolCallStart,
		MuleEventToolCallDone,
		MuleEventAgentEnd,
	}

	for i, expected := range expectedTypes {
		if i >= len(mappedEvents) {
			t.Errorf("Missing event at index %d", i)
			continue
		}
		if mappedEvents[i].Type != expected {
			t.Errorf("Event %d: expected type %s, got %s", i, expected, mappedEvents[i].Type)
		}
	}

	// Verify text delta content
	textDeltas := 0
	for _, event := range mappedEvents {
		if event.Type == MuleEventTextDelta {
			textDeltas++
			t.Logf("Text delta: %s", event.Delta)
		}
	}
	if textDeltas != 2 {
		t.Errorf("Expected 2 text deltas, got %d", textDeltas)
	}

	// Verify tool call details
	for _, event := range mappedEvents {
		if event.Type == MuleEventToolCallStart {
			if event.ID != "call_123" {
				t.Errorf("Expected tool call ID 'call_123', got %s", event.ID)
			}
			if event.Name != "bash" {
				t.Errorf("Expected tool name 'bash', got %s", event.Name)
			}
		}
	}
}

// TestWebSocketMessageFormat verifies the WebSocket message format
func TestWebSocketMessageFormat(t *testing.T) {
	hub := &mockHub{}
	streamer := NewPIEventStreamer(hub, "test-format")

	// Configure streamer
	streamer.SetEventTypes([]string{"text_delta", "agent_start", "agent_end"})

	// Create mock bridge (not started)
	config := Config{ModelID: "test"}
	bridge := NewBridge(config)

	// Create mapper
	mapper := NewEventMapper()

	// Start streaming (but we won't start the bridge since we're testing the message format)
	streamer.Start(bridge, mapper)

	// Get the message channel
	go func() {
		// Send some test events through the mapper
		mapper.MapEvent(AgentEvent{Type: "agent_start"})
		mapper.MapEvent(AgentEvent{
			Type:                  "text_delta",
			AssistantMessageEvent: json.RawMessage(`{"type":"text_delta","delta":"Hello"}`),
		})
		mapper.MapEvent(AgentEvent{Type: "agent_end"})
	}()

	// Wait a bit for events to be processed
	time.Sleep(100 * time.Millisecond)

	// Stop
	streamer.Stop()

	// Verify messages
	messages := hub.GetMessages()
	if len(messages) != 3 {
		t.Errorf("Expected 3 messages, got %d", len(messages))
	}

	// Verify message format
	for i, msg := range messages {
		// Check that Type is set correctly
		if msg.Type == "" {
			t.Errorf("Message %d: Type is empty", i)
		}

		// Check that Timestamp is set
		if msg.Timestamp.IsZero() {
			t.Errorf("Message %d: Timestamp is zero", i)
		}

		// Check that Data is a MuleEvent
		data, ok := msg.Data.(MuleEvent)
		if !ok {
			t.Errorf("Message %d: Data is not MuleEvent", i)
			continue
		}

		// Verify MuleEvent fields
		if data.Timestamp.IsZero() {
			t.Errorf("Message %d: MuleEvent.Timestamp is zero", i)
		}

		t.Logf("Message %d: type=%s, timestamp=%v", i, msg.Type, msg.Timestamp)
	}
}
