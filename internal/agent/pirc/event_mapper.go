package pirc

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/mule-ai/mule/internal/api"
)

// MuleEventType represents the type of event sent to Mule clients
type MuleEventType string

const (
	// Text events
	MuleEventTextDelta     MuleEventType = "text_delta"
	MuleEventTextDone      MuleEventType = "text_done"
	MuleEventThinkingDelta MuleEventType = "thinking_delta"
	MuleEventThinkingDone  MuleEventType = "thinking_done"

	// Tool events
	MuleEventToolCallStart    MuleEventType = "tool_call_start"
	MuleEventToolCallProgress MuleEventType = "tool_call_progress"
	MuleEventToolCallDone     MuleEventType = "tool_call_done"
	MuleEventToolResult       MuleEventType = "tool_result"

	// Agent lifecycle events
	MuleEventAgentStart MuleEventType = "agent_start"
	MuleEventAgentEnd   MuleEventType = "agent_end"
	MuleEventAgentError MuleEventType = "agent_error"

	// Message events
	MuleEventMessageStart MuleEventType = "message_start"
	MuleEventMessageEnd   MuleEventType = "message_end"

	// Extension UI events
	MuleEventExtensionUIRequest  MuleEventType = "extension_ui_request"
	MuleEventExtensionUIResponse MuleEventType = "extension_ui_response"

	// Generic events
	MuleEventRaw       MuleEventType = "raw"
	MuleEventConnected MuleEventType = "connected"
)

// TextContent represents text content in a message
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// ThinkingContent represents thinking content
type ThinkingContent struct {
	Type     string `json:"type"`
	Thinking string `json:"thinking"`
	TraceID  string `json:"traceId,omitempty"`
}

// ToolCallContent represents a tool call
type ToolCallContent struct {
	Type     string          `json:"type"`
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Input    json.RawMessage `json:"input,omitempty"`
	Progress string          `json:"progress,omitempty"`
	Output   string          `json:"output,omitempty"`
	Error    string          `json:"error,omitempty"`
}

// MuleEvent represents an event to be sent to Mule clients
type MuleEvent struct {
	Type      MuleEventType   `json:"type"`
	Content   json.RawMessage `json:"content,omitempty"`
	Delta     string          `json:"delta,omitempty"`
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name,omitempty"`
	Index     int             `json:"index,omitempty"`
	Error     string          `json:"error,omitempty"`
	Timestamp time.Time       `json:"timestamp"`
}

// ToWebSocketMessage converts a MuleEvent to a WebSocket message
func (e *MuleEvent) ToWebSocketMessage() api.WebSocketMessage {
	return api.WebSocketMessage{
		Type:      string(e.Type),
		Data:      e,
		Timestamp: e.Timestamp,
	}
}

// EventMapper maps pi events to Mule events
type EventMapper struct {
	eventChan chan MuleEvent
}

// NewEventMapper creates a new event mapper
func NewEventMapper() *EventMapper {
	return &EventMapper{
		eventChan: make(chan MuleEvent, 1000),
	}
}

// Events returns the channel of Mule events
func (m *EventMapper) Events() <-chan MuleEvent {
	return m.eventChan
}

// MapEvent converts a pi AgentEvent to a MuleEvent
func (m *EventMapper) MapEvent(event AgentEvent) {
	muleEvent := MuleEvent{
		Timestamp: time.Now(),
	}

	switch event.Type {
	case "agent_start":
		muleEvent.Type = MuleEventAgentStart

	case "agent_end":
		muleEvent.Type = MuleEventAgentEnd

	case "response":
		// Response event - this indicates the agent has completed
		// Extract content from the response
		muleEvent.Type = MuleEventAgentEnd
		// Try to extract text content from the response
		if len(event.Message) > 0 {
			muleEvent.Content = event.Message
		}

	case "error", "agent_error":
		muleEvent.Type = MuleEventAgentError
		if event.IsError {
			muleEvent.Error = extractErrorMessage(event)
		} else {
			muleEvent.Error = extractErrorMessage(event)
		}

	case "message_start":
		muleEvent.Type = MuleEventMessageStart

	case "message_end":
		muleEvent.Type = MuleEventMessageEnd

	case "message_update":
		muleEvent.Type, muleEvent.Delta = m.handleMessageUpdate(event)

	case "text_delta":
		muleEvent.Type = MuleEventTextDelta
		muleEvent.Delta = extractTextDelta(event)

	case "text_done":
		muleEvent.Type = MuleEventTextDone

	case "thinking_delta":
		muleEvent.Type = MuleEventThinkingDelta
		muleEvent.Delta = extractThinkingDelta(event)

	case "thinking_done":
		muleEvent.Type = MuleEventThinkingDone

	case "tool_execution_start":
		muleEvent.Type = MuleEventToolCallStart
		muleEvent.ID = event.ToolCallID
		muleEvent.Name = event.ToolName
		muleEvent.Content = event.Args

	case "tool_execution_progress":
		muleEvent.Type = MuleEventToolCallProgress
		muleEvent.ID = event.ToolCallID
		muleEvent.Name = event.ToolName
		muleEvent.Delta = extractProgress(event)

	case "tool_execution_done":
		muleEvent.Type = MuleEventToolCallDone
		muleEvent.ID = event.ToolCallID
		muleEvent.Name = event.ToolName
		muleEvent.Content = event.Result

	case "tool_result":
		muleEvent.Type = MuleEventToolResult
		muleEvent.ID = event.ToolCallID
		muleEvent.Name = event.ToolName
		muleEvent.Content = event.Result

	case "extension_ui_request":
		muleEvent.Type = MuleEventExtensionUIRequest
		uiReq := ExtensionUIRequest{}
		if err := json.Unmarshal(event.Message, &uiReq); err == nil {
			muleEvent.ID = uiReq.ID
			muleEvent.Content = event.Message
		} else {
			muleEvent.Content = event.Message
		}

	case "extension_ui_response":
		muleEvent.Type = MuleEventExtensionUIResponse

	default:
		// For unknown event types, send as raw
		muleEvent.Type = MuleEventRaw
		muleEvent.Content = event.Message
	}

	// Send the mapped event
	select {
	case m.eventChan <- muleEvent:
	default:
		fmt.Printf("Event mapper: channel full, dropping event: %s\n", event.Type)
	}
}

// handleMessageUpdate handles the complex message_update event
// It returns the MuleEventType and any delta content
func (m *EventMapper) handleMessageUpdate(event AgentEvent) (MuleEventType, string) {
	// Try to parse the assistant message event to determine the type
	if len(event.AssistantMessageEvent) > 0 {
		var assistantMsg struct {
			Type  string `json:"type"`
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal(event.AssistantMessageEvent, &assistantMsg); err == nil {
			switch assistantMsg.Type {
			case "text_delta":
				return MuleEventTextDelta, assistantMsg.Delta
			case "thinking_delta":
				return MuleEventThinkingDelta, assistantMsg.Delta
			}
		}
	}

	// Try to parse message for content deltas
	if len(event.Message) > 0 {
		var msg struct {
			Content []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				Thinking string `json:"thinking"`
				Delta    string `json:"delta"`
			} `json:"content"`
		}
		if err := json.Unmarshal(event.Message, &msg); err == nil && len(msg.Content) > 0 {
			switch msg.Content[0].Type {
			case "text":
				// Try delta first, then text
				if msg.Content[0].Delta != "" {
					return MuleEventTextDelta, msg.Content[0].Delta
				}
				return MuleEventTextDelta, msg.Content[0].Text
			case "thinking":
				if msg.Content[0].Thinking != "" {
					return MuleEventThinkingDelta, msg.Content[0].Thinking
				}
				if msg.Content[0].Delta != "" {
					return MuleEventThinkingDelta, msg.Content[0].Delta
				}
			case "text_delta":
				return MuleEventTextDelta, msg.Content[0].Delta
			case "thinking_delta":
				return MuleEventThinkingDelta, msg.Content[0].Delta
			}
		}
	}

	// Try partialResult as fallback
	if len(event.PartialResult) > 0 {
		var result struct {
			Type     string `json:"type"`
			Delta    string `json:"delta"`
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
		}
		if err := json.Unmarshal(event.PartialResult, &result); err == nil {
			switch result.Type {
			case "text_delta":
				return MuleEventTextDelta, result.Delta
			case "thinking_delta":
				return MuleEventThinkingDelta, result.Delta
			}
			if result.Text != "" {
				return MuleEventTextDelta, result.Text
			}
			if result.Thinking != "" {
				return MuleEventThinkingDelta, result.Thinking
			}
		}
	}

	return MuleEventRaw, ""
}

// StartMapping starts the event mapping loop
func (m *EventMapper) StartMapping(piEvents <-chan AgentEvent) {
	go func() {
		for event := range piEvents {
			m.MapEvent(event)
		}
		// Close the output channel when input is closed
		close(m.eventChan)
	}()
}

// extractTextDelta extracts text delta from an event
func extractTextDelta(event AgentEvent) string {
	// Try assistantMessageEvent first
	if len(event.AssistantMessageEvent) > 0 {
		var deltaMsg struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal(event.AssistantMessageEvent, &deltaMsg); err == nil && deltaMsg.Delta != "" {
			return deltaMsg.Delta
		}
	}

	// Try Message field
	if len(event.Message) > 0 {
		var msg struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal(event.Message, &msg); err == nil && len(msg.Content) > 0 {
			if msg.Content[0].Type == "text" {
				return msg.Content[0].Text
			}
		}
	}

	// Try partialResult
	if len(event.PartialResult) > 0 {
		var result struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal(event.PartialResult, &result); err == nil && result.Delta != "" {
			return result.Delta
		}
	}

	return ""
}

// extractThinkingDelta extracts thinking delta from an event
func extractThinkingDelta(event AgentEvent) string {
	// Try assistantMessageEvent first
	if len(event.AssistantMessageEvent) > 0 {
		var deltaMsg struct {
			Delta string `json:"delta"`
		}
		if err := json.Unmarshal(event.AssistantMessageEvent, &deltaMsg); err == nil && deltaMsg.Delta != "" {
			return deltaMsg.Delta
		}
	}

	// Try Message field
	if len(event.Message) > 0 {
		var msg struct {
			Content []struct {
				Type     string `json:"type"`
				Thinking string `json:"thinking"`
			} `json:"content"`
		}
		if err := json.Unmarshal(event.Message, &msg); err == nil && len(msg.Content) > 0 {
			if msg.Content[0].Type == "thinking" {
				return msg.Content[0].Thinking
			}
		}
	}

	return ""
}

// extractProgress extracts progress from a tool execution event
func extractProgress(event AgentEvent) string {
	if len(event.PartialResult) > 0 {
		var result struct {
			Progress string `json:"progress"`
		}
		if err := json.Unmarshal(event.PartialResult, &result); err == nil && result.Progress != "" {
			return result.Progress
		}
	}
	return ""
}

// extractErrorMessage extracts error message from an event
func extractErrorMessage(event AgentEvent) string {
	// Try Result field
	if len(event.Result) > 0 {
		var result struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(event.Result, &result); err == nil && result.Error != "" {
			return result.Error
		}
	}

	// Try Message field
	if len(event.Message) > 0 {
		var msg struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(event.Message, &msg); err == nil && msg.Error != "" {
			return msg.Error
		}
	}

	return "Unknown error"
}
