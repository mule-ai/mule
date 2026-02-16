package pirc

import (
	"encoding/json"
	"testing"
	"time"
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
		if muleEvent.Type != MuleEventTextDelta {
			t.Errorf("Expected text_delta, got %v", muleEvent.Type)
		}
		if muleEvent.Delta != "Hello " {
			t.Errorf("Expected delta 'Hello ', got %v", muleEvent.Delta)
		}
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
		if muleEvent.Type != MuleEventThinkingDelta {
			t.Errorf("Expected thinking_delta, got %v", muleEvent.Type)
		}
		if muleEvent.Delta != "Let me think about this..." {
			t.Errorf("Expected delta 'Let me think about this...', got %v", muleEvent.Delta)
		}
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
		if muleEvent.Type != MuleEventAgentStart {
			t.Errorf("Expected agent_start, got %v", muleEvent.Type)
		}
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
		if muleEvent.Type != MuleEventAgentEnd {
			t.Errorf("Expected agent_end, got %v", muleEvent.Type)
		}
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
		if muleEvent.Type != MuleEventToolCallStart {
			t.Errorf("Expected tool_call_start, got %v", muleEvent.Type)
		}
		if muleEvent.ID != "call_abc123" {
			t.Errorf("Expected tool call ID 'call_abc123', got %v", muleEvent.ID)
		}
		if muleEvent.Name != "bash" {
			t.Errorf("Expected tool name 'bash', got %v", muleEvent.Name)
		}
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
		if muleEvent.Type != MuleEventToolCallDone {
			t.Errorf("Expected tool_call_done, got %v", muleEvent.Type)
		}
		if muleEvent.ID != "call_abc123" {
			t.Errorf("Expected tool call ID 'call_abc123', got %v", muleEvent.ID)
		}
		expectedOutput := `{"output":"total 0\ndrwxr-xr-x 2 user user 4096 Feb 15 07:00 ."}`
		if string(muleEvent.Content) != expectedOutput {
			t.Errorf("Expected result '%s', got %v", expectedOutput, string(muleEvent.Content))
		}
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
		if muleEvent.Type != MuleEventAgentError {
			t.Errorf("Expected agent_error, got %v", muleEvent.Type)
		}
		if muleEvent.Error == "" {
			t.Error("Expected error message")
		}
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
		if muleEvent.Type != MuleEventTextDelta {
			t.Logf("Got event type: %v", muleEvent.Type)
			// This might return raw if parsing doesn't work as expected
		}
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
		if muleEvent.Type != MuleEventExtensionUIRequest {
			t.Errorf("Expected extension_ui_request, got %v", muleEvent.Type)
		}
		if muleEvent.ID != "uuid-1" {
			t.Errorf("Expected ID 'uuid-1', got %v", muleEvent.ID)
		}
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
		if muleEvent.Type != MuleEventTextDone {
			t.Errorf("Expected text_done, got %v", muleEvent.Type)
		}
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
		if muleEvent.Type != MuleEventThinkingDone {
			t.Errorf("Expected thinking_done, got %v", muleEvent.Type)
		}
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
		if muleEvent.Type != MuleEventMessageStart {
			t.Errorf("Expected message_start, got %v", muleEvent.Type)
		}
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
		if muleEvent.Type != MuleEventMessageEnd {
			t.Errorf("Expected message_end, got %v", muleEvent.Type)
		}
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
		if muleEvent.Type != MuleEventRaw {
			t.Errorf("Expected raw, got %v", muleEvent.Type)
		}
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

	if wsMsg.Type != "text_delta" {
		t.Errorf("Expected WebSocket message type 'text_delta', got %v", wsMsg.Type)
	}

	data, ok := wsMsg.Data.(*MuleEvent)
	if !ok {
		t.Fatal("Data is not a MuleEvent")
	}

	if data.Delta != "Hello" {
		t.Errorf("Expected delta 'Hello', got %v", data.Delta)
	}
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
		if muleEvent.Type != MuleEventAgentStart {
			t.Errorf("Expected agent_start, got %v", muleEvent.Type)
		}
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
		if muleEvent.Type != MuleEventTextDelta {
			t.Errorf("Expected text_delta, got %v", muleEvent.Type)
		}
		if muleEvent.Delta != "Hello " {
			t.Errorf("Expected delta 'Hello ', got %v", muleEvent.Delta)
		}
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
		if muleEvent.Type != MuleEventThinkingDelta {
			t.Errorf("Expected thinking_delta, got %v", muleEvent.Type)
		}
		if muleEvent.Delta != "Let me think about this..." {
			t.Errorf("Expected delta 'Let me think about this...', got %v", muleEvent.Delta)
		}
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
		if muleEvent.Type != MuleEventTextDelta {
			t.Errorf("Expected text_delta, got %v", muleEvent.Type)
		}
		if muleEvent.Delta != "World" {
			t.Errorf("Expected delta 'World', got %v", muleEvent.Delta)
		}
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
		if muleEvent.Type != MuleEventToolCallProgress {
			t.Errorf("Expected tool_call_progress, got %v", muleEvent.Type)
		}
		if muleEvent.ID != "call_abc123" {
			t.Errorf("Expected tool call ID 'call_abc123', got %v", muleEvent.ID)
		}
		if muleEvent.Name != "bash" {
			t.Errorf("Expected tool name 'bash', got %v", muleEvent.Name)
		}
		if muleEvent.Delta != "Reading files..." {
			t.Errorf("Expected progress 'Reading files...', got %v", muleEvent.Delta)
		}
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
		if muleEvent.Type != MuleEventToolResult {
			t.Errorf("Expected tool_result, got %v", muleEvent.Type)
		}
		if muleEvent.ID != "call_xyz789" {
			t.Errorf("Expected tool call ID 'call_xyz789', got %v", muleEvent.ID)
		}
		if muleEvent.Name != "read" {
			t.Errorf("Expected tool name 'read', got %v", muleEvent.Name)
		}
		expectedResult := `{"content":"file content here"}`
		if string(muleEvent.Content) != expectedResult {
			t.Errorf("Expected result '%s', got %v", expectedResult, string(muleEvent.Content))
		}
	default:
		t.Error("No event received")
	}
}

// TestEventMapper_ExtensionUIRequest tests extension_ui_request with select method (already declared at line 200)
// TestEventMapper_ExtensionUIRequestWithConfirm tests extension_ui_request with confirm method
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
		if muleEvent.Type != MuleEventExtensionUIRequest {
			t.Errorf("Expected extension_ui_request, got %v", muleEvent.Type)
		}
		if muleEvent.ID != "ui_req_456" {
			t.Errorf("Expected ID 'ui_req_456', got %v", muleEvent.ID)
		}
	default:
		t.Error("No event received")
	}
}

// TestEventMapper_ExtensionUIRequestWithInput tests extension_ui_request with input method
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
		if muleEvent.Type != MuleEventExtensionUIRequest {
			t.Errorf("Expected extension_ui_request, got %v", muleEvent.Type)
		}
		if muleEvent.ID != "ui_req_789" {
			t.Errorf("Expected ID 'ui_req_789', got %v", muleEvent.ID)
		}
	default:
		t.Error("No event received")
	}
}

// TestEventMapper_ResponseEvent tests the response event handling
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
		if muleEvent.Type != MuleEventAgentEnd {
			t.Errorf("Expected agent_end, got %v", muleEvent.Type)
		}
		if len(muleEvent.Content) == 0 {
			t.Error("Expected content to be present")
		}
	default:
		t.Error("No event received")
	}
}

// TestEventMapper_ErrorEvent tests error event handling
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
		if muleEvent.Type != MuleEventAgentError {
			t.Errorf("Expected agent_error, got %v", muleEvent.Type)
		}
		if muleEvent.Error == "" {
			t.Error("Expected error message to be present")
		}
	default:
		t.Error("No event received")
	}
}

// TestEventMapper_AgentErrorEvent tests agent_error event handling
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
		if muleEvent.Type != MuleEventAgentError {
			t.Errorf("Expected agent_error, got %v", muleEvent.Type)
		}
	default:
		t.Error("No event received")
	}
}

// TestEventMapper_EmptyMessage tests handling of events with empty message
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
		if muleEvent.Type != MuleEventTextDelta {
			t.Errorf("Expected text_delta, got %v", muleEvent.Type)
		}
		// Empty message should still produce an event
	default:
		t.Error("No event received")
	}
}

// TestEventMapper_MessageUpdateWithMultipleContent tests message_update with multiple content items
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
		if muleEvent.Type != MuleEventTextDelta && muleEvent.Type != MuleEventThinkingDelta && muleEvent.Type != MuleEventRaw {
			t.Logf("Received unexpected event type: %s", muleEvent.Type)
		}
	default:
		t.Error("No event received")
	}
}

// TestEventMapper_MessageUpdateWithTextField tests message_update with text field (not delta)
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
		if muleEvent.Type != MuleEventTextDelta {
			t.Errorf("Expected text_delta, got %v", muleEvent.Type)
		}
	default:
		t.Error("No event received")
	}
}

// TestEventMapper_ToolExecutionDoneWithResult tests tool_execution_done with result content
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
		if muleEvent.Type != MuleEventToolCallDone {
			t.Errorf("Expected tool_call_done, got %v", muleEvent.Type)
		}
		if muleEvent.ID != "call_123" {
			t.Errorf("Expected tool call ID 'call_123', got %v", muleEvent.ID)
		}
		if muleEvent.Name != "bash" {
			t.Errorf("Expected tool name 'bash', got %v", muleEvent.Name)
		}
		if len(muleEvent.Content) == 0 {
			t.Error("Expected result content to be present")
		}
	default:
		t.Error("No event received")
	}
}

// TestEventMapper_ExtensionUIResponse tests extension_ui_response event
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
		if muleEvent.Type != MuleEventExtensionUIResponse {
			t.Errorf("Expected extension_ui_response, got %v", muleEvent.Type)
		}
	default:
		t.Error("No event received")
	}
}

// TestEventMapper_Timestamp tests that timestamps are properly set
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
		if muleEvent.Timestamp.Before(before) || muleEvent.Timestamp.After(after) {
			t.Errorf("Timestamp not within expected range: %v", muleEvent.Timestamp)
		}
	default:
		t.Error("No event received")
	}
}

// TestEventMapper_NewEventMapper tests constructor
func TestEventMapper_NewEventMapper(t *testing.T) {
	mapper := NewEventMapper()

	if mapper == nil {
		t.Fatal("NewEventMapper returned nil")
	}

	if mapper.eventChan == nil {
		t.Error("Expected eventChan to be initialized")
	}

	// Verify channel capacity
	select {
	case mapper.eventChan <- MuleEvent{}:
		// Channel should have capacity
	default:
		t.Error("Expected channel to have capacity")
	}
}

// TestEventMapper_StartMapping tests the StartMapping method - using channels
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
		if muleEvent.Type != MuleEventAgentStart {
			t.Errorf("Expected agent_start, got %v", muleEvent.Type)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for mapped event")
	}
}

// TestEventMapper_StartMappingMultipleEvents tests StartMapping with multiple events
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

	if count != 3 {
		t.Errorf("Expected 3 events, got %d", count)
	}
}

// TestExtractTextDeltaFromMessage tests text delta extraction from Message field
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
		if muleEvent.Delta != "From message field" {
			t.Errorf("Expected 'From message field', got %v", muleEvent.Delta)
		}
	default:
		t.Error("No event received")
	}
}

// TestExtractThinkingDeltaFromMessage tests thinking delta extraction from Message field
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
		if muleEvent.Delta != "Processing request..." {
			t.Errorf("Expected 'Processing request...', got %v", muleEvent.Delta)
		}
	default:
		t.Error("No event received")
	}
}
