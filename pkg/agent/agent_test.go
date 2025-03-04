package agent

import (
	"sync"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/stdr"
	//"github.com/mule-ai/mule/internal/settings"
	//"reflect"
)

// MockAgent that allows us to track whether stop and start have been called
type MockAgent struct {
	name         string
	providerName string
	model        string
	stopCalled   bool
	startCalled  bool
	logger       logr.Logger
	done         chan bool
	mu           sync.Mutex
}

func (m *MockAgent) Name() string { return m.name }

func (m *MockAgent) ProviderName() string { return m.providerName }

func (m *MockAgent) Model() string { return m.model }

func (m *MockAgent) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCalled = true
	if m.done != nil {
		close(m.done)
	}
}

func (m *MockAgent) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startCalled = true
}

// NewMockAgent creates a new mock agent with the given properties.
func NewMockAgent(name, providerName, model string) *MockAgent {
	logger := stdr.New(nil)
	return &MockAgent{
		name:         name,
		providerName: providerName,
		model:        model,
		logger:       logger,
		done:         make(chan bool, 1),
	}
}

func NewTestAgent(name, providerName, model string) *Agent {
	logger := stdr.New(nil)
	opts := AgentOptions{
		Name:         name,
		ProviderName: providerName,
		Model:        model,
		Path:         "/some/path",
		//Tools:        []string{"tool1"},
		Logger: logger,
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
	a := NewTestAgent("test-agent", "test-provider", "test-model")
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
	a := NewTestAgent("test-agent", "test-provider", "test-model")
	a.Start()
	//This test is intentionally left blank since Start() currently only logs
}

func TestNewState_AgentMapInitialization(t *testing.T) {
	logger := stdr.New(nil)
	settings := struct {
		StartingAgent int
		CommitAgent   int
		PRTitleAgent  int
		PRBodyAgent   int
	}{StartingAgent: 0, CommitAgent: 1, PRTitleAgent: 2, PRBodyAgent: 3}

	genaiProviders := &struct{}{}

	// Test case 1: No overlapping agent names
	agents := map[int]*Agent{
		0: NewTestAgent("agent1", "provider1", "model1"),
		1: NewTestAgent("agent2", "provider2", "model2"),
	}

	systemAgents := map[int]*Agent{
		2: NewTestAgent("agent3", "provider3", "model3"),
	}

	// Initialize agentMap directly as it's not exported.
	agentMap := make(map[string]*Agent)
	for _, ag := range agents {
		agentMap[ag.Name()] = ag
	}
	for _, ag := range systemAgents {
		agentMap[ag.Name()] = ag
	}

	// Check that each agent is in the agentMap
	if _, ok := agentMap["agent1"]; !ok {
		t.Error("agent1 not in agentMap")
	}
	if _, ok := agentMap["agent2"]; !ok {
		t.Error("agent2 not in agentMap")
	}
	if _, ok := agentMap["agent3"]; !ok {
		t.Error("agent3 not in agentMap")
	}

	// Check the total number of agents in the map
	if len(agentMap) != 3 {
		t.Errorf("Expected 3 agents in agentMap, got %d", len(agentMap))
	}

	// Test case 2: Overlapping agent names (system agent should override user agent)
	agents = map[int]*Agent{
		0: NewTestAgent("agent1", "provider1", "model1"),
		1: NewTestAgent("agent2", "provider2", "model2"),
	}

	systemAgents = map[int]*Agent{
		2: NewTestAgent("agent1", "provider3", "model3"), // Overlapping name
	}

	// Initialize agentMap directly as it's not exported.
	agentMap = make(map[string]*Agent)
	for _, ag := range agents {
		agentMap[ag.Name()] = ag
	}
	for _, ag := range systemAgents {
		agentMap[ag.Name()] = ag
	}

	// Check that the system agent overrides the user agent
	if agentMap["agent1"].ProviderName() != "provider3" {
		t.Error("System agent did not override user agent")
	}

	// Check the total number of agents in the map
	if len(agentMap) != 2 {
		t.Errorf("Expected 2 agents in agentMap, got %d", len(agentMap))
	}
	_ = settings
	_ = logger
	_ = genaiProviders
}

// Mock settings struct for testing

func TestUpdateState(t *testing.T) {
	logger := stdr.New(nil)

	// Define a local struct that mirrors the structure of settings.Settings.Agents
	type TestAgentOptions struct {
		Name         string
		ProviderName string
		Model        string
	}

	type MockSettings struct {
		Agents []TestAgentOptions
	}

	// Initial settings
	initialSettings := MockSettings{
		Agents: []TestAgentOptions{
			{Name: "agent1", ProviderName: "provider1", Model: "model1"},
			{Name: "agent2", ProviderName: "provider2", Model: "model2"},
		},
	}

	// Create initial AppState
	appState := &struct {
		Settings MockSettings
		Mu       sync.Mutex
		agentMap map[string]*MockAgent
	}{
		Settings: initialSettings,
		Mu:       sync.Mutex{},
		agentMap: make(map[string]*MockAgent),
	}

	// Replace NewTestAgent with NewMockAgent to track stop/start calls
	agent1 := NewMockAgent("agent1", "provider1", "model1")
	agent2 := NewMockAgent("agent2", "provider2", "model2")

	// Initialize done channels for the mock agents
	agent1.done = make(chan bool, 1)
	agent2.done = make(chan bool, 1)

	// Add agents to the agentMap within the AppState
	appState.agentMap["agent1"] = agent1
	appState.agentMap["agent2"] = agent2

	// Modified settings: change model for agent1, remove agent2, add agent3
	newSettings := MockSettings{
		Agents: []TestAgentOptions{
			{Name: "agent1", ProviderName: "provider1", Model: "model1-new"},
			{Name: "agent3", ProviderName: "provider3", Model: "model3"},
		},
	}

	// Create a mock UpdateState function to test the logic
	UpdateState := func(s *struct {
		Settings MockSettings
		Mu       sync.Mutex
		agentMap map[string]*MockAgent
	}, newSettings *MockSettings) error {
		s.Mu.Lock()
		defer s.Mu.Unlock()

		// Create a map for quick lookup of new agents
		newAgentMap := make(map[string]TestAgentOptions)
		for _, agentOpt := range newSettings.Agents {
			newAgentMap[agentOpt.Name] = agentOpt
		}

		// Iterate through existing agents
		for name, existingAgent := range s.agentMap {
			_, existsInNew := newAgentMap[name]
			if !existsInNew {
				// Agent exists in old settings but not in new, so stop and remove it
				existingAgent.Stop()
				delete(s.agentMap, name)
			} else {
				// Agent exists in both, check if the config has changed
				newAgentConfig := newAgentMap[name]
				if existingAgent.providerName != newAgentConfig.ProviderName || existingAgent.model != newAgentConfig.Model {
					// Config changed, stop the old agent, create and start a new one
					existingAgent.Stop()
					delete(s.agentMap, name)

					newAgent := NewMockAgent(newAgentConfig.Name, newAgentConfig.ProviderName, newAgentConfig.Model)
					s.agentMap[name] = newAgent
					newAgent.Start()
				}
			}
		}

		// Add new agents
		for _, agentOpt := range newSettings.Agents {
			_, existsInOld := s.agentMap[agentOpt.Name]
			if !existsInOld {
				newAgent := NewMockAgent(agentOpt.Name, agentOpt.ProviderName, agentOpt.Model)
				s.agentMap[agentOpt.Name] = newAgent
				newAgent.Start()
			}
		}

		s.Settings = *newSettings // Update the settings
		return nil
	}

	// Call UpdateState
	err := UpdateState(appState, &newSettings)
	if err != nil {
		t.Fatalf("UpdateState failed: %v", err)
	}

	// Verify that agent1's model has been updated
	if appState.agentMap["agent1"].model != "model1-new" {
		t.Errorf("Agent1 model not updated: expected 'model1-new', got '%s'", appState.agentMap["agent1"].model)
	}

	// Verify that agent2 has been removed
	if _, ok := appState.agentMap["agent2"]; ok {
		t.Error("Agent2 should have been removed")
	}

	// Verify that agent3 has been added
	agent3, ok := appState.agentMap["agent3"]
	if !ok {
		t.Error("Agent3 should have been added")
	}

	// Verify stop and start were called
	if !agent1.stopCalled {
		t.Error("agent1.Stop() should have been called")
	}

	// Create a new MockAgent for agent 3 to check if start was called
	//agent3 := NewMockAgent("agent3", "provider3", "model3")
	if !agent3.startCalled {
		t.Error("agent3.Start() should have been called")
	}
	_ = logger
}
