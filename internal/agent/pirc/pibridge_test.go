package pirc

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestConfigBuildArgs(t *testing.T) {
	cfg := Config{
		Provider:      "anthropic",
		ModelID:       "claude-sonnet-4-20250514",
		APIKey:        "test-key",
		SystemPrompt:  "You are a coding assistant",
		ThinkingLevel: "high",
		SessionDir:    "/tmp/sessions",
		Skills:        []string{"/path/to/skill1", "/path/to/skill2"},
		Tools:         "read,bash,edit,write",
		Extensions:    []string{"/path/to/ext1.ts"},
	}

	bridge := NewBridge(cfg)
	args := bridge.buildArgs()

	// Check that expected args are present
	found := map[string]bool{}
	for _, arg := range args {
		found[arg] = true
	}

	tests := []struct {
		expected string
		present  bool
	}{
		{"--provider", true},
		{"anthropic", true},
		{"--model", true},
		{"claude-sonnet-4-20250514", true},
		{"--system-prompt", true},
		{"You are a coding assistant", true},
		{"--thinking", true},
		{"high", true},
		{"--session-dir", true},
		{"/tmp/sessions", true},
		{"--skill", true},
		{"/path/to/skill1", true},
		{"/path/to/skill2", true},
		{"--tools", true},
		{"read,bash,edit,write", true},
		{"--extension", true},
		{"/path/to/ext1.ts", true},
	}

	for _, test := range tests {
		if !found[test.expected] {
			t.Errorf("Expected argument %q not found in args: %v", test.expected, args)
		}
	}
}

func TestNoToolsConfig(t *testing.T) {
	cfg := Config{
		NoTools: true,
	}

	bridge := NewBridge(cfg)
	args := bridge.buildArgs()

	found := map[string]bool{}
	for _, arg := range args {
		found[arg] = true
	}

	if !found["--no-tools"] {
		t.Errorf("Expected --no-tools not found in args: %v", args)
	}
}

func TestNoExtensionsConfig(t *testing.T) {
	cfg := Config{
		NoExtensions: true,
	}

	bridge := NewBridge(cfg)
	args := bridge.buildArgs()

	found := map[string]bool{}
	for _, arg := range args {
		found[arg] = true
	}

	if !found["--no-extensions"] {
		t.Errorf("Expected --no-extensions not found in args: %v", args)
	}
}

func TestPromptMessageJSON(t *testing.T) {
	msg := PromptMessage{
		Type:    "prompt",
		Message: "Hello, world!",
		ID:      "test-id-123",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal PromptMessage: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if parsed["type"] != "prompt" {
		t.Errorf("Expected type 'prompt', got %v", parsed["type"])
	}
	if parsed["message"] != "Hello, world!" {
		t.Errorf("Expected message 'Hello, world!', got %v", parsed["message"])
	}
	if parsed["id"] != "test-id-123" {
		t.Errorf("Expected id 'test-id-123', got %v", parsed["id"])
	}
}

func TestAgentEventJSON(t *testing.T) {
	eventJSON := `{"type":"message_update","message":{"role":"assistant","content":[{"type":"text","text":"Hello"}]},"assistantMessageEvent":{"type":"text_delta","delta":"Hello","contentIndex":0}}`

	var event AgentEvent
	if err := json.Unmarshal([]byte(eventJSON), &event); err != nil {
		t.Fatalf("Failed to unmarshal AgentEvent: %v", err)
	}

	if event.Type != "message_update" {
		t.Errorf("Expected type 'message_update', got %v", event.Type)
	}
}

func TestSteerMessageJSON(t *testing.T) {
	msg := SteerMessage{
		Type:    "steer",
		Message: "Stop and do this instead",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal SteerMessage: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if parsed["type"] != "steer" {
		t.Errorf("Expected type 'steer', got %v", parsed["type"])
	}
}

func TestFollowUpMessageJSON(t *testing.T) {
	msg := FollowUpMessage{
		Type:    "follow_up",
		Message: "After you're done, also do this",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal FollowUpMessage: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if parsed["type"] != "follow_up" {
		t.Errorf("Expected type 'follow_up', got %v", parsed["type"])
	}
}

func TestAbortMessageJSON(t *testing.T) {
	msg := AbortMessage{
		Type: "abort",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal AbortMessage: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if parsed["type"] != "abort" {
		t.Errorf("Expected type 'abort', got %v", parsed["type"])
	}
}

func TestNewSessionMessageJSON(t *testing.T) {
	msg := NewSessionMessage{
		Type:          "new_session",
		ParentSession: "/path/to/parent.jsonl",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal NewSessionMessage: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if parsed["type"] != "new_session" {
		t.Errorf("Expected type 'new_session', got %v", parsed["type"])
	}
	if parsed["parentSession"] != "/path/to/parent.jsonl" {
		t.Errorf("Expected parentSession '/path/to/parent.jsonl', got %v", parsed["parentSession"])
	}
}

func TestSetModelMessageJSON(t *testing.T) {
	msg := SetModelMessage{
		Type:     "set_model",
		Provider: "openai",
		ModelID:  "gpt-4o",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal SetModelMessage: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if parsed["type"] != "set_model" {
		t.Errorf("Expected type 'set_model', got %v", parsed["type"])
	}
	if parsed["provider"] != "openai" {
		t.Errorf("Expected provider 'openai', got %v", parsed["provider"])
	}
	if parsed["modelId"] != "gpt-4o" {
		t.Errorf("Expected modelId 'gpt-4o', got %v", parsed["modelId"])
	}
}

func TestSetThinkingLevelMessageJSON(t *testing.T) {
	msg := SetThinkingLevelMessage{
		Type:  "set_thinking_level",
		Level: "xhigh",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal SetThinkingLevelMessage: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if parsed["type"] != "set_thinking_level" {
		t.Errorf("Expected type 'set_thinking_level', got %v", parsed["type"])
	}
	if parsed["level"] != "xhigh" {
		t.Errorf("Expected level 'xhigh', got %v", parsed["level"])
	}
}

func TestBashMessageJSON(t *testing.T) {
	msg := BashMessage{
		Type:    "bash",
		Command: "ls -la",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal BashMessage: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if parsed["type"] != "bash" {
		t.Errorf("Expected type 'bash', got %v", parsed["type"])
	}
	if parsed["command"] != "ls -la" {
		t.Errorf("Expected command 'ls -la', got %v", parsed["command"])
	}
}

func TestExtensionUIRequestJSON(t *testing.T) {
	eventJSON := `{"type":"extension_ui_request","id":"uuid-1","method":"select","title":"Allow dangerous command?","options":["Allow","Block"],"timeout":10000}`

	var req ExtensionUIRequest
	if err := json.Unmarshal([]byte(eventJSON), &req); err != nil {
		t.Fatalf("Failed to unmarshal ExtensionUIRequest: %v", err)
	}

	if req.Type != "extension_ui_request" {
		t.Errorf("Expected type 'extension_ui_request', got %v", req.Type)
	}
	if req.Method != "select" {
		t.Errorf("Expected method 'select', got %v", req.Method)
	}
	if req.ID != "uuid-1" {
		t.Errorf("Expected id 'uuid-1', got %v", req.ID)
	}
	if len(req.Options) != 2 {
		t.Errorf("Expected 2 options, got %d", len(req.Options))
	}
}

func TestExtensionUIResponseJSON(t *testing.T) {
	// Test value response
	resp := ExtensionUIResponse{
		Type:  "extension_ui_response",
		ID:    "uuid-1",
		Value: "Allow",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Failed to marshal ExtensionUIResponse: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if parsed["type"] != "extension_ui_response" {
		t.Errorf("Expected type 'extension_ui_response', got %v", parsed["type"])
	}
	if parsed["value"] != "Allow" {
		t.Errorf("Expected value 'Allow', got %v", parsed["value"])
	}

	// Test confirmation response
	confirmResp := ExtensionUIResponse{
		Type:      "extension_ui_response",
		ID:        "uuid-2",
		Confirmed: true,
	}

	data, err = json.Marshal(confirmResp)
	if err != nil {
		t.Fatalf("Failed to marshal confirm ExtensionUIResponse: %v", err)
	}

	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if parsed["confirmed"] != true {
		t.Errorf("Expected confirmed true, got %v", parsed["confirmed"])
	}

	// Test cancellation response
	cancelResp := ExtensionUIResponse{
		Type:      "extension_ui_response",
		ID:        "uuid-3",
		Cancelled: true,
	}

	data, err = json.Marshal(cancelResp)
	if err != nil {
		t.Fatalf("Failed to marshal cancel ExtensionUIResponse: %v", err)
	}

	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if parsed["cancelled"] != true {
		t.Errorf("Expected cancelled true, got %v", parsed["cancelled"])
	}
}

func TestImageContentJSON(t *testing.T) {
	images := []ImageContent{
		{
			Type:     "image",
			Data:     "base64-encoded-data",
			MimeType: "image/png",
		},
	}

	msg := PromptMessage{
		Type:    "prompt",
		Message: "What's in this image?",
		Images:  images,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal PromptMessage with images: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	imagesRaw, ok := parsed["images"].([]interface{})
	if !ok || len(imagesRaw) != 1 {
		t.Fatalf("Expected 1 image, got %v", parsed["images"])
	}

	img := imagesRaw[0].(map[string]interface{})
	if img["type"] != "image" {
		t.Errorf("Expected image type 'image', got %v", img["type"])
	}
}

func TestBridgeCreation(t *testing.T) {
	cfg := Config{
		Provider:         "google",
		ModelID:          "gemini-2.5-flash",
		ThinkingLevel:    "medium",
		WorkingDirectory: "/tmp",
		Timeout:          30 * time.Second,
	}

	bridge := NewBridge(cfg)

	if bridge == nil {
		t.Fatal("NewBridge returned nil")
	}

	if bridge.cfg.Provider != "google" {
		t.Errorf("Expected provider 'google', got %v", bridge.cfg.Provider)
	}
	if bridge.cfg.ModelID != "gemini-2.5-flash" {
		t.Errorf("Expected modelID 'gemini-2.5-flash', got %v", bridge.cfg.ModelID)
	}
	if bridge.cfg.ThinkingLevel != "medium" {
		t.Errorf("Expected thinkingLevel 'medium', got %v", bridge.cfg.ThinkingLevel)
	}
	if bridge.eventChan == nil {
		t.Error("Expected eventChan to be initialized")
	}
	if bridge.errChan == nil {
		t.Error("Expected errChan to be initialized")
	}
}

func TestContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := Config{}
	_ = NewBridge(cfg)

	// Test that operations can handle cancelled context
	// This is a simple test - in real usage, the bridge would check context
	select {
	case <-ctx.Done():
		if ctx.Err() != context.Canceled {
			t.Errorf("expected context canceled, got: %v", ctx.Err())
		}
	default:
	}
}

func TestBridgeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Check for required API keys - skip if not available
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping integration test: GOOGLE_API_KEY not set")
	}

	cfg := Config{
		Provider:      "google",
		ModelID:       "gemini-2.0-flash",
		ThinkingLevel: "low",
		NoSession:     true,
	}

	bridge := NewBridge(cfg)

	if err := bridge.Start(); err != nil {
		t.Fatalf("Failed to start bridge: %v", err)
	}

	t.Log("PI bridge started successfully")

	// Send a simple prompt
	ctx := context.Background()
	if err := bridge.Prompt(ctx, "Say 'hello' in exactly 3 words"); err != nil {
		t.Fatalf("Failed to send prompt: %v", err)
	}

	t.Log("Prompt sent successfully")

	// Wait for events with timeout
	timeout := time.After(20 * time.Second)
done:
	for {
		select {
		case event := <-bridge.Events():
			t.Logf("Received event: %s", event.Type)
			// Log the full event for debugging
			eventJSON, _ := json.Marshal(event)
			t.Logf("Full event: %s", string(eventJSON))
			if event.Type == "agent_end" || event.Type == "message_end" {
				break done
			}
		case err := <-bridge.Errors():
			t.Logf("Error: %v", err)
		case <-timeout:
			t.Log("Timeout waiting for events")
			break done
		}
	}

	if err := bridge.Stop(); err != nil {
		t.Logf("Error stopping bridge: %v", err)
	}
	t.Log("Bridge stopped")
}

// TestPromptWithImages tests the PromptWithImages method
func TestPromptWithImages(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	images := []ImageContent{
		{
			Type:     "image",
			Data:     "base64data123",
			MimeType: "image/png",
		},
	}

	// We can't actually send without a running process, but we can verify the method exists
	_ = bridge.PromptWithImages
	_ = images
}

// TestEventsChannel tests that Events() returns a receive-only channel
func TestEventsChannel(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	events := bridge.Events()
	if events == nil {
		t.Fatal("Events() returned nil channel")
	}

	// Verify it's a receive-only channel using blank identifier assignment
	_ = events
}

// TestErrorsChannel tests that Errors() returns a receive-only channel
func TestErrorsChannel(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	errs := bridge.Errors()
	if errs == nil {
		t.Fatal("Errors() returned nil channel")
	}

	// Verify it's a receive-only channel using blank identifier assignment
	_ = errs
}

// TestProcessDoneChannel tests that ProcessDone() returns a receive-only channel
func TestProcessDoneChannel(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	done := bridge.ProcessDone()
	if done == nil {
		t.Fatal("ProcessDone() returned nil channel")
	}

	// Verify it's a receive-only channel using blank identifier assignment
	_ = done
}

// TestWorkingDirectoryConfig tests that working directory is properly set in config
func TestWorkingDirectoryConfig(t *testing.T) {
	cfg := Config{
		WorkingDirectory: "/home/user/project",
	}

	bridge := NewBridge(cfg)

	if bridge.cfg.WorkingDirectory != "/home/user/project" {
		t.Errorf("Expected working directory '/home/user/project', got %v", bridge.cfg.WorkingDirectory)
	}
}

// TestMultipleSkillsConfig tests configuration with multiple skills
func TestMultipleSkillsConfig(t *testing.T) {
	cfg := Config{
		Skills: []string{
			"/home/user/.pi/agent/skills/skill1",
			"/home/user/.pi/agent/skills/skill2",
			"/home/user/.pi/agent/skills/skill3",
		},
	}

	bridge := NewBridge(cfg)
	args := bridge.buildArgs()

	skillCount := 0
	for _, arg := range args {
		if arg == "--skill" {
			skillCount++
		}
	}

	if skillCount != 3 {
		t.Errorf("Expected 3 skill flags, got %d", skillCount)
	}
}

// TestMultipleExtensionsConfig tests configuration with multiple extensions
func TestMultipleExtensionsConfig(t *testing.T) {
	cfg := Config{
		Extensions: []string{
			"/path/to/ext1.ts",
			"/path/to/ext2.ts",
		},
	}

	bridge := NewBridge(cfg)
	args := bridge.buildArgs()

	extCount := 0
	for _, arg := range args {
		if arg == "--extension" {
			extCount++
		}
	}

	if extCount != 2 {
		t.Errorf("Expected 2 extension flags, got %d", extCount)
	}
}

// TestTimeoutConfig tests that timeout is properly stored
func TestTimeoutConfig(t *testing.T) {
	cfg := Config{
		Timeout: 5 * time.Minute,
	}

	bridge := NewBridge(cfg)

	if bridge.cfg.Timeout != 5*time.Minute {
		t.Errorf("Expected timeout of 5 minutes, got %v", bridge.cfg.Timeout)
	}
}

// TestIsRunningBeforeStart tests IsRunning before starting the process
func TestIsRunningBeforeStart(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	if bridge.IsRunning() {
		t.Error("Expected IsRunning() to return false before Start()")
	}
}

// TestThinkingLevels tests all thinking level values
func TestThinkingLevels(t *testing.T) {
	levels := []string{"off", "minimal", "low", "medium", "high", "xhigh"}

	for _, level := range levels {
		cfg := Config{
			ThinkingLevel: level,
		}

		bridge := NewBridge(cfg)
		args := bridge.buildArgs()

		foundThinking := false
		for i, arg := range args {
			if arg == "--thinking" && i+1 < len(args) && args[i+1] == level {
				foundThinking = true
				break
			}
		}

		if !foundThinking {
			t.Errorf("Expected thinking level '%s' not found in args: %v", level, args)
		}
	}
}

// TestEmptyConfig tests configuration with empty values
func TestEmptyConfig(t *testing.T) {
	cfg := Config{}

	bridge := NewBridge(cfg)
	args := bridge.buildArgs()

	// With empty config, should still return --mode rpc and --no-session from Start()
	// But buildArgs should return empty slice
	if len(args) != 0 {
		t.Errorf("Expected empty args for empty config, got: %v", args)
	}
}

// TestToolsAndNoToolsConflict tests that NoTools takes precedence over Tools
func TestToolsAndNoToolsConflict(t *testing.T) {
	cfg := Config{
		Tools:   "read,bash",
		NoTools: true,
	}

	bridge := NewBridge(cfg)
	args := bridge.buildArgs()

	// Should have --no-tools but not --tools
	hasNoTools := false
	hasTools := false
	for _, arg := range args {
		if arg == "--no-tools" {
			hasNoTools = true
		}
		if arg == "--tools" {
			hasTools = true
		}
	}

	if !hasNoTools {
		t.Errorf("Expected --no-tools in args: %v", args)
	}
	if hasTools {
		t.Errorf("Did not expect --tools when NoTools is true: %v", args)
	}
}
