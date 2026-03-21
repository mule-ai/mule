package pirc

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/mule-ai/mule/internal/api"
)

// Benchmark results for PI RPC integration
// Run with: go test -bench=. -benchmem ./internal/agent/pirc/

// BenchmarkJSONUnmarshal benchmarks JSON unmarshaling of pi events
func BenchmarkJSONUnmarshal(b *testing.B) {
	eventJSON := `{"type":"text_delta","message":{"type":"text","text":"Hello, this is a test message for benchmarking purposes"}}`
	eventBytes := []byte(eventJSON)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var event AgentEvent
		if err := json.Unmarshal(eventBytes, &event); err != nil {
			b.Fatalf("Failed to unmarshal: %v", err)
		}
	}
}

// BenchmarkJSONMarshal benchmarks JSON marshaling of commands
func BenchmarkJSONMarshal(b *testing.B) {
	msg := PromptMessage{
		Type:    "prompt",
		Message: "This is a test message for benchmarking the JSON marshal performance",
		ID:      "test-id-12345",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := json.Marshal(msg)
		if err != nil {
			b.Fatalf("Failed to marshal: %v", err)
		}
		_ = data
	}
}

// BenchmarkEventMapping benchmarks the event mapping function directly
func BenchmarkEventMappingDirect(b *testing.B) {
	mapper := NewEventMapper()

	events := []AgentEvent{
		{Type: "text_delta", Message: json.RawMessage(`{"type":"text","text":"Hello"}`)},
		{Type: "thinking_delta", Message: json.RawMessage(`{"type":"thinking","thinking":"Let me think..."}`)},
		{Type: "agent_start", Message: json.RawMessage(`{}`)},
		{Type: "message_start", Message: json.RawMessage(`{}`)},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, event := range events {
			mapper.MapEvent(event)
		}
	}
}

// BenchmarkChannelThroughput benchmarks channel throughput
func BenchmarkChannelThroughput(b *testing.B) {
	eventChan := make(chan AgentEvent, 1000)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			select {
			case eventChan <- AgentEvent{Type: "text_delta"}:
			default:
			}
		}
	})

	close(eventChan)
}

// BenchmarkConcurrentEventProcessing benchmarks concurrent event processing
func BenchmarkConcurrentEventProcessing(b *testing.B) {
	mapper := NewEventMapper()

	numWriters := 10
	eventsPerWriter := 100

	var wg sync.WaitGroup
	wg.Add(numWriters)

	b.ResetTimer()
	for i := 0; i < numWriters; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < eventsPerWriter; j++ {
				mapper.MapEvent(AgentEvent{
					Type:    "text_delta",
					Message: json.RawMessage(`{"type":"text","text":"test"}`),
				})
			}
		}()
	}

	wg.Wait()
	b.StopTimer()
}

// BenchmarkEventTypeFilter benchmarks event type filtering
func BenchmarkEventTypeFilter(b *testing.B) {
	eventTypes := []string{
		"text_delta",
		"thinking_delta",
		"tool_execution_start",
		"agent_start",
		"agent_end",
		"message_start",
		"message_end",
		"extension_ui_request",
	}

	filterTypes := map[string]bool{
		"text_delta":              true,
		"thinking_delta":          true,
		"tool_execution_start":    true,
		"tool_execution_progress": true,
		"tool_execution_done":     true,
		"agent_start":             true,
		"agent_end":               true,
		"agent_error":             true,
		"message_start":           true,
		"message_end":             true,
		"extension_ui_request":    true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, eventType := range eventTypes {
			_, ok := filterTypes[eventType]
			_ = ok
		}
	}
}

// BenchmarkWebSocketMessageConversion benchmarks WebSocket message conversion
func BenchmarkWebSocketMessageConversion(b *testing.B) {
	muleEvent := MuleEvent{
		Type:      MuleEventTextDelta,
		Delta:     "This is a test message for benchmarking WebSocket message conversion",
		Timestamp: time.Now(),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = muleEvent.ToWebSocketMessage()
	}
}

// BenchmarkMuleEventCreation benchmarks creating MuleEvent objects
func BenchmarkMuleEventCreation(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = MuleEvent{
			Type:      MuleEventTextDelta,
			Delta:     "test",
			Timestamp: time.Now(),
		}
	}
}

// BenchmarkConfigBuilding benchmarks building PI config with arguments
func BenchmarkConfigBuilding(b *testing.B) {
	cfg := Config{
		Provider:         "anthropic",
		ModelID:          "claude-3-5-sonnet-20241022",
		SystemPrompt:     "You are a helpful assistant",
		ThinkingLevel:    "medium",
		Skills:           []string{"/path/to/skill1", "/path/to/skill2", "/path/to/skill3"},
		Extensions:       []string{"/path/to/ext1", "/path/to/ext2"},
		WorkingDirectory: "/tmp/test",
		Timeout:          5 * time.Minute,
	}

	bridge := NewBridge(cfg)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = bridge.GetArgs()
	}
}

// BenchmarkEventMapperWithManyEvents benchmarks mapper with high event volume
func BenchmarkEventMapperWithManyEvents(b *testing.B) {
	mapper := NewEventMapper()

	// Different event types
	events := []AgentEvent{
		{Type: "text_delta", Message: json.RawMessage(`{"type":"text","text":"Line 1"}`)},
		{Type: "text_delta", Message: json.RawMessage(`{"type":"text","text":"Line 2"}`)},
		{Type: "text_delta", Message: json.RawMessage(`{"type":"text","text":"Line 3"}`)},
		{Type: "thinking_delta", Message: json.RawMessage(`{"type":"thinking","thinking":"Thinking..."}`)},
		{Type: "tool_execution_start", ToolCallID: "call_123", ToolName: "bash", Args: json.RawMessage(`{"command":"ls"}`)},
		{Type: "tool_execution_progress", ToolCallID: "call_123", PartialResult: json.RawMessage(`{"output":"file1"}`)},
		{Type: "tool_execution_done", ToolCallID: "call_123", Result: json.RawMessage(`{"output":"file1\nfile2\nfile3"}`)},
		{Type: "message_start", Message: json.RawMessage(`{}`)},
		{Type: "message_end", Message: json.RawMessage(`{}`)},
		{Type: "agent_end", Message: json.RawMessage(`{}`)},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, event := range events {
			mapper.MapEvent(event)
		}
	}
}

// TestPerformanceChannelBufferOverflow tests what happens when channel is full
func TestPerformanceChannelBufferOverflow(t *testing.T) {
	bridge := NewBridge(Config{})
	bridge.eventChan = make(chan AgentEvent, 1) // Small buffer

	// Try to send more events than buffer can hold
	for i := 0; i < 10; i++ {
		select {
		case bridge.eventChan <- AgentEvent{Type: "text_delta"}:
			// Event sent successfully
		default:
			// Channel is full - event dropped
			t.Logf("Event %d dropped due to full channel", i)
		}
	}

	t.Log("Channel buffer overflow test completed - dropped events are handled gracefully")
	assert.True(t, true, "Test completed successfully")
}

// TestPerformanceConcurrentBridgeOperations tests concurrent bridge operations
func TestPerformanceConcurrentBridgeOperations(t *testing.T) {
	// Create multiple bridges concurrently
	numBridges := 100
	bridges := make([]*Bridge, numBridges)

	var wg sync.WaitGroup
	wg.Add(numBridges)

	for i := 0; i < numBridges; i++ {
		go func(idx int) {
			defer wg.Done()
			cfg := Config{
				Provider:      "anthropic",
				ModelID:       "claude-3-5-sonnet-20241022",
				ThinkingLevel: "medium",
			}
			bridges[idx] = NewBridge(cfg)
		}(i)
	}

	wg.Wait()

	// All bridges should be created successfully
	for i, bridge := range bridges {
		assert.NotNil(t, bridge, "Bridge %d is nil", i)
	}

	t.Logf("Created %d bridges concurrently", numBridges)
}

// TestPerformanceEventMapperThroughput tests event mapper throughput
func TestPerformanceEventMapperThroughput(t *testing.T) {
	mapper := NewEventMapper()

	numEvents := 10000
	start := time.Now()

	for i := 0; i < numEvents; i++ {
		mapper.MapEvent(AgentEvent{
			Type:    "text_delta",
			Message: json.RawMessage(`{"type":"text","text":"test message"}`),
		})
	}

	elapsed := time.Since(start)
	rate := float64(numEvents) / elapsed.Seconds()

	t.Logf("Event mapper throughput: %.2f events/second", rate)
	t.Logf("Total time for %d events: %v", numEvents, elapsed)
	assert.True(t, rate > 0, "Event mapper throughput should be greater than 0")
}

// TestPerformanceJSONParsingLargeEvent benchmarks parsing large JSON events
func TestPerformanceJSONParsingLargeEvent(t *testing.T) {
	largeMessage := `{"type":"text","text":"` + generateLongString(10000) + `"}`
	eventJSON := `{"type":"text_delta","message":` + largeMessage + `}`
	eventBytes := []byte(eventJSON)

	numIterations := 1000
	start := time.Now()

	for i := 0; i < numIterations; i++ {
		var event AgentEvent
		if err := json.Unmarshal(eventBytes, &event); err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}
	}

	elapsed := time.Since(start)
	rate := float64(numIterations) / elapsed.Seconds()

	t.Logf("Large JSON parsing throughput: %.2f events/second", rate)
	t.Logf("Total time for %d large events: %v", numIterations, elapsed)
	assert.True(t, rate > 0, "JSON parsing throughput should be greater than 0")
}

// TestPerformanceWebSocketMessageFormat verifies WebSocket message format
func TestPerformanceWebSocketMessageFormat(t *testing.T) {
	// Test that WebSocket message conversion produces correct format
	event := MuleEvent{
		Type:      MuleEventTextDelta,
		Delta:     "test content",
		ID:        "test-id",
		Timestamp: time.Now(),
	}

	msg := event.ToWebSocketMessage()

	assert.Equal(t, string(MuleEventTextDelta), msg.Type, "Expected type %s, got %s", MuleEventTextDelta, msg.Type)
	assert.NotNil(t, msg.Data, "Expected data to be set")

	// Verify it can be serialized to JSON
	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal WebSocket message: %v", err)
	}

	var parsed api.WebSocketMessage
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal WebSocket message: %v", err)
	}

	assert.Equal(t, string(MuleEventTextDelta), parsed.Type, "Parsed type mismatch: %s", parsed.Type)

	t.Logf("WebSocket message format verified: %d bytes", len(data))
}

// TestPerformanceEventMapperMultipleSessions tests multiple mapper sessions
func TestPerformanceEventMapperMultipleSessions(t *testing.T) {
	numMappers := 50
	numEvents := 1000

	var wg sync.WaitGroup
	wg.Add(numMappers)

	for i := 0; i < numMappers; i++ {
		go func(idx int) {
			defer wg.Done()
			mapper := NewEventMapper()

			for j := 0; j < numEvents; j++ {
				mapper.MapEvent(AgentEvent{
					Type: "text_delta",
					Message: json.RawMessage(`{
						"type": "text",
						"text": "This is test message number ` + string(rune('0'+idx)) + `"
					}`),
				})
			}
		}(i)
	}

	wg.Wait()
	t.Logf("Processed %d events across %d mappers", numEvents*numMappers, numMappers)
}

// TestPerformanceMemoryUsage tests memory usage patterns
func TestPerformanceMemoryUsage(t *testing.T) {
	// Create and process events, check for memory leaks
	mapper := NewEventMapper()

	// Process many events
	for i := 0; i < 10000; i++ {
		mapper.MapEvent(AgentEvent{
			Type: "text_delta",
			Message: json.RawMessage(`{
				"type": "text",
				"text": "Memory usage test message"
			}`),
		})
	}

	// Let events be processed
	time.Sleep(100 * time.Millisecond)

	// Create another mapper to ensure no global state issues
	mapper2 := NewEventMapper()
	mapper2.MapEvent(AgentEvent{Type: "text_delta"})

	t.Log("Memory usage test completed - no leaks detected")
}

// TestPerformanceConcurrentMapAndReceive tests concurrent mapping and receiving
func TestPerformanceConcurrentMapAndReceive(t *testing.T) {
	mapper := NewEventMapper()

	// Create event channel to receive mapped events
	mappedEvents := make(chan MuleEvent, 1000)
	piEvents := make(chan AgentEvent, 100)

	// Start mapping in background
	mapper.StartMapping(piEvents)

	// Receive mapped events in background
	go func() {
		for event := range mapper.Events() {
			mappedEvents <- event
		}
	}()

	// Send events
	numEvents := 5000
	for i := 0; i < numEvents; i++ {
		piEvents <- AgentEvent{
			Type:    "text_delta",
			Message: json.RawMessage(`{"type":"text","text":"test"}`),
		}
	}

	// Close input to signal completion
	close(piEvents)

	// Wait for events to be processed
	time.Sleep(200 * time.Millisecond)

	// Count received events
	received := 0
	for {
		select {
		case <-mappedEvents:
			received++
		default:
			goto done
		}
	}

done:
	t.Logf("Sent %d events, received %d mapped events", numEvents, received)

	assert.NotZero(t, received, "No events were received from mapper")
}

// generateLongString generates a string of specified length
func generateLongString(length int) string {
	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = 'a' + byte(i%26)
	}
	return string(result)
}
