package agent

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/jbutlerdev/genai"
)

func TestNewAgent(t *testing.T) {
	logger := logr.Discard()

	opts := AgentOptions{
		Provider:       &genai.Provider{}, // Replace with a mock provider if needed
		Name:           "TestAgent",
		Model:          "test-model",
		PromptTemplate: "test-template",
		Logger:         logger,
	}

	agent := NewAgent(opts)

	if agent == nil {
		t.Error("NewAgent returned nil")
		return
	}

	if agent.name != "TestAgent" {
		t.Errorf("Expected agent name to be 'TestAgent', got '%s'", agent.name)
	}
}
