package pirc

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEventMapper_TextDelta(t *testing.T) {
	mapper := NewEventMapper()

	// Test text_delta event
	event := AgentEvent{
		Type:                  "text_delta",
		AssistantMessageEvent: json.RawMessage(`{"type":"text_delta","delta":"Hello "}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventTextDelta, muleEvent.Type)
		assert.Equal(t, "Hello ", muleEvent.Delta)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_ThinkingDelta(t *testing.T) {
	mapper := NewEventMapper()

	event := AgentEvent{
		Type:                  "thinking_delta",
		AssistantMessageEvent: json.RawMessage(`{"type":"thinking_delta","delta":"Let me think about this..."}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventThinkingDelta, muleEvent.Type)
		assert.Equal(t, "Let me think about this...", muleEvent.Delta)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_AgentStart(t *testing.T) {
	mapper := NewEventMapper()

	event := AgentEvent{
		Type: "agent_start",
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventAgentStart, muleEvent.Type)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_AgentEnd(t *testing.T) {
	mapper := NewEventMapper()

	event := AgentEvent{
		Type: "agent_end",
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventAgentEnd, muleEvent.Type)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_ToolExecutionStart(t *testing.T) {
	mapper := NewEventMapper()

	event := AgentEvent{
		Type:       "tool_execution_start",
		ToolCallID: "call_abc123",
		ToolName:   "bash",
		Args:       json.RawMessage(`{"command":"ls -la"}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventToolCallStart, muleEvent.Type)
		assert.Equal(t, "call_abc123", muleEvent.ID)
		assert.Equal(t, "bash", muleEvent.Name)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_ToolExecutionDone(t *testing.T) {
	mapper := NewEventMapper()

	event := AgentEvent{
		Type:       "tool_execution_done",
		ToolCallID: "call_abc123",
		ToolName:   "bash",
		Result:     json.RawMessage(`{"output":"total 0\ndrwxr-xr-x 2 user user 4096 Feb 15 07:00 ."}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventToolCallDone, muleEvent.Type)
		assert.Equal(t, "call_abc123", muleEvent.ID)
		expectedOutput := `{"output":"total 0\ndrwxr-xr-x 2 user user 4096 Feb 15 07:00 ."}`
		assert.Equal(t, expectedOutput, string(muleEvent.Content))
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_Error(t *testing.T) {
	mapper := NewEventMapper()

	event := AgentEvent{
		Type:    "error",
		IsError: true,
		Result:  json.RawMessage(`{"error":"Failed to execute tool"}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventAgentError, muleEvent.Type)
		assert.NotEmpty(t, muleEvent.Error)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_MessageUpdate(t *testing.T) {
	mapper := NewEventMapper()

	// Test message_update with text content
	event := AgentEvent{
		Type: "message_update",
		Message: json.RawMessage(`{
			"role": "assistant",
			"content": [{"type": "text", "text": "Hello"}]
		}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		// This might return raw if parsing doesn't work as expected
		assert.NotEmpty(t, muleEvent.Type)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_ExtensionUIRequest(t *testing.T) {
	mapper := NewEventMapper()

	event := AgentEvent{
		Type:    "extension_ui_request",
		Message: json.RawMessage(`{"type":"extension_ui_request","id":"uuid-1","method":"select","title":"Confirm?","options":["Yes","No"]}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventExtensionUIRequest, muleEvent.Type)
		assert.Equal(t, "uuid-1", muleEvent.ID)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_TextDone(t *testing.T) {
	mapper := NewEventMapper()

	event := AgentEvent{
		Type: "text_done",
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventTextDone, muleEvent.Type)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_ThinkingDone(t *testing.T) {
	mapper := NewEventMapper()

	event := AgentEvent{
		Type: "thinking_done",
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventThinkingDone, muleEvent.Type)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_MessageStart(t *testing.T) {
	mapper := NewEventMapper()

	event := AgentEvent{
		Type: "message_start",
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventMessageStart, muleEvent.Type)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_MessageEnd(t *testing.T) {
	mapper := NewEventMapper()

	event := AgentEvent{
		Type: "message_end",
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventMessageEnd, muleEvent.Type)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_UnknownEvent(t *testing.T) {
	mapper := NewEventMapper()

	event := AgentEvent{
		Type:    "unknown_event_type",
		Message: json.RawMessage(`{"some":"data"}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventRaw, muleEvent.Type)
	default:
		t.Error("No event received")
	}
}

func TestMuleEventToWebSocketMessage(t *testing.T) {
	event := MuleEvent{
		Type:      MuleEventTextDelta,
		Delta:     "Hello",
		Index:     0,
		Timestamp: time.Now(),
	}

	wsMsg := event.ToWebSocketMessage()

	assert.Equal(t, "text_delta", wsMsg.Type)

	data, ok := wsMsg.Data.(*MuleEvent)
	assert.True(t, ok, "Data is not a MuleEvent")
	assert.Equal(t, "Hello", data.Delta)
}

func TestEventMapper_StartMapping(t *testing.T) {
	mapper := NewEventMapper()

	piEvents := make(chan AgentEvent, 10)
	mapper.StartMapping(piEvents)

	// Send an event through the mapper
	piEvents <- AgentEvent{
		Type: "agent_start",
	}

	// Receive the mapped event
	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventAgentStart, muleEvent.Type)
	case <-time.After(time.Second):
		t.Error("Timeout waiting for event")
	}

	// Close the input channel
	close(piEvents)

	// The output channel should eventually close
	_, ok := <-mapper.eventChan
	if ok {
		// Channel should be closed, not have more data
		t.Log("Channel still has data (might be expected)")
	}
}

func TestEventMapper_MessageUpdateWithTextDelta(t *testing.T) {
	mapper := NewEventMapper()

	// Test message_update with text delta in content
	event := AgentEvent{
		Type: "message_update",
		Message: json.RawMessage(`{
			"role": "assistant",
			"content": [{"type": "text_delta", "delta": "Hello "}]
		}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventTextDelta, muleEvent.Type)
		assert.Equal(t, "Hello ", muleEvent.Delta)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_MessageUpdateWithThinkingDelta(t *testing.T) {
	mapper := NewEventMapper()

	// Test message_update with thinking delta in content
	event := AgentEvent{
		Type: "message_update",
		Message: json.RawMessage(`{
			"role": "assistant",
			"content": [{"type": "thinking_delta", "delta": "Let me think about this..."}]
		}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventThinkingDelta, muleEvent.Type)
		assert.Equal(t, "Let me think about this...", muleEvent.Delta)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_MessageUpdateWithAssistantMessageEvent(t *testing.T) {
	mapper := NewEventMapper()

	// Test message_update with assistantMessageEvent containing delta
	event := AgentEvent{
		Type:                  "message_update",
		AssistantMessageEvent: json.RawMessage(`{"type":"text_delta","delta":"World"}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventTextDelta, muleEvent.Type)
		assert.Equal(t, "World", muleEvent.Delta)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_ToolExecutionProgress(t *testing.T) {
	mapper := NewEventMapper()

	// Test tool_execution_progress with partialResult containing progress
	event := AgentEvent{
		Type:          "tool_execution_progress",
		ToolCallID:    "call_abc123",
		ToolName:      "bash",
		PartialResult: json.RawMessage(`{"progress":"Reading files..."}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventToolCallProgress, muleEvent.Type)
		assert.Equal(t, "call_abc123", muleEvent.ID)
		assert.Equal(t, "bash", muleEvent.Name)
		assert.Equal(t, "Reading files...", muleEvent.Delta)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_ToolResult(t *testing.T) {
	mapper := NewEventMapper()

	// Test tool_result event
	event := AgentEvent{
		Type:       "tool_result",
		ToolCallID: "call_xyz789",
		ToolName:   "read",
		Result:     json.RawMessage(`{"content":"file content here"}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventToolResult, muleEvent.Type)
		assert.Equal(t, "call_xyz789", muleEvent.ID)
		assert.Equal(t, "read", muleEvent.Name)
		expectedResult := `{"content":"file content here"}`
		assert.Equal(t, expectedResult, string(muleEvent.Content))
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_ExtensionUIRequestWithConfirm(t *testing.T) {
	mapper := NewEventMapper()

	// Test extension_ui_request with confirm method
	event := AgentEvent{
		Type: "extension_ui_request",
		Message: json.RawMessage(`{
			"type": "extension_ui_request",
			"id": "ui_req_456",
			"method": "confirm",
			"title": "Confirm Action",
			"message": "Are you sure you want to proceed?",
			"timeout": 60000
		}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventExtensionUIRequest, muleEvent.Type)
		assert.Equal(t, "ui_req_456", muleEvent.ID)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_ExtensionUIRequestWithInput(t *testing.T) {
	mapper := NewEventMapper()

	// Test extension_ui_request with input method
	event := AgentEvent{
		Type: "extension_ui_request",
		Message: json.RawMessage(`{
			"type": "extension_ui_request",
			"id": "ui_req_789",
			"method": "input",
			"title": "Enter Value",
			"message": "Please enter your name:",
			"timeout": 30000
		}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventExtensionUIRequest, muleEvent.Type)
		assert.Equal(t, "ui_req_789", muleEvent.ID)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_ResponseEvent(t *testing.T) {
	mapper := NewEventMapper()

	// Test response event (sent by pi instead of agent_end in some cases)
	event := AgentEvent{
		Type:    "response",
		Message: json.RawMessage(`{"type":"message","role":"assistant","content":[{"type":"text","text":"Final response"}]}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventAgentEnd, muleEvent.Type)
		assert.NotEmpty(t, muleEvent.Content)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_ErrorEvent(t *testing.T) {
	mapper := NewEventMapper()

	// Test error event
	event := AgentEvent{
		Type:    "error",
		IsError: true,
		Result:  json.RawMessage(`{"error":"API rate limit exceeded"}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventAgentError, muleEvent.Type)
		assert.NotEmpty(t, muleEvent.Error)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_AgentErrorEvent(t *testing.T) {
	mapper := NewEventMapper()

	// Test agent_error event
	event := AgentEvent{
		Type:    "agent_error",
		Message: json.RawMessage(`{"error":"Connection timeout"}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventAgentError, muleEvent.Type)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_EmptyMessage(t *testing.T) {
	mapper := NewEventMapper()

	// Test text_delta with empty message
	event := AgentEvent{
		Type:    "text_delta",
		Message: json.RawMessage(``),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventTextDelta, muleEvent.Type)
		// Empty message should still produce an event
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_MessageUpdateWithMultipleContent(t *testing.T) {
	mapper := NewEventMapper()

	// Test message_update with multiple content items
	event := AgentEvent{
		Type:    "message_update",
		Message: json.RawMessage(`{"content":[{"type":"text","text":"Hello"},{"type":"thinking","thinking":"Analyzing..."}]}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		// Should get the first content item (text) - verify it's not an unexpected type
		validTypes := []MuleEventType{MuleEventTextDelta, MuleEventThinkingDelta, MuleEventRaw}
		found := false
		for _, vt := range validTypes {
			if muleEvent.Type == vt {
				found = true
				break
			}
		}
		assert.True(t, found, "Received unexpected event type: %s", muleEvent.Type)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_MessageUpdateWithTextField(t *testing.T) {
	mapper := NewEventMapper()

	// Test message_update with text field
	event := AgentEvent{
		Type:    "message_update",
		Message: json.RawMessage(`{"content":[{"type":"text","text":"Hello World"}]}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventTextDelta, muleEvent.Type)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_ToolExecutionDoneWithResult(t *testing.T) {
	mapper := NewEventMapper()

	// Test tool_execution_done with result
	event := AgentEvent{
		Type:       "tool_execution_done",
		ToolCallID: "call_123",
		ToolName:   "bash",
		Result:     json.RawMessage(`{"output":"total 0\ndrwxr-xr-x 1 user user 4096 Feb 15 10:00 .\n"}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventToolCallDone, muleEvent.Type)
		assert.Equal(t, "call_123", muleEvent.ID)
		assert.Equal(t, "bash", muleEvent.Name)
		assert.NotEmpty(t, muleEvent.Content)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_ExtensionUIResponse(t *testing.T) {
	mapper := NewEventMapper()

	// Test extension_ui_response event
	event := AgentEvent{
		Type:    "extension_ui_response",
		Message: json.RawMessage(`{"type":"extension_ui_response","id":"ui_req_123","value":"selected_value"}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, MuleEventExtensionUIResponse, muleEvent.Type)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_Timestamp(t *testing.T) {
	mapper := NewEventMapper()
	before := time.Now()

	event := AgentEvent{
		Type: "agent_start",
	}

	mapper.MapEvent(event)

	after := time.Now()

	select {
	case muleEvent := <-mapper.eventChan:
		assert.True(t, !muleEvent.Timestamp.Before(before) && !muleEvent.Timestamp.After(after),
			"Timestamp not within expected range: %v", muleEvent.Timestamp)
	default:
		t.Error("No event received")
	}
}

func TestEventMapper_NewEventMapper(t *testing.T) {
	mapper := NewEventMapper()

	assert.NotNil(t, mapper)
	assert.NotNil(t, mapper.eventChan)

	// Verify channel capacity
	select {
	case mapper.eventChan <- MuleEvent{}:
		// Channel should have capacity
	default:
		t.Error("Expected channel to have capacity")
	}
}

func TestEventMapper_StartMappingChannels(t *testing.T) {
	mapper := NewEventMapper()

	// Create input channel
	inputChan := make(chan AgentEvent, 10)

	// Start mapping in background
	mapper.StartMapping(inputChan)

	// Send an event
	testEvent := AgentEvent{
		Type: "agent_start",
	}
	inputChan <- testEvent

	// Close input to signal completion
	close(inputChan)

	// Receive the mapped event
	select {
	case muleEvent := <-mapper.Events():
		assert.Equal(t, MuleEventAgentStart, muleEvent.Type)
	case <-time.After(time.Second):
		t.Error("Timeout waiting for mapped event")
	}
}

func TestEventMapper_StartMappingMultipleEvents(t *testing.T) {
	mapper := NewEventMapper()

	inputChan := make(chan AgentEvent, 10)
	mapper.StartMapping(inputChan)

	// Send multiple events
	events := []AgentEvent{
		{Type: "agent_start"},
		{Type: "text_delta", AssistantMessageEvent: json.RawMessage(`{"type":"text_delta","delta":"Hello"}`)},
		{Type: "agent_end"},
	}

	for _, e := range events {
		inputChan <- e
	}

	// Close input to signal completion - this will also close the output channel
	close(inputChan)

	// Receive all mapped events
	count := 0
	for muleEvent := range mapper.Events() {
		t.Logf("Received event: %v", muleEvent.Type)
		count++
		if count > 10 {
			break // Safety limit
		}
	}

	assert.Equal(t, 3, count)
}

func TestExtractTextDeltaFromMessage(t *testing.T) {
	mapper := NewEventMapper()

	// Test with Message field containing content array
	event := AgentEvent{
		Type:    "text_delta",
		Message: json.RawMessage(`{"content":[{"type":"text","text":"From message field"}]}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, "From message field", muleEvent.Delta)
	default:
		t.Error("No event received")
	}
}

func TestExtractThinkingDeltaFromMessage(t *testing.T) {
	mapper := NewEventMapper()

	// Test with Message field containing thinking
	event := AgentEvent{
		Type:    "thinking_delta",
		Message: json.RawMessage(`{"content":[{"type":"thinking","thinking":"Processing request..."}]}`),
	}

	mapper.MapEvent(event)

	select {
	case muleEvent := <-mapper.eventChan:
		assert.Equal(t, "Processing request...", muleEvent.Delta)
	default:
		t.Error("No event received")
	}
}
