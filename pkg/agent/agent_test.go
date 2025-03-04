package agent

import (
	"github.com/go-logr/stdr"
	"testing"
)

func NewTestAgent() *Agent {
	logger := stdr.New(nil)
	opts := AgentOptions{
		Name:         "test-agent",
		ProviderName: "test-provider",
		Model:        "test-model",
		Path:         "/some/path",
		Tools:        []string{"tool1"},
		Logger:       logger,
	}
	return NewAgent(opts)
}

func TestNewAgent(t *testing.T) {
	opts := AgentOptions{
		Name:         "test-agent",
		ProviderName: "test-provider",
		Model:        "test-model",
		Path:         "/some/path",
		Tools:        []string{"tool1"},
	}

	logger := stdr.New(nil)
	opts.Logger = logger

	agent := NewAgent(opts)
	if agent.Name() != opts.Name {
		t.Errorf("Expected name %s, got %s", opts.Name, agent.Name())
	}
	if agent.ProviderName() != opts.ProviderName {
		t.Errorf("Provider name mismatch")
	}
	if agent.Model() != opts.Model {
		t.Errorf("Model mismatch")
	}
}

func TestAgentStop(t *testing.T) {
	a := NewTestAgent()
	a.done = make(chan bool, 1)
	a.Stop()
	// Check done channel is closed
	select {
	case _, ok := <-a.done:
		if ok {
			t.Error("Done channel not closed")
		}
	default:
		t.Error("Done channel not closed")
	}
}

func TestAgent_Start(t *testing.T) {
	a := NewTestAgent()
	a.Start()
	//This test is intentionally left blank since Start() currently only logs
}
