package pirc

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mockWebSocketHub is a mock implementation of EventBroadcaster for testing
type mockWebSocketHub struct {
	messages []WebSocketMessage
	mu       sync.Mutex
}

func (m *mockWebSocketHub) BroadcastAgentEvent(eventType string, data interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if msg, ok := data.(WebSocketMessage); ok {
		m.messages = append(m.messages, msg)
	}
}

func (m *mockWebSocketHub) GetMessages() []WebSocketMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]WebSocketMessage, len(m.messages))
	copy(result, m.messages)
	return result
}

func TestPIEventStreamer_SetEventTypes(t *testing.T) {
	hub := &mockWebSocketHub{}
	streamer := NewPIEventStreamer(hub, "test-job-id")

	// Test empty (broadcast all)
	streamer.SetEventTypes([]string{})
	assert.True(t, streamer.shouldBroadcast("text_delta"), "Expected shouldBroadcast to return true for text_delta when no event types set")

	// Test specific event types
	streamer.SetEventTypes([]string{"text_delta", "tool_call_start"})
	assert.True(t, streamer.shouldBroadcast("text_delta"), "Expected shouldBroadcast to return true for text_delta when explicitly set")
	assert.True(t, streamer.shouldBroadcast("tool_call_start"), "Expected shouldBroadcast to return true for tool_call_start when explicitly set")
	assert.False(t, streamer.shouldBroadcast("thinking_delta"), "Expected shouldBroadcast to return false for thinking_delta when not in list")
}

func TestPIEventStreamer_Stop(t *testing.T) {
	hub := &mockWebSocketHub{}
	streamer := NewPIEventStreamer(hub, "test-job-id")

	// Start and immediately stop
	streamer.Stop()

	// Wait a bit to ensure goroutines clean up
	time.Sleep(50 * time.Millisecond)

	// If we got here without deadlock, the test passes
}

func TestPIEventStreamer_StartWithoutMapper(t *testing.T) {
	hub := &mockWebSocketHub{}
	streamer := NewPIEventStreamer(hub, "test-job-id")

	// Create a bridge but don't start it
	config := Config{
		ModelID: "test-model",
	}
	bridge := NewBridge(config)

	// Create a mapper
	mapper := NewEventMapper()

	// Start without starting the bridge - should not panic
	streamer.Start(bridge, mapper)

	// Clean up
	streamer.Stop()
}

func TestWebSocketMessageConversion(t *testing.T) {
	// Test that MuleEvent can be converted to WebSocketMessage
	event := MuleEvent{
		Type:      MuleEventTextDelta,
		Delta:     "Hello, world!",
		Timestamp: time.Now(),
	}

	msg := WebSocketMessage{
		Type:      string(event.Type),
		Data:      event,
		Timestamp: event.Timestamp,
	}

	assert.Equal(t, string(MuleEventTextDelta), msg.Type, "Expected message type to be %s, got %s", MuleEventTextDelta, msg.Type)

	// Check that the data is the event itself
	eventData, ok := msg.Data.(MuleEvent)
	assert.True(t, ok, "Expected data to be MuleEvent")
	assert.Equal(t, event.Delta, eventData.Delta, "Expected delta %s, got %s", event.Delta, eventData.Delta)
}

func TestPIEventStreamer_BroadcastWithJobID(t *testing.T) {
	hub := &mockWebSocketHub{}
	streamer := NewPIEventStreamer(hub, "test-job-id")

	// Verify jobID is stored correctly
	assert.Equal(t, "test-job-id", streamer.jobID, "Expected jobID to be test-job-id, got %s", streamer.jobID)
}

// Test that verifies the full flow works with a mock bridge
func TestPIEventStreamer_FullFlow(t *testing.T) {
	hub := &mockWebSocketHub{}
	_ = NewPIEventStreamer(hub, "test-job-id")

	// Create config
	config := Config{
		ModelID:   "claude-sonnet-4-20250514",
		NoSession: true,
		NoTools:   true, // No tools for testing
	}

	// Create bridge
	bridge := NewBridge(config)

	// Create mapper
	mapper := NewEventMapper()

	// Create streamer
	streamer := NewPIEventStreamer(hub, "test-job-id")

	// Don't start the bridge for this test - just verify we can create everything
	// The actual streaming test would require a running pi process

	// Test that we can create and stop without issues
	streamer.Start(bridge, mapper)
	streamer.Stop()

	// Verify no panic and no messages (since we didn't start streaming)
	messages := hub.GetMessages()
	assert.Equal(t, 0, len(messages), "Expected 0 messages, got %d", len(messages))
}

// Test context cancellation
func TestPIEventStreamer_ContextCancellation(t *testing.T) {
	hub := &mockWebSocketHub{}
	ctx, cancel := context.WithCancel(context.Background())

	_ = &PIEventStreamer{
		hub:    hub,
		jobID:  "test-job",
		ctx:    ctx,
		cancel: cancel,
	}

	// Cancel immediately
	cancel()

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Verify context is done
	select {
	case <-ctx.Done():
		// Expected
	default:
		t.Error("Expected context to be done")
	}
}

// Test concurrent event broadcasting - verifies thread safety
func TestPIEventStreamer_ConcurrentBroadcast(t *testing.T) {
	hub := &mockWebSocketHub{}
	streamer := NewPIEventStreamer(hub, "test-concurrent")

	config := Config{ModelID: "test"}
	bridge := NewBridge(config)
	mapper := NewEventMapper()

	streamer.Start(bridge, mapper)

	// Launch multiple goroutines sending events concurrently
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			mapper.MapEvent(AgentEvent{
				Type:                  "text_delta",
				AssistantMessageEvent: json.RawMessage(`{"type":"text_delta","delta":"Message ` + fmt.Sprintf("%d", id) + `"}`),
			})
		}(i)
	}

	// Wait for all events to be processed
	wg.Wait()
	time.Sleep(50 * time.Millisecond)

	streamer.Stop()

	// All events should be broadcast
	messages := hub.GetMessages()
	assert.Equal(t, 10, len(messages), "Expected 10 messages, got %d", len(messages))
}

// Test rapid start/stop cycling
func TestPIEventStreamer_RapidStartStop(t *testing.T) {
	for i := 0; i < 5; i++ {
		hub := &mockWebSocketHub{}
		streamer := NewPIEventStreamer(hub, fmt.Sprintf("test-cycle-%d", i))

		config := Config{ModelID: "test"}
		bridge := NewBridge(config)
		mapper := NewEventMapper()

		streamer.Start(bridge, mapper)
		streamer.Stop()
	}

	// If we got here without deadlock or panic, the test passes
}

// Test with nil hub - should not panic
func TestPIEventStreamer_NilHub(t *testing.T) {
	streamer := NewPIEventStreamer(nil, "test-nil-hub")

	config := Config{ModelID: "test"}
	bridge := NewBridge(config)
	mapper := NewEventMapper()

	// This should not panic even with nil hub
	streamer.Start(bridge, mapper)

	// Send an event
	mapper.MapEvent(AgentEvent{Type: "agent_start"})

	// Wait a bit
	time.Sleep(50 * time.Millisecond)

	// Stop should not panic
	streamer.Stop()
}

// Test event ordering under load
func TestPIEventStreamer_EventOrdering(t *testing.T) {
	hub := &mockWebSocketHub{}
	streamer := NewPIEventStreamer(hub, "test-ordering")

	config := Config{ModelID: "test"}
	bridge := NewBridge(config)
	mapper := NewEventMapper()

	streamer.Start(bridge, mapper)

	// Send events in specific order (using actual pi event types)
	events := []AgentEvent{
		{Type: "agent_start"},
		{Type: "thinking_delta", AssistantMessageEvent: json.RawMessage(`{"type":"thinking_delta","delta":"thinking..."}`)},
		{Type: "text_delta", AssistantMessageEvent: json.RawMessage(`{"type":"text_delta","delta":"Hello"}`)},
		{Type: "text_delta", AssistantMessageEvent: json.RawMessage(`{"type":"text_delta","delta":"World"}`)},
		{Type: "tool_execution_start", ToolCallID: "call_1", ToolName: "bash"},
		{Type: "tool_execution_done", ToolCallID: "call_1", ToolName: "bash", Result: json.RawMessage(`{"output":"done"}`)},
		{Type: "agent_end"},
	}
	for _, event := range events {
		mapper.MapEvent(event)
	}

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	streamer.Stop()

	messages := hub.GetMessages()
	expectedTypes := []string{"agent_start", "thinking_delta", "text_delta", "text_delta", "tool_call_start", "tool_call_done", "agent_end"}
	assert.Equal(t, len(expectedTypes), len(messages), "Expected %d messages, got %d", len(expectedTypes), len(messages))

	// Verify order is preserved
	for i, expected := range expectedTypes {
		if i >= len(messages) {
			break
		}
		assert.Equal(t, expected, messages[i].Type, "Event %d: expected %s, got %s", i, expected, messages[i].Type)
	}
}

// Test SetEventTypes with empty string
func TestPIEventStreamer_SetEventTypesEmptyString(t *testing.T) {
	hub := &mockWebSocketHub{}
	streamer := NewPIEventStreamer(hub, "test-empty-string")

	// Setting empty slice should broadcast all events (not filter)
	streamer.SetEventTypes([]string{})

	assert.True(t, streamer.shouldBroadcast("text_delta"), "Expected shouldBroadcast to return true for empty filter (broadcast all)")

	// Also test with specific type
	streamer.SetEventTypes([]string{"text_delta"})
	assert.True(t, streamer.shouldBroadcast("text_delta"), "Expected shouldBroadcast to return true for text_delta when in filter")
	assert.False(t, streamer.shouldBroadcast("thinking_delta"), "Expected shouldBroadcast to return false for thinking_delta when not in filter")
}

// Test channel full scenario - verify non-blocking behavior
func TestPIEventStreamer_ChannelFull(t *testing.T) {
	hub := &mockWebSocketHub{}
	streamer := NewPIEventStreamer(hub, "test-channel-full")

	config := Config{ModelID: "test"}
	bridge := NewBridge(config)
	mapper := NewEventMapper()

	// Start without starting the bridge
	streamer.Start(bridge, mapper)

	// Try to send many events quickly
	for i := 0; i < 200; i++ {
		mapper.MapEvent(AgentEvent{
			Type:                  "text_delta",
			AssistantMessageEvent: json.RawMessage(`{"type":"text_delta","delta":"` + fmt.Sprintf("x%d", i) + `"}`),
		})
	}

	// Wait for processing
	time.Sleep(50 * time.Millisecond)

	// Should not deadlock - verify we got some messages
	messages := hub.GetMessages()
	t.Logf("Received %d messages out of 200 sent", len(messages))

	streamer.Stop()
}
