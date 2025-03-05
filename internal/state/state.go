package state

import (
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"github.com/jbutlerdev/genai"
	"github.com/mule-ai/mule/internal/scheduler"
	"github.com/mule-ai/mule/internal/settings"
	"github.com/mule-ai/mule/pkg/agent"
	"github.com/mule-ai/mule/pkg/remote"
	"github.com/mule-ai/mule/pkg/repository"
)

var State *AppState

type AppState struct {
	Repositories map[string]*repository.Repository `json:"repositories"`
	Settings     settings.Settings                 `json:"settings"`
	Scheduler    *scheduler.Scheduler
	Mu           sync.RWMutex
	Logger       logr.Logger
	GenAI        *GenAIProviders
	Remote       *RemoteProviders
	Agents       map[int]*agent.Agent
	agentMap     map[string]*agent.Agent // Track agents by name
}

type GenAIProviders struct {
	Ollama *genai.Provider
	Gemini *genai.Provider
}

type RemoteProviders struct {
	GitHub remote.Provider
	Local  remote.Provider
}

func NewState(logger logr.Logger, settings settings.Settings) *AppState {
	genaiProviders := initializeGenAIProviders(logger, settings)
	systemAgents := initializeSystemAgents(logger, settings, genaiProviders)
	agents := initializeAgents(logger, settings, genaiProviders)
	agents = mergeAgents(agents, systemAgents)
	agentMap := make(map[string]*agent.Agent)
	for _, ag := range agents {
		agentMap[ag.Name()] = ag
	}
	return &AppState{
		Repositories: make(map[string]*repository.Repository),
		Settings:     settings,
		Scheduler:    scheduler.NewScheduler(logger.WithName("scheduler")),
		Logger:       logger,
		GenAI:        genaiProviders,
		Remote: &RemoteProviders{
			GitHub: remote.New(remote.ProviderOptions{
				Type:        remote.GITHUB,
				GitHubToken: settings.GitHubToken,
			}),
			Local: remote.New(remote.ProviderOptions{
				Type: remote.LOCAL,
				Path: "/",
			}),
		},
		Agents:   agents,
		agentMap: agentMap,
	}
}

func initializeGenAIProviders(logger logr.Logger, settings settings.Settings) *GenAIProviders {
	providers := &GenAIProviders{}
	for _, provider := range settings.AIProviders {
		genaiProvider, err := genai.NewProviderWithLog(provider.Provider, genai.ProviderOptions{
			APIKey:  provider.APIKey,
			BaseURL: provider.Server,
			Log:     logger.WithName(provider.Provider),
		})
		if err != nil {
			logger.Error(err, "Error creating provider")
			continue
		}
		switch provider.Provider {
		case genai.OLLAMA:
			providers.Ollama = genaiProvider
		case genai.GEMINI:
			providers.Gemini = genaiProvider
		}
	}
	return providers
}

func initializeAgents(logger logr.Logger, settingsInput settings.Settings, genaiProviders *GenAIProviders) map[int]*agent.Agent {
	agents := make(map[int]*agent.Agent)
	for i, agentOpts := range settingsInput.Agents {
		switch agentOpts.ProviderName {
		case genai.OLLAMA:
			if genaiProviders.Ollama == nil {
				logger.Error(fmt.Errorf("ollama provider not found"), "ollama provider not found")
				continue
			}
			agentOpts.Provider = genaiProviders.Ollama
		case genai.GEMINI:
			if genaiProviders.Gemini == nil {
				logger.Error(fmt.Errorf("gemini provider not found"), "gemini provider not found")
				continue
			}
			agentOpts.Provider = genaiProviders.Gemini
		default:
			logger.Error(fmt.Errorf("provider not found"), "provider not found")
			continue
		}
		agentOpts.Logger = logger.WithName("agent").WithValues("model", agentOpts.Model)
		agentInstance := agent.NewAgent(agentOpts)
		agents[settings.StartingAgent+i] = agentInstance
	}
	return agents
}

func initializeSystemAgents(logger logr.Logger, settingsInput settings.Settings, genaiProviders *GenAIProviders) map[int]*agent.Agent {
	agents := make(map[int]*agent.Agent)
	systemAgentOpts := agent.AgentOptions{
		ProviderName: settingsInput.SystemAgent.ProviderName,
		Model:        settingsInput.SystemAgent.Model,
		Logger:       logger.WithName("system-agent"),
	}
	switch settingsInput.SystemAgent.ProviderName {
	case genai.OLLAMA:
		systemAgentOpts.Provider = genaiProviders.Ollama
	case genai.GEMINI:
		systemAgentOpts.Provider = genaiProviders.Gemini
	}
	systemAgentOpts.PromptTemplate = settingsInput.SystemAgent.CommitTemplate
	agentInstance := agent.NewAgent(systemAgentOpts)
	agents[settings.CommitAgent] = agentInstance
	systemAgentOpts.PromptTemplate = settingsInput.SystemAgent.PRTitleTemplate
	agentInstance = agent.NewAgent(systemAgentOpts)
	agents[settings.PRTitleAgent] = agentInstance
	systemAgentOpts.PromptTemplate = settingsInput.SystemAgent.PRBodyTemplate
	agentInstance = agent.NewAgent(systemAgentOpts)
	agents[settings.PRBodyAgent] = agentInstance
	return agents
}

func mergeAgents(agents map[int]*agent.Agent, systemAgents map[int]*agent.Agent) map[int]*agent.Agent {
	for id, agent := range systemAgents {
		agents[id] = agent
	}
	return agents
}

func (s *AppState) UpdateState(newSettings *settings.Settings) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	// Create a map for quick lookup of new agents
	newAgentMap := make(map[string]agent.AgentOptions)
	for _, agentOpt := range newSettings.Agents {
		newAgentMap[agentOpt.Name] = agentOpt
	}

	// Remove agents that are present in the old settings but not in the new settings
	for name, existingAgent := range s.agentMap {
		_, existsInNew := newAgentMap[name]
		if !existsInNew {
			existingAgent.Stop()
			delete(s.agentMap, name)
		}
	}

	// Iterate through the new agent configurations and update or create agents
	newAgents := make(map[int]*agent.Agent)
	for i, newConfig := range newSettings.Agents {
		existingAgent, ok := s.agentMap[newConfig.Name]
		if ok {
			// Agent exists, check if config has changed
			if newConfig.ProviderName != existingAgent.ProviderName() || newConfig.Model != existingAgent.Model() {
				// Config has changed, restart agent
				existingAgent.Stop()
				delete(s.agentMap, newConfig.Name)

				// Initialize new agent instance with updated config
				newConfig.Logger = s.Logger.WithName("agent").WithValues("model", newConfig.Model)
				newAgent := agent.NewAgent(newConfig)
				s.agentMap[newConfig.Name] = newAgent
				newAgent.Start()
				newAgents[i] = newAgent
			} else {
				// Agent exists and config is the same, keep the existing agent
				newAgents[i] = existingAgent
			}
		} else {
			// Agent doesn't exist, create new agent
			newConfig.Logger = s.Logger.WithName("agent").WithValues("model", newConfig.Model)
			newAgent := agent.NewAgent(newConfig)
			s.agentMap[newConfig.Name] = newAgent
			newAgent.Start()
			newAgents[i] = newAgent
		}
	}

	// Update the main agents map
	s.Agents = newAgents

	s.Settings = *newSettings
	return nil
}
