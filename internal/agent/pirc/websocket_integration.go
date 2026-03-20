package pirc

import (
	"context"
	"sync"
	"time"
)

// EventBroadcaster is an interface for broadcasting events
type EventBroadcaster interface {
	BroadcastAgentEvent(eventType string, data interface{})
}

// WebSocketMessage represents a message sent over WebSocket
type WebSocketMessage struct {
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
}

// PIEventStreamer handles streaming pi events to WebSocket clients
type PIEventStreamer struct {
	hub        EventBroadcaster
	jobID      string
	eventTypes []string // Event types to broadcast (empty = all)
	mu         sync.Mutex
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewPIEventStreamer creates a new PI event streamer
func NewPIEventStreamer(hub EventBroadcaster, jobID string) *PIEventStreamer {
	ctx, cancel := context.WithCancel(context.Background())
	return &PIEventStreamer{
		hub:    hub,
		jobID:  jobID,
		ctx:    ctx,
		cancel: cancel,
	}
}

// SetEventTypes sets which event types to broadcast (empty = all)
func (s *PIEventStreamer) SetEventTypes(eventTypes []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eventTypes = eventTypes
}

// Start starts streaming events from the bridge to WebSocket
func (s *PIEventStreamer) Start(bridge *Bridge, mapper *EventMapper) {
	// Start mapping bridge events to Mule events
	mapper.StartMapping(bridge.Events())

	// Start streaming Mule events to WebSocket
	s.wg.Add(1)
	go s.streamEvents(mapper)
}

// streamEvents streams Mule events to WebSocket clients
func (s *PIEventStreamer) streamEvents(mapper *EventMapper) {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		case event, ok := <-mapper.Events():
			if !ok {
				// Channel closed, stream ended
				// Send a final agent_end event
				s.broadcastEvent(MuleEvent{
					Type:      MuleEventAgentEnd,
					Timestamp: time.Now(),
				})
				return
			}

			// Check if we should broadcast this event type
			if !s.shouldBroadcast(event.Type) {
				continue
			}

			s.broadcastEvent(event)
		}
	}
}

// shouldBroadcast checks if the event type should be broadcast
func (s *PIEventStreamer) shouldBroadcast(eventType MuleEventType) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.eventTypes) == 0 {
		return true // Broadcast all events
	}

	for _, t := range s.eventTypes {
		if string(eventType) == t {
			return true
		}
	}
	return false
}

// broadcastEvent sends an event to all WebSocket clients
func (s *PIEventStreamer) broadcastEvent(event MuleEvent) {
	// Use the hub to broadcast the event
	if s.hub != nil {
		// Convert MuleEvent to WebSocket message
		msg := WebSocketMessage{
			Type:      string(event.Type),
			Data:      event,
			Timestamp: event.Timestamp,
		}
		s.hub.BroadcastAgentEvent(string(event.Type), msg)
	}
}

// Stop stops the event streamer
func (s *PIEventStreamer) Stop() {
	s.cancel()
	s.wg.Wait()
}

// StreamAgentExecution executes an agent with pi and streams events via WebSocket
// This is the main entry point for streaming agent execution
func StreamAgentExecution(
	ctx context.Context,
	hub EventBroadcaster,
	config Config,
	messages []string,
	jobID string,
) (*string, error) {
	// Create the bridge
	bridge := NewBridge(config)

	// Start the pi process
	if err := bridge.Start(); err != nil {
		return nil, err
	}

	// Create event mapper
	mapper := NewEventMapper()

	// Create streamer
	streamer := NewPIEventStreamer(hub, jobID)

	// Configure which events to broadcast
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
		"response",
		"agent_error",
		"message_start",
		"message_end",
		"extension_ui_request",
		"extension_ui_response",
	})

	// Start streaming
	streamer.Start(bridge, mapper)

	// Send the prompt
	for _, msg := range messages {
		if err := bridge.Prompt(ctx, msg); err != nil {
			_ = bridge.Stop()
			streamer.Stop()
			return nil, err
		}
	}

	// Wait for agent to finish, respecting context cancellation
	select {
	case <-bridge.ProcessDone():
		// Process finished normally
	case <-ctx.Done():
		// Context was cancelled or timed out
		// Stop the bridge forcefully
		_ = bridge.Stop()
	}

	// Give a moment for final events to be processed
	time.Sleep(100 * time.Millisecond)

	// Stop streaming
	streamer.Stop()

	// Return nil - caller should collect final response from events if needed
	return nil, nil
}
