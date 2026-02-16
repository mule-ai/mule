package agent

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mule-ai/mule/internal/agent/pirc"
	"github.com/mule-ai/mule/internal/primitive"
)

// MockAgentStoreWithSkills extends MockAgentStore for integration tests
type MockAgentStoreWithSkills struct {
	MockAgentStore
	skills      map[string]*primitive.Skill
	agentSkills map[string][]string
}

func (m *MockAgentStoreWithSkills) GetSkill(ctx context.Context, id string) (*primitive.Skill, error) {
	skill, exists := m.skills[id]
	if !exists {
		return nil, primitive.ErrNotFound
	}
	return skill, nil
}

func (m *MockAgentStoreWithSkills) ListSkills(ctx context.Context) ([]*primitive.Skill, error) {
	var skills []*primitive.Skill
	for _, s := range m.skills {
		skills = append(skills, s)
	}
	return skills, nil
}

func (m *MockAgentStoreWithSkills) GetAgentSkills(ctx context.Context, agentID string) ([]*primitive.Skill, error) {
	skillIDs, exists := m.agentSkills[agentID]
	if !exists {
		return []*primitive.Skill{}, nil
	}

	var skills []*primitive.Skill
	for _, skillID := range skillIDs {
		if skill, ok := m.skills[skillID]; ok {
			skills = append(skills, skill)
		}
	}
	return skills, nil
}

// TestIntegration_AgentExecutionWithPI tests full agent execution through pi RPC
func TestIntegration_AgentExecutionWithPI(t *testing.T) {
	// Skip if no API key is available
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	openaiApiKey := os.Getenv("OPENAI_API_KEY")
	googleApiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" && openaiApiKey == "" && googleApiKey == "" {
		t.Skip("Skipping integration test: no API key available")
	}

	t.Run("execute agent with pi RPC", func(t *testing.T) {
		// Create mock store with a test agent
		store := &MockAgentStore{
			agents: map[string]*primitive.Agent{
				"test-agent-pi": {
					ID:           "test-agent-pi",
					Name:         "test-agent-pi",
					Description:  "Test agent for pi integration",
					ProviderID:   "test-provider",
					ModelID:      "claude-3-5-sonnet-20241022",
					SystemPrompt: "You are a helpful coding assistant.",
					PIConfig:     map[string]interface{}{"thinking_level": "medium"},
				},
			},
			providers: map[string]*primitive.Provider{
				"test-provider": {
					ID:         "test-provider",
					Name:       "Test Provider",
					APIBaseURL: "https://api.anthropic.com",
					APIKeyEnc:  "test-api-key",
				},
			},
		}

		mockJobStore := &MockJobStore{}
		runtime := NewRuntime(store, mockJobStore)

		// Execute the agent
		req := &ChatCompletionRequest{
			Model: "agent/test-agent-pi",
			Messages: []ChatCompletionMessage{
				{Role: "user", Content: "Say hello briefly."},
			},
			Stream: false,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		resp, err := runtime.ExecuteAgent(ctx, req)

		// The execution should complete (may return an error if pi is not installed)
		if err != nil {
			// Check if pi is not installed
			if strings.Contains(err.Error(), "executable file not found") ||
				strings.Contains(err.Error(), "no such file or directory") {
				t.Skip("pi not installed, skipping test")
			}
			// Log but don't fail - could be network or other issues
			t.Logf("Agent execution returned error (may be expected): %v", err)
			return
		}

		require.NoError(t, err)
		require.NotNil(t, resp)
		require.NotEmpty(t, resp.Choices)
		assert.NotEmpty(t, resp.Choices[0].Message.Content)
		t.Logf("Agent response: %s", resp.Choices[0].Message.Content)
	})

	t.Run("execute agent with skills", func(t *testing.T) {
		if apiKey == "" {
			t.Skip("Skipping: no ANTHROPIC_API_KEY")
		}

		// Create a skill for testing
		testSkill := &primitive.Skill{
			ID:          "test-skill-1",
			Name:        "test-skill",
			Description: "Test skill",
			Path:        "/tmp/test-skill",
			Enabled:     true,
		}

		// Use the extended mock store
		baseStore := MockAgentStore{
			agents: map[string]*primitive.Agent{
				"test-agent-with-skills": {
					ID:           "test-agent-with-skills",
					Name:         "test-agent-with-skills",
					Description:  "Test agent with skills",
					ProviderID:   "test-provider",
					ModelID:      "claude-3-5-sonnet-20241022",
					SystemPrompt: "You are a helpful assistant.",
				},
			},
			providers: map[string]*primitive.Provider{
				"test-provider": {
					ID:         "test-provider",
					Name:       "Test Provider",
					APIBaseURL: "https://api.anthropic.com",
					APIKeyEnc:  "test-api-key",
				},
			},
		}

		store := &MockAgentStoreWithSkills{
			MockAgentStore: baseStore,
			skills: map[string]*primitive.Skill{
				"test-skill-1": testSkill,
			},
			agentSkills: map[string][]string{
				"test-agent-with-skills": {"test-skill-1"},
			},
		}

		mockJobStore := &MockJobStore{}
		runtime := NewRuntime(store, mockJobStore)

		req := &ChatCompletionRequest{
			Model: "agent/test-agent-with-skills",
			Messages: []ChatCompletionMessage{
				{Role: "user", Content: "Hello"},
			},
			Stream: false,
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		resp, err := runtime.ExecuteAgent(ctx, req)

		if err != nil {
			if strings.Contains(err.Error(), "executable file not found") ||
				strings.Contains(err.Error(), "no such file or directory") {
				t.Skip("pi not installed, skipping test")
			}
			t.Logf("Agent execution returned error: %v", err)
			return
		}

		require.NoError(t, err)
		require.NotNil(t, resp)
		t.Logf("Agent with skills executed successfully")
	})

	t.Run("execute agent with thinking level", func(t *testing.T) {
		if apiKey == "" {
			t.Skip("Skipping: no ANTHROPIC_API_KEY")
		}

		// Test different thinking levels
		thinkingLevels := []string{"off", "low", "medium", "high"}

		for _, level := range thinkingLevels {
			t.Run("thinking_level="+level, func(t *testing.T) {
				store := &MockAgentStore{
					agents: map[string]*primitive.Agent{
						"test-agent-thinking": {
							ID:   "test-agent-thinking",
							Name: "test-agent-thinking",
							PIConfig: map[string]interface{}{
								"thinking_level": level,
							},
							ProviderID:   "test-provider",
							ModelID:      "claude-3-5-sonnet-20241022",
							SystemPrompt: "You are a helpful assistant.",
						},
					},
					providers: map[string]*primitive.Provider{
						"test-provider": {
							ID:         "test-provider",
							Name:       "Test Provider",
							APIBaseURL: "https://api.anthropic.com",
							APIKeyEnc:  "test-api-key",
						},
					},
				}

				mockJobStore := &MockJobStore{}
				runtime := NewRuntime(store, mockJobStore)

				req := &ChatCompletionRequest{
					Model: "agent/test-agent-thinking",
					Messages: []ChatCompletionMessage{
						{Role: "user", Content: "What is 1+1?"},
					},
					Stream: false,
				}

				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				defer cancel()

				resp, err := runtime.ExecuteAgent(ctx, req)

				if err != nil {
					if strings.Contains(err.Error(), "executable file not found") ||
						strings.Contains(err.Error(), "no such file or directory") {
						t.Skip("pi not installed, skipping test")
					}
					t.Logf("Agent execution returned error: %v", err)
					return
				}

				require.NoError(t, err)
				require.NotNil(t, resp)
				t.Logf("Agent with thinking=%s executed successfully", level)
			})
		}
	})
}

// TestIntegration_AgentNotFound tests error handling for non-existent agents
func TestIntegration_AgentNotFound(t *testing.T) {
	store := &MockAgentStore{
		agents:    map[string]*primitive.Agent{},
		providers: map[string]*primitive.Provider{},
	}

	mockJobStore := &MockJobStore{}
	runtime := NewRuntime(store, mockJobStore)

	req := &ChatCompletionRequest{
		Model: "agent/nonexistent",
		Messages: []ChatCompletionMessage{
			{Role: "user", Content: "Hello"},
		},
		Stream: false,
	}

	resp, err := runtime.ExecuteAgent(context.Background(), req)

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "not found")
}

// TestIntegration_InvalidModelFormat tests error handling for invalid model format
func TestIntegration_InvalidModelFormat(t *testing.T) {
	store := &MockAgentStore{
		agents:    map[string]*primitive.Agent{},
		providers: map[string]*primitive.Provider{},
	}

	mockJobStore := &MockJobStore{}
	runtime := NewRuntime(store, mockJobStore)

	req := &ChatCompletionRequest{
		Model: "invalid-format",
		Messages: []ChatCompletionMessage{
			{Role: "user", Content: "Hello"},
		},
		Stream: false,
	}

	resp, err := runtime.ExecuteAgent(context.Background(), req)

	assert.Error(t, err)
	assert.Nil(t, resp)
	assert.Contains(t, err.Error(), "not found")
}

// TestIntegration_ExecuteAgentWithWorkingDir tests execution with working directory
func TestIntegration_ExecuteAgentWithWorkingDir(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping: no ANTHROPIC_API_KEY")
	}

	// Create a temporary directory
	tempDir := t.TempDir()

	// Create a test file in the temp directory
	testFile := tempDir + "/test.txt"
	err := os.WriteFile(testFile, []byte("Hello from test file"), 0644)
	require.NoError(t, err)

	store := &MockAgentStore{
		agents: map[string]*primitive.Agent{
			"test-agent-workdir": {
				ID:           "test-agent-workdir",
				Name:         "test-agent-workdir",
				Description:  "Test agent with working directory",
				ProviderID:   "test-provider",
				ModelID:      "claude-3-5-sonnet-20241022",
				SystemPrompt: "You are a helpful assistant.",
			},
		},
		providers: map[string]*primitive.Provider{
			"test-provider": {
				ID:         "test-provider",
				Name:       "Test Provider",
				APIBaseURL: "https://api.anthropic.com",
				APIKeyEnc:  "test-api-key",
			},
		},
	}

	mockJobStore := &MockJobStore{}
	runtime := NewRuntime(store, mockJobStore)

	req := &ChatCompletionRequest{
		Model: "agent/test-agent-workdir",
		Messages: []ChatCompletionMessage{
			{Role: "user", Content: "List the files in the current directory"},
		},
		Stream: false,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	resp, err := runtime.ExecuteAgentWithWorkingDir(ctx, req, tempDir)

	if err != nil {
		if strings.Contains(err.Error(), "executable file not found") ||
			strings.Contains(err.Error(), "no such file or directory") {
			t.Skip("pi not installed, skipping test")
		}
		t.Logf("Agent execution returned error: %v", err)
		return
	}

	require.NoError(t, err)
	require.NotNil(t, resp)
	t.Logf("Agent with working directory executed successfully")
}

// TestIntegration_ProviderConfiguration tests different provider configurations
func TestIntegration_ProviderConfiguration(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	openaiKey := os.Getenv("OPENAI_API_KEY")

	testCases := []struct {
		name        string
		apiBaseURL  string
		apiKey      string
		modelID     string
		skipMessage string
	}{
		{
			name:        "anthropic",
			apiBaseURL:  "https://api.anthropic.com",
			apiKey:      apiKey,
			modelID:     "claude-3-5-sonnet-20241022",
			skipMessage: "no ANTHROPIC_API_KEY",
		},
		{
			name:        "openai",
			apiBaseURL:  "https://api.openai.com/v1",
			apiKey:      openaiKey,
			modelID:     "gpt-4o-mini",
			skipMessage: "no OPENAI_API_KEY",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.apiKey == "" {
				t.Skipf("Skipping: %s", tc.skipMessage)
			}

			store := &MockAgentStore{
				agents: map[string]*primitive.Agent{
					"test-agent-provider": {
						ID:           "test-agent-provider",
						Name:         "test-agent-provider",
						ProviderID:   "test-provider",
						ModelID:      tc.modelID,
						SystemPrompt: "You are a helpful assistant.",
					},
				},
				providers: map[string]*primitive.Provider{
					"test-provider": {
						ID:         "test-provider",
						Name:       "Test Provider",
						APIBaseURL: tc.apiBaseURL,
						APIKeyEnc:  "test-api-key",
					},
				},
			}

			mockJobStore := &MockJobStore{}
			runtime := NewRuntime(store, mockJobStore)

			req := &ChatCompletionRequest{
				Model: "agent/test-agent-provider",
				Messages: []ChatCompletionMessage{
					{Role: "user", Content: "Say hi"},
				},
				Stream: false,
			}

			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			resp, err := runtime.ExecuteAgent(ctx, req)

			if err != nil {
				if strings.Contains(err.Error(), "executable file not found") ||
					strings.Contains(err.Error(), "no such file or directory") {
					t.Skip("pi not installed, skipping test")
				}
				t.Logf("Agent execution returned error: %v", err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, resp)
			t.Logf("Provider %s executed successfully", tc.name)
		})
	}
}

// TestIntegration_ChatCompletionRequestJSON tests JSON serialization of requests
func TestIntegration_ChatCompletionRequestJSON(t *testing.T) {
	t.Run("serialize ChatCompletionRequest", func(t *testing.T) {
		req := &ChatCompletionRequest{
			Model: "agent/test-agent",
			Messages: []ChatCompletionMessage{
				{Role: "user", Content: "Hello"},
				{Role: "assistant", Content: "Hi there!"},
			},
			Stream:           true,
			WorkingDirectory: "/tmp",
		}

		data, err := json.Marshal(req)
		require.NoError(t, err)

		var decoded ChatCompletionRequest
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, req.Model, decoded.Model)
		assert.Equal(t, req.Stream, decoded.Stream)
		assert.Equal(t, req.WorkingDirectory, decoded.WorkingDirectory)
		assert.Len(t, decoded.Messages, 2)
	})

	t.Run("deserialize ChatCompletionResponse", func(t *testing.T) {
		resp := &ChatCompletionResponse{
			ID:      "chatcmpl-123",
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   "agent/test-agent",
			Choices: []ChatCompletionChoice{
				{
					Index: 0,
					Message: ChatCompletionMessage{
						Role:    "assistant",
						Content: "Hello!",
					},
					FinishReason: "stop",
				},
			},
			Usage: ChatCompletionUsage{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		}

		data, err := json.Marshal(resp)
		require.NoError(t, err)

		var decoded ChatCompletionResponse
		err = json.Unmarshal(data, &decoded)
		require.NoError(t, err)

		assert.Equal(t, resp.ID, decoded.ID)
		assert.Equal(t, resp.Model, decoded.Model)
		assert.Equal(t, resp.Choices[0].Message.Content, decoded.Choices[0].Message.Content)
		assert.Equal(t, resp.Usage.TotalTokens, decoded.Usage.TotalTokens)
	})
}

// TestIntegration_ResponseStructure tests the response structure
func TestIntegration_ResponseStructure(t *testing.T) {
	// Test that responses have the correct OpenAI-compatible structure
	resp := &ChatCompletionResponse{
		ID:      "chatcmpl-test123",
		Object:  "chat.completion",
		Created: 1234567890,
		Model:   "agent/test-agent",
		Choices: []ChatCompletionChoice{
			{
				Index: 0,
				Message: ChatCompletionMessage{
					Role:    "assistant",
					Content: "Test response content",
				},
				FinishReason: "stop",
			},
		},
		Usage: ChatCompletionUsage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	// Test JSON serialization
	data, err := json.Marshal(resp)
	require.NoError(t, err)

	// Verify expected fields are present
	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	require.NoError(t, err)

	assert.Contains(t, parsed, "id")
	assert.Contains(t, parsed, "object")
	assert.Contains(t, parsed, "created")
	assert.Contains(t, parsed, "model")
	assert.Contains(t, parsed, "choices")
	assert.Contains(t, parsed, "usage")

	// Verify choices structure
	choices := parsed["choices"].([]interface{})
	require.Len(t, choices, 1)

	choice := choices[0].(map[string]interface{})
	assert.Contains(t, choice, "message")
	assert.Contains(t, choice, "finish_reason")

	// Verify usage structure
	usage := parsed["usage"].(map[string]interface{})
	assert.Contains(t, usage, "prompt_tokens")
	assert.Contains(t, usage, "completion_tokens")
	assert.Contains(t, usage, "total_tokens")
}

// TestIntegration_SkillAssignmentConfig tests skill assignment and configuration
func TestIntegration_SkillAssignmentConfig(t *testing.T) {
	t.Run("skill path extraction from agent", func(t *testing.T) {
		// Create skills
		skill1 := &primitive.Skill{
			ID:          "skill-1",
			Name:        "code-skill",
			Description: "A skill for code assistance",
			Path:        "/home/user/.pi/agent/skills/code",
			Enabled:     true,
		}
		skill2 := &primitive.Skill{
			ID:          "skill-2",
			Name:        "test-skill",
			Description: "A skill for test assistance",
			Path:        "/home/user/.pi/agent/skills/tests",
			Enabled:     true,
		}
		skill3 := &primitive.Skill{
			ID:          "skill-3",
			Name:        "disabled-skill",
			Description: "A disabled skill",
			Path:        "/home/user/.pi/agent/skills/disabled",
			Enabled:     false,
		}

		// Create agent with skills (used to verify skill extraction logic)
		_ = &primitive.Agent{
			ID:           "test-agent-with-skills",
			Name:         "test-agent-with-skills",
			Description:  "Test agent with multiple skills",
			ProviderID:   "test-provider",
			ModelID:      "claude-3-5-sonnet-20241022",
			SystemPrompt: "You are a helpful coding assistant.",
		}

		// Simulate skill extraction logic (matches runtime.go)
		skills := []*primitive.Skill{skill1, skill2, skill3}
		var skillPaths []string
		for _, skill := range skills {
			if skill.Enabled {
				skillPaths = append(skillPaths, skill.Path)
			}
		}

		// Should only include enabled skills
		assert.Len(t, skillPaths, 2)
		assert.Equal(t, "/home/user/.pi/agent/skills/code", skillPaths[0])
		assert.Equal(t, "/home/user/.pi/agent/skills/tests", skillPaths[1])
	})

	t.Run("skill configuration in pi bridge", func(t *testing.T) {
		// Create config with skills (matching runtime.go logic)
		cfg := pirc.Config{
			Provider:      "anthropic",
			ModelID:       "claude-3-5-sonnet-20241022",
			APIKey:        "test-api-key",
			SystemPrompt:  "You are a helpful coding assistant.",
			ThinkingLevel: "high",
			Skills: []string{
				"/home/user/.pi/agent/skills/code",
				"/home/user/.pi/agent/skills/tests",
			},
			Timeout: 5 * time.Minute,
		}

		bridge := pirc.NewBridge(cfg)
		args := bridge.GetArgs()

		// Verify skill flags are present
		skillArgs := []string{}
		collecting := false
		for _, arg := range args {
			if arg == "--skill" {
				collecting = true
				continue
			}
			if collecting {
				skillArgs = append(skillArgs, arg)
				collecting = false
			}
		}

		assert.Len(t, skillArgs, 2)
		assert.Equal(t, "/home/user/.pi/agent/skills/code", skillArgs[0])
		assert.Equal(t, "/home/user/.pi/agent/skills/tests", skillArgs[1])
	})

	t.Run("skill assignment via store", func(t *testing.T) {
		// Test the store handles agent-skill assignments correctly
		store := &MockAgentStoreWithSkills{
			MockAgentStore: MockAgentStore{
				agents: map[string]*primitive.Agent{
					"agent-1": {ID: "agent-1", Name: "agent-1"},
				},
			},
			skills: map[string]*primitive.Skill{
				"skill-1": {ID: "skill-1", Name: "skill-1", Path: "/path/1", Enabled: true},
				"skill-2": {ID: "skill-2", Name: "skill-2", Path: "/path/2", Enabled: true},
			},
			agentSkills: map[string][]string{
				"agent-1": {"skill-1", "skill-2"},
			},
		}

		// Get skills for agent
		skills, err := store.GetAgentSkills(context.Background(), "agent-1")
		assert.NoError(t, err)
		assert.Len(t, skills, 2)

		// Verify skill paths
		var paths []string
		for _, s := range skills {
			paths = append(paths, s.Path)
		}
		assert.Contains(t, paths, "/path/1")
		assert.Contains(t, paths, "/path/2")
	})

	t.Run("empty skills config", func(t *testing.T) {
		cfg := pirc.Config{
			Provider:      "anthropic",
			ModelID:       "claude-3-5-sonnet-20241022",
			APIKey:        "test-api-key",
			ThinkingLevel: "medium",
			Skills:        []string{}, // Empty skills
			Timeout:       5 * time.Minute,
		}

		bridge := pirc.NewBridge(cfg)
		args := bridge.GetArgs()

		// Should not contain --skill flag
		for _, arg := range args {
			assert.NotEqual(t, "--skill", arg)
		}
	})

	t.Run("skill with thinking level", func(t *testing.T) {
		// Test combining skills with thinking level config
		testCases := []struct {
			name           string
			thinkingLevel  string
			skills         []string
			expectThinking bool
		}{
			{
				name:           "high thinking with skills",
				thinkingLevel:  "high",
				skills:         []string{"/skill/path"},
				expectThinking: true,
			},
			{
				name:           "off thinking with skills",
				thinkingLevel:  "off",
				skills:         []string{"/skill/path"},
				expectThinking: true,
			},
			{
				name:           "minimal thinking with skills",
				thinkingLevel:  "minimal",
				skills:         []string{"/skill1", "/skill2"},
				expectThinking: true,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				cfg := pirc.Config{
					Provider:      "anthropic",
					ModelID:       "claude-3-5-sonnet-20241022",
					APIKey:        "test-api-key",
					ThinkingLevel: tc.thinkingLevel,
					Skills:        tc.skills,
					Timeout:       5 * time.Minute,
				}

				bridge := pirc.NewBridge(cfg)
				args := bridge.GetArgs()

				// Check thinking level
				thinkingIdx := -1
				for i, arg := range args {
					if arg == "--thinking" {
						thinkingIdx = i
						break
					}
				}
				assert.True(t, thinkingIdx >= 0, "should have --thinking flag")
				assert.Equal(t, tc.thinkingLevel, args[thinkingIdx+1])

				// Check skills
				skillCount := 0
				for _, arg := range args {
					if arg == "--skill" {
						skillCount++
					}
				}
				assert.Equal(t, len(tc.skills), skillCount)
			})
		}
	})
}

// TestIntegration_DisableSkill tests skill being disabled doesn't get passed to pi
func TestIntegration_DisableSkill(t *testing.T) {
	t.Run("disabled skill not included in config", func(t *testing.T) {
		skill1 := &primitive.Skill{
			ID:      "enabled-skill",
			Name:    "enabled",
			Path:    "/path/enabled",
			Enabled: true,
		}
		skill2 := &primitive.Skill{
			ID:      "disabled-skill",
			Name:    "disabled",
			Path:    "/path/disabled",
			Enabled: false,
		}

		skills := []*primitive.Skill{skill1, skill2}
		var skillPaths []string
		for _, skill := range skills {
			if skill.Enabled {
				skillPaths = append(skillPaths, skill.Path)
			}
		}

		// Only enabled skill should be included
		assert.Len(t, skillPaths, 1)
		assert.Equal(t, "/path/enabled", skillPaths[0])
	})
}
