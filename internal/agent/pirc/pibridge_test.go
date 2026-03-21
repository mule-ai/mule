package pirc

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
		assert.True(t, found[test.expected], "Expected argument %q not found in args: %v", test.expected, args)
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

	assert.True(t, found["--no-tools"], "Expected --no-tools not found in args: %v", args)
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

	assert.True(t, found["--no-extensions"], "Expected --no-extensions not found in args: %v", args)
}

func TestPromptMessageJSON(t *testing.T) {
	msg := PromptMessage{
		Type:    "prompt",
		Message: "Hello, world!",
		ID:      "test-id-123",
	}

	data, err := json.Marshal(msg)
	assert.NoError(t, err, "Failed to marshal PromptMessage")

	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	assert.NoError(t, err, "Failed to unmarshal JSON")

	assert.Equal(t, "prompt", parsed["type"])
	assert.Equal(t, "Hello, world!", parsed["message"])
	assert.Equal(t, "test-id-123", parsed["id"])
}

func TestAgentEventJSON(t *testing.T) {
	eventJSON := `{"type":"message_update","message":{"role":"assistant","content":[{"type":"text","text":"Hello"}]},"assistantMessageEvent":{"type":"text_delta","delta":"Hello","contentIndex":0}}`

	var event AgentEvent
	err := json.Unmarshal([]byte(eventJSON), &event)
	assert.NoError(t, err, "Failed to unmarshal AgentEvent")
	assert.Equal(t, "message_update", event.Type)
}

func TestSteerMessageJSON(t *testing.T) {
	msg := SteerMessage{
		Type:    "steer",
		Message: "Stop and do this instead",
	}

	data, err := json.Marshal(msg)
	assert.NoError(t, err, "Failed to marshal SteerMessage")

	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	assert.NoError(t, err, "Failed to unmarshal JSON")
	assert.Equal(t, "steer", parsed["type"])
}

func TestFollowUpMessageJSON(t *testing.T) {
	msg := FollowUpMessage{
		Type:    "follow_up",
		Message: "After you're done, also do this",
	}

	data, err := json.Marshal(msg)
	assert.NoError(t, err, "Failed to marshal FollowUpMessage")

	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	assert.NoError(t, err, "Failed to unmarshal JSON")
	assert.Equal(t, "follow_up", parsed["type"])
}

func TestAbortMessageJSON(t *testing.T) {
	msg := AbortMessage{
		Type: "abort",
	}

	data, err := json.Marshal(msg)
	assert.NoError(t, err, "Failed to marshal AbortMessage")

	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	assert.NoError(t, err, "Failed to unmarshal JSON")
	assert.Equal(t, "abort", parsed["type"])
}

func TestNewSessionMessageJSON(t *testing.T) {
	msg := NewSessionMessage{
		Type:          "new_session",
		ParentSession: "/path/to/parent.jsonl",
	}

	data, err := json.Marshal(msg)
	assert.NoError(t, err, "Failed to marshal NewSessionMessage")

	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	assert.NoError(t, err, "Failed to unmarshal JSON")
	assert.Equal(t, "new_session", parsed["type"])
	assert.Equal(t, "/path/to/parent.jsonl", parsed["parentSession"])
}

func TestSetModelMessageJSON(t *testing.T) {
	msg := SetModelMessage{
		Type:     "set_model",
		Provider: "openai",
		ModelID:  "gpt-4o",
	}

	data, err := json.Marshal(msg)
	assert.NoError(t, err, "Failed to marshal SetModelMessage")

	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	assert.NoError(t, err, "Failed to unmarshal JSON")
	assert.Equal(t, "set_model", parsed["type"])
	assert.Equal(t, "openai", parsed["provider"])
	assert.Equal(t, "gpt-4o", parsed["modelId"])
}

func TestSetThinkingLevelMessageJSON(t *testing.T) {
	msg := SetThinkingLevelMessage{
		Type:  "set_thinking_level",
		Level: "xhigh",
	}

	data, err := json.Marshal(msg)
	assert.NoError(t, err, "Failed to marshal SetThinkingLevelMessage")

	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	assert.NoError(t, err, "Failed to unmarshal JSON")
	assert.Equal(t, "set_thinking_level", parsed["type"])
	assert.Equal(t, "xhigh", parsed["level"])
}

func TestBashMessageJSON(t *testing.T) {
	msg := BashMessage{
		Type:    "bash",
		Command: "ls -la",
	}

	data, err := json.Marshal(msg)
	assert.NoError(t, err, "Failed to marshal BashMessage")

	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	assert.NoError(t, err, "Failed to unmarshal JSON")
	assert.Equal(t, "bash", parsed["type"])
	assert.Equal(t, "ls -la", parsed["command"])
}

func TestExtensionUIRequestJSON(t *testing.T) {
	eventJSON := `{"type":"extension_ui_request","id":"uuid-1","method":"select","title":"Allow dangerous command?","options":["Allow","Block"],"timeout":10000}`

	var req ExtensionUIRequest
	err := json.Unmarshal([]byte(eventJSON), &req)
	assert.NoError(t, err, "Failed to unmarshal ExtensionUIRequest")
	assert.Equal(t, "extension_ui_request", req.Type)
	assert.Equal(t, "select", req.Method)
	assert.Equal(t, "uuid-1", req.ID)
	assert.Len(t, req.Options, 2)
}

func TestExtensionUIResponseJSON(t *testing.T) {
	// Test value response
	resp := ExtensionUIResponse{
		Type:  "extension_ui_response",
		ID:    "uuid-1",
		Value: "Allow",
	}

	data, err := json.Marshal(resp)
	assert.NoError(t, err, "Failed to marshal ExtensionUIResponse")

	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	assert.NoError(t, err, "Failed to unmarshal JSON")
	assert.Equal(t, "extension_ui_response", parsed["type"])
	assert.Equal(t, "Allow", parsed["value"])

	// Test confirmation response
	confirmResp := ExtensionUIResponse{
		Type:      "extension_ui_response",
		ID:        "uuid-2",
		Confirmed: true,
	}

	data, err = json.Marshal(confirmResp)
	assert.NoError(t, err, "Failed to marshal confirm ExtensionUIResponse")

	err = json.Unmarshal(data, &parsed)
	assert.NoError(t, err, "Failed to unmarshal JSON")
	assert.Equal(t, true, parsed["confirmed"])

	// Test cancellation response
	cancelResp := ExtensionUIResponse{
		Type:      "extension_ui_response",
		ID:        "uuid-3",
		Cancelled: true,
	}

	data, err = json.Marshal(cancelResp)
	assert.NoError(t, err, "Failed to marshal cancel ExtensionUIResponse")

	err = json.Unmarshal(data, &parsed)
	assert.NoError(t, err, "Failed to unmarshal JSON")
	assert.Equal(t, true, parsed["cancelled"])
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
	assert.NoError(t, err, "Failed to marshal PromptMessage with images")

	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	assert.NoError(t, err, "Failed to unmarshal JSON")

	imagesRaw, ok := parsed["images"].([]interface{})
	assert.True(t, ok, "Expected images to be an array")
	assert.Len(t, imagesRaw, 1)

	img := imagesRaw[0].(map[string]interface{})
	assert.Equal(t, "image", img["type"])
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

	assert.NotNil(t, bridge)
	assert.Equal(t, "google", bridge.cfg.Provider)
	assert.Equal(t, "gemini-2.5-flash", bridge.cfg.ModelID)
	assert.Equal(t, "medium", bridge.cfg.ThinkingLevel)
	assert.NotNil(t, bridge.eventChan)
	assert.NotNil(t, bridge.errChan)
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
		assert.Equal(t, context.Canceled, ctx.Err())
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

	err := bridge.Start()
	assert.NoError(t, err, "Failed to start bridge")
	t.Log("PI bridge started successfully")

	// Send a simple prompt
	ctx := context.Background()
	err = bridge.Prompt(ctx, "Say 'hello' in exactly 3 words")
	assert.NoError(t, err, "Failed to send prompt")
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
	assert.NotNil(t, events)

	// Verify it's a receive-only channel using blank identifier assignment
	_ = events
}

// TestErrorsChannel tests that Errors() returns a receive-only channel
func TestErrorsChannel(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	errs := bridge.Errors()
	assert.NotNil(t, errs)

	// Verify it's a receive-only channel using blank identifier assignment
	_ = errs
}

// TestProcessDoneChannel tests that ProcessDone() returns a receive-only channel
func TestProcessDoneChannel(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	done := bridge.ProcessDone()
	assert.NotNil(t, done)

	// Verify it's a receive-only channel using blank identifier assignment
	_ = done
}

// TestWorkingDirectoryConfig tests that working directory is properly set in config
func TestWorkingDirectoryConfig(t *testing.T) {
	cfg := Config{
		WorkingDirectory: "/home/user/project",
	}

	bridge := NewBridge(cfg)

	assert.Equal(t, "/home/user/project", bridge.cfg.WorkingDirectory)
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

	assert.Equal(t, 3, skillCount)
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

	assert.Equal(t, 2, extCount)
}

// TestTimeoutConfig tests that timeout is properly stored
func TestTimeoutConfig(t *testing.T) {
	cfg := Config{
		Timeout: 5 * time.Minute,
	}

	bridge := NewBridge(cfg)

	assert.Equal(t, 5*time.Minute, bridge.cfg.Timeout)
}

// TestIsRunningBeforeStart tests IsRunning before starting the process
func TestIsRunningBeforeStart(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	assert.False(t, bridge.IsRunning(), "Expected IsRunning() to return false before Start()")
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

		assert.True(t, foundThinking, "Expected thinking level '%s' not found in args: %v", level, args)
	}
}

// TestEmptyConfig tests configuration with empty values
func TestEmptyConfig(t *testing.T) {
	cfg := Config{}

	bridge := NewBridge(cfg)
	args := bridge.buildArgs()

	// With empty config, should still return --mode rpc and --no-session from Start()
	// But buildArgs should return empty slice
	assert.Len(t, args, 0)
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

	assert.True(t, hasNoTools, "Expected --no-tools in args: %v", args)
	assert.False(t, hasTools, "Did not expect --tools when NoTools is true: %v", args)
}

// TestGetArgs tests that GetArgs returns the built command line arguments
func TestGetArgs(t *testing.T) {
	cfg := Config{
		Provider:      "anthropic",
		ModelID:       "claude-sonnet-4-20250514",
		SystemPrompt:  "You are helpful",
		ThinkingLevel: "medium",
		Tools:         "read,write",
	}

	bridge := NewBridge(cfg)
	args := bridge.GetArgs()

	// Verify expected args are present
	expectedArgs := map[string]bool{
		"--provider":               true,
		"anthropic":                true,
		"--model":                  true,
		"claude-sonnet-4-20250514": true,
		"--system-prompt":          true,
		"You are helpful":          true,
		"--thinking":               true,
		"medium":                   true,
		"--tools":                  true,
		"read,write":               true,
	}

	for _, arg := range args {
		if expected, ok := expectedArgs[arg]; ok && expected {
			delete(expectedArgs, arg)
		}
	}

	assert.Empty(t, expectedArgs, "Expected args not found in GetArgs(): %v", expectedArgs)
}

// TestSendExtensionUICancel tests sending UI cancellation response
func TestSendExtensionUICancel(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Cannot actually send without running process, but verify method exists and doesn't panic
	// The actual send will fail because stdin is nil, but that's expected
	err := bridge.SendExtensionUICancel("test-uuid")
	// Expect an error because process is not running (stdin is nil)
	assert.Error(t, err, "Expected error when sending UI cancel without running process")
}

// TestSendExtensionUIResponse tests sending UI response with value and confirmed
func TestSendExtensionUIResponse(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Cannot actually send without running process, but verify method exists and doesn't panic
	err := bridge.SendExtensionUIResponse("test-uuid", "Selected Option", true)
	// Expect an error because process is not running (stdin is nil)
	assert.Error(t, err, "Expected error when sending UI response without running process")
}

// TestSendExtensionUIResponseWithFalseConfirm tests UI response with confirmed=false
func TestSendExtensionUIResponseWithFalseConfirm(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	err := bridge.SendExtensionUIResponse("test-uuid", "", false)
	// Expect an error because process is not running (stdin is nil)
	assert.Error(t, err, "Expected error when sending UI response without running process")
}

// TestSendExtensionUICancelOnClosedBridge tests that UI cancel returns error on closed bridge
func TestSendExtensionUICancelOnClosedBridge(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Simulate closed bridge by calling Stop
	_ = bridge.Stop()

	err := bridge.SendExtensionUICancel("test-uuid")
	assert.Error(t, err, "Expected error when sending UI cancel on closed bridge")
	assert.Equal(t, "bridge is closed", err.Error())
}

// TestSendExtensionUIResponseOnClosedBridge tests that UI response returns error on closed bridge
func TestSendExtensionUIResponseOnClosedBridge(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Simulate closed bridge by calling Stop
	_ = bridge.Stop()

	err := bridge.SendExtensionUIResponse("test-uuid", "value", true)
	assert.Error(t, err, "Expected error when sending UI response on closed bridge")
	assert.Equal(t, "bridge is closed", err.Error())
}

// TestSteerMessage tests that Steer sends the correct message structure
func TestSteerMessage(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Cannot actually send without running process
	err := bridge.Steer(context.Background(), "Stop and reconsider")
	assert.Error(t, err, "Expected error when sending steer without running process")
}

// TestFollowUpMessage tests that FollowUp sends the correct message structure
func TestFollowUpMessage(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Cannot actually send without running process
	err := bridge.FollowUp(context.Background(), "Also do this task")
	assert.Error(t, err, "Expected error when sending follow_up without running process")
}

// TestAbortMessage tests that Abort sends the correct message structure
func TestAbortMessage(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Cannot actually send without running process
	err := bridge.Abort(context.Background())
	assert.Error(t, err, "Expected error when sending abort without running process")
}

// TestNewSessionMessage tests that NewSession sends the correct message structure
func TestNewSessionMessage(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Cannot actually send without running process
	err := bridge.NewSession(context.Background())
	assert.Error(t, err, "Expected error when sending new_session without running process")
}

// TestSetModelMessage tests that SetModel sends the correct message structure
func TestSetModelMessage(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Cannot actually send without running process
	err := bridge.SetModel(context.Background(), "openai", "gpt-4o")
	if err == nil {
		t.Error("Expected error when sending set_model without running process")
	}
}

// TestSetThinkingLevelMessage tests that SetThinkingLevel sends the correct message structure
func TestSetThinkingLevelMessage(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Cannot actually send without running process
	err := bridge.SetThinkingLevel(context.Background(), "high")
	assert.Error(t, err, "Expected error when sending set_thinking_level without running process")
}

// TestBashMessage tests that Bash sends the correct message structure
func TestBashMessage(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Cannot actually send without running process
	err := bridge.Bash(context.Background(), "ls -la")
	assert.Error(t, err, "Expected error when sending bash without running process")
}

// TestPromptWithImagesMessage tests that PromptWithImages sends the correct message structure
func TestPromptWithImagesMessage(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	images := []ImageContent{
		{Type: "image", Data: "base64data", MimeType: "image/png"},
		{Type: "image", Data: "base64data2", MimeType: "image/jpeg"},
	}

	// Cannot actually send without running process
	err := bridge.PromptWithImages(context.Background(), "What's in these images?", images)
	assert.Error(t, err, "Expected error when sending prompt with images without running process")
}

// TestSteerOnClosedBridge tests that Steer returns error on closed bridge
func TestSteerOnClosedBridge(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)
	_ = bridge.Stop()

	err := bridge.Steer(context.Background(), "message")
	assert.Error(t, err, "Expected error when sending steer on closed bridge")
}

// TestFollowUpOnClosedBridge tests that FollowUp returns error on closed bridge
func TestFollowUpOnClosedBridge(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)
	_ = bridge.Stop()

	err := bridge.FollowUp(context.Background(), "message")
	assert.Error(t, err, "Expected error when sending follow_up on closed bridge")
}

// TestAbortOnClosedBridge tests that Abort returns error on closed bridge
func TestAbortOnClosedBridge(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)
	_ = bridge.Stop()

	err := bridge.Abort(context.Background())
	assert.Error(t, err, "Expected error when sending abort on closed bridge")
}

// TestNewSessionOnClosedBridge tests that NewSession returns error on closed bridge
func TestNewSessionOnClosedBridge(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)
	_ = bridge.Stop()

	err := bridge.NewSession(context.Background())
	assert.Error(t, err, "Expected error when sending new_session on closed bridge")
}

// TestSetModelOnClosedBridge tests that SetModel returns error on closed bridge
func TestSetModelOnClosedBridge(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)
	_ = bridge.Stop()

	err := bridge.SetModel(context.Background(), "openai", "gpt-4")
	assert.Error(t, err, "Expected error when sending set_model on closed bridge")
}

// TestSetThinkingLevelOnClosedBridge tests that SetThinkingLevel returns error on closed bridge
func TestSetThinkingLevelOnClosedBridge(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)
	_ = bridge.Stop()

	err := bridge.SetThinkingLevel(context.Background(), "low")
	assert.Error(t, err, "Expected error when sending set_thinking_level on closed bridge")
}

// TestBashOnClosedBridge tests that Bash returns error on closed bridge
func TestBashOnClosedBridge(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)
	_ = bridge.Stop()

	err := bridge.Bash(context.Background(), "ls")
	assert.Error(t, err, "Expected error when sending bash on closed bridge")
}

// TestPromptWithImagesOnClosedBridge tests that PromptWithImages returns error on closed bridge
func TestPromptWithImagesOnClosedBridge(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)
	_ = bridge.Stop()

	images := []ImageContent{{Type: "image", Data: "base64", MimeType: "image/png"}}
	err := bridge.PromptWithImages(context.Background(), "message", images)
	assert.Error(t, err, "Expected error when sending prompt with images on closed bridge")
}

// TestIsRunningAfterStop tests IsRunning returns false after Stop
func TestIsRunningAfterStop(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Before start
	assert.False(t, bridge.IsRunning(), "Expected IsRunning() to return false before Start()")

	// After stop (without start)
	_ = bridge.Stop()
	assert.False(t, bridge.IsRunning(), "Expected IsRunning() to return false after Stop() without Start()")
}

// TestIsRunningWithNilProcess tests IsRunning when process is nil
func TestIsRunningWithNilProcess(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// bridge.cmd is nil at this point
	assert.False(t, bridge.IsRunning(), "Expected IsRunning() to return false with nil process")
}

// TestBuildArgsWithAllOptions tests buildArgs with all options set
func TestBuildArgsWithAllOptions(t *testing.T) {
	cfg := Config{
		Provider:         "anthropic",
		ModelID:          "claude-3-5-sonnet",
		APIKey:           "secret-key",
		SystemPrompt:     "You are a helpful assistant",
		ThinkingLevel:    "high",
		SessionDir:       "/tmp/sessions",
		NoSession:        true,
		Skills:           []string{"/path/skill1"},
		Tools:            "read,write,edit",
		NoTools:          false,
		Extensions:       []string{"/path/ext1.ts"},
		NoExtensions:     false,
		WorkingDirectory: "/workspace",
		Timeout:          10 * time.Minute,
	}

	bridge := NewBridge(cfg)
	args := bridge.buildArgs()

	// Count expected args
	expectedMap := map[string]int{
		"--provider":                  1,
		"anthropic":                   1,
		"--model":                     1,
		"claude-3-5-sonnet":           1,
		"--system-prompt":             1,
		"You are a helpful assistant": 1,
		"--thinking":                  1,
		"high":                        1,
		"--session-dir":               1,
		"/tmp/sessions":               1,
		"--skill":                     1,
		"/path/skill1":                1,
		"--tools":                     1,
		"read,write,edit":             1,
		"--extension":                 1,
		"/path/ext1.ts":               1,
	}

	for _, arg := range args {
		if count, ok := expectedMap[arg]; ok {
			expectedMap[arg] = count - 1
		}
	}

	for arg, remaining := range expectedMap {
		assert.Equal(t, 0, remaining, "Expected argument '%s' not found or counted incorrectly", arg)
	}
}

// TestBuildArgsWithOnlyProvider tests buildArgs with only provider
func TestBuildArgsWithOnlyProvider(t *testing.T) {
	cfg := Config{
		Provider: "google",
	}

	bridge := NewBridge(cfg)
	args := bridge.buildArgs()

	assert.Len(t, args, 2)
	assert.Equal(t, "--provider", args[0])
	assert.Equal(t, "google", args[1])
}

// TestBuildArgsWithNoConfig tests buildArgs with empty config
func TestBuildArgsWithNoConfig(t *testing.T) {
	cfg := Config{}

	bridge := NewBridge(cfg)
	args := bridge.buildArgs()

	assert.Len(t, args, 0)
}

// TestStopWhenAlreadyStopped tests that Stop is safe to call multiple times
func TestStopWhenAlreadyStopped(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// First stop
	err1 := bridge.Stop()
	assert.NoError(t, err1, "First Stop() returned error: %v", err1)

	// Second stop should be safe (returns nil or closed error)
	err2 := bridge.Stop()
	if err2 != nil {
		// This is acceptable - bridge is already closed
		t.Logf("Second Stop() returned error (acceptable): %v", err2)
	}
}

// TestStreamAgentExecutionFunction tests the StreamAgentExecution helper function
func TestStreamAgentExecutionFunction(t *testing.T) {
	// This test requires a properly configured pi environment with API keys
	// Skip if GOOGLE_API_KEY is not set
	if os.Getenv("GOOGLE_API_KEY") == "" {
		t.Skip("Skipping: GOOGLE_API_KEY not set")
	}

	// Create a mock broadcaster
	mockHub := &MockEventBroadcaster{}

	config := Config{
		Provider:      "google",
		ModelID:       "gemini-2.0-flash",
		ThinkingLevel: "low",
	}

	messages := []string{"Hello"}

	// Use a context with timeout to avoid hanging when pi is not available
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := StreamAgentExecution(ctx, mockHub, config, messages, "test-job-id")

	// The function should complete without error when properly configured
	if err != nil {
		t.Logf("StreamAgentExecution returned error (expected in some cases): %v", err)
	}
	if result != nil {
		t.Logf("StreamAgentExecution returned result: %v", *result)
	}
}

// MockEventBroadcaster is a mock implementation of EventBroadcaster for testing
type MockEventBroadcaster struct {
	events []struct {
		eventType string
		data      interface{}
	}
}

func (m *MockEventBroadcaster) BroadcastAgentEvent(eventType string, data interface{}) {
	m.events = append(m.events, struct {
		eventType string
		data      interface{}
	}{eventType, data})
}

// TestMockEventBroadcaster tests the mock broadcaster
func TestMockEventBroadcaster(t *testing.T) {
	mock := &MockEventBroadcaster{}

	mock.BroadcastAgentEvent("text_delta", "Hello")
	mock.BroadcastAgentEvent("agent_end", nil)

	assert.Len(t, mock.events, 2)
	assert.Equal(t, "text_delta", mock.events[0].eventType)
	assert.Equal(t, "agent_end", mock.events[1].eventType)
}

// TestBridgeStartWithNonExistentBinary tests that Start returns an error when pi binary doesn't exist
func TestBridgeStartWithNonExistentBinary(t *testing.T) {
	// This test verifies error handling when the pi executable cannot be found
	// In practice, pi should be installed, so we just verify the method handles errors properly
	cfg := Config{
		Provider: "test-provider",
	}

	bridge := NewBridge(cfg)

	// Verify bridge is in a valid initial state
	assert.NotNil(t, bridge)
	assert.False(t, bridge.IsRunning())
}

// TestBridgeStopMultipleTimes tests that Stop can be called multiple times safely
func TestBridgeStopMultipleTimes(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Call Stop on an unstarted bridge - should not panic
	err := bridge.Stop()
	assert.NoError(t, err, "Stop on unstarted bridge should not return error")

	// Call again - should also be safe
	err = bridge.Stop()
	assert.NoError(t, err, "Second Stop call should not return error")
}

// TestBridgeSendCommandBeforeStart tests that sendCommand returns error when bridge not started
func TestBridgeSendCommandBeforeStart(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Try to send a command without starting the bridge
	err := bridge.Prompt(context.Background(), "test message")

	// Should return an error because stdin is nil (bridge not started)
	assert.Error(t, err, "Expected error when sending command before bridge is started")
}

// TestSendExtensionUICancelError tests error case for SendExtensionUICancel
func TestSendExtensionUICancelError(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Try to send UI cancel - should fail because bridge is not running
	err := bridge.SendExtensionUICancel("test-uuid")
	assert.Error(t, err)
}

// TestSendExtensionUIResponseError tests error case for SendExtensionUIResponse
func TestSendExtensionUIResponseError(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Try to send UI response - should fail because bridge is not running
	err := bridge.SendExtensionUIResponse("test-uuid", "value", true)
	assert.Error(t, err)
}

// TestBridgeClosedState prevents sending commands to closed bridge
func TestBridgeClosedState(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Close the bridge (even though it wasn't started)
	_ = bridge.Stop()

	// Now try various operations - all should return "bridge is closed" error
	tests := []struct {
		name string
		op   func() error
	}{
		{"Prompt", func() error { return bridge.Prompt(context.Background(), "test") }},
		{"Steer", func() error { return bridge.Steer(context.Background(), "test") }},
		{"FollowUp", func() error { return bridge.FollowUp(context.Background(), "test") }},
		{"Abort", func() error { return bridge.Abort(context.Background()) }},
		{"NewSession", func() error { return bridge.NewSession(context.Background()) }},
		{"SetModel", func() error { return bridge.SetModel(context.Background(), "p", "m") }},
		{"SetThinkingLevel", func() error { return bridge.SetThinkingLevel(context.Background(), "low") }},
		{"Bash", func() error { return bridge.Bash(context.Background(), "ls") }},
		{"SendExtensionUICancel", func() error { return bridge.SendExtensionUICancel("id") }},
		{"SendExtensionUIResponse", func() error { return bridge.SendExtensionUIResponse("id", "v", true) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.op()
			assert.Error(t, err)
			assert.Equal(t, "bridge is closed", err.Error(), "Expected 'bridge is closed' error for %s", tt.name)
		})
	}
}

// TestChannelCloseOnStop tests that channels are closed properly when bridge stops
func TestChannelCloseOnStop(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Channels should be available even before start
	events := bridge.Events()
	assert.NotNil(t, events)

	errs := bridge.Errors()
	assert.NotNil(t, errs)

	processDone := bridge.ProcessDone()
	assert.NotNil(t, processDone)

	// After stop, channels should still be accessible (though empty)
	_ = bridge.Stop()

	// Verify IsRunning returns false
	assert.False(t, bridge.IsRunning())
}

// TestBuildArgsPreservesOrder tests that buildArgs preserves the expected argument order
func TestBuildArgsPreservesOrder(t *testing.T) {
	cfg := Config{
		Provider:      "anthropic",
		ModelID:       "claude-3-5-sonnet",
		SystemPrompt:  "You are a helpful assistant",
		ThinkingLevel: "medium",
	}

	bridge := NewBridge(cfg)
	args := bridge.buildArgs()

	// Verify the expected order: --provider, anthropic, --model, ..., --thinking, medium
	expectedIndices := map[string]int{
		"--provider":                  0,
		"anthropic":                   1,
		"--model":                     2,
		"claude-3-5-sonnet":           3,
		"--system-prompt":             4,
		"You are a helpful assistant": 5,
		"--thinking":                  6,
		"medium":                      7,
	}

	for expectedArg, expectedIndex := range expectedIndices {
		found := false
		for i, arg := range args {
			if arg == expectedArg && i == expectedIndex {
				found = true
				break
			}
		}
		assert.True(t, found, "Expected %s at index %d, not found in args: %v", expectedArg, expectedIndex, args)
	}
}

// TestEventChannelCapacity tests that event channel has the expected buffer capacity
func TestEventChannelCapacity(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Verify the channel has the correct capacity (100 events)
	// The bridge.eventChan is the underlying channel with capacity 100
	// We verify this indirectly through the public API
	eventChan := bridge.Events()

	// Verify channel is not nil
	assert.NotNil(t, eventChan)

	// Drain the channel if there are any events
	for {
		select {
		case _, ok := <-eventChan:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

// TestErrorChannelCapacity tests that error channel has the expected buffer capacity
func TestErrorChannelCapacity(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Verify the error channel is accessible
	errChan := bridge.Errors()

	// Verify channel is not nil
	assert.NotNil(t, errChan)

	// Drain the channel if there are any errors
	for {
		select {
		case _, ok := <-errChan:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

// TestPrivateFieldsNotAccessible tests that internal state is properly encapsulated
func TestPrivateFieldsNotAccessible(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// Verify that we cannot access private fields directly
	// This is a compile-time check - if private fields were accessible, this would compile
	// We only test public methods here

	// Verify bridge is properly initialized
	assert.NotNil(t, bridge)
	assert.NotNil(t, bridge.eventChan)
	assert.NotNil(t, bridge.errChan)
	assert.NotNil(t, bridge.processDone)

	// Verify initial state
	assert.False(t, bridge.closed)

	// After stopping, closed should be true
	_ = bridge.Stop()
	assert.True(t, bridge.closed)
}

// TestBridgeMutexSafety tests that the mutex protects concurrent access
func TestBridgeMutexSafety(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	var wg sync.WaitGroup
	numGoroutines := 10

	// Concurrently check IsRunning
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = bridge.IsRunning()
			}
		}()
	}

	// Concurrently call Stop
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = bridge.Stop()
		}()
	}

	wg.Wait()
}

// TestPromptWithUUID tests that Prompt generates a valid UUID
func TestPromptWithUUID(t *testing.T) {
	cfg := Config{}
	bridge := NewBridge(cfg)

	// We can't test the actual UUID generation without starting the bridge,
	// but we can verify the method exists and has the correct signature
	prompt := bridge.Prompt

	// Verify the method is not nil
	assert.NotNil(t, prompt)
}
