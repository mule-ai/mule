package state

import (
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"github.com/jbutlerdev/genai"
	"github.com/mule-ai/mule/internal/scheduler"
	"github.com/mule-ai/mule/internal/settings"
	"github.com/mule-ai/mule/pkg/agent"
	"github.com/mule-ai/mule/pkg/rag"
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
	RAG          *rag.Store
	Workflows    map[string]struct {
		Steps               []agent.WorkflowStep
		ValidationFunctions []string
	}
}

type GenAIProviders struct {
	Ollama *genai.Provider
	Gemini *genai.Provider
	OpenAI *genai.Provider
}

type RemoteProviders struct {
	GitHub remote.Provider
	Local  remote.Provider
}

func NewState(logger logr.Logger, settings settings.Settings) *AppState {
	rag := rag.NewStore(logger.WithName("rag"))
	genaiProviders := initializeGenAIProviders(logger, settings)
	systemAgents := initializeSystemAgents(logger, settings, genaiProviders)
	agents := initializeAgents(logger, settings, genaiProviders, rag)
	agents = mergeAgents(agents, systemAgents)
	workflows := initializeWorkflows(settings)
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
		Agents:    agents,
		RAG:       rag,
		Workflows: workflows,
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
		case genai.OPENAI:
			providers.OpenAI = genaiProvider
		}
	}
	return providers
}

func initializeAgents(logger logr.Logger, settingsInput settings.Settings, genaiProviders *GenAIProviders, rag *rag.Store) map[int]*agent.Agent {
	agents := make(map[int]*agent.Agent)
	for _, agentOpts := range settingsInput.Agents {
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
		case genai.OPENAI:
			if genaiProviders.OpenAI == nil {
				logger.Error(fmt.Errorf("openai provider not found"), "openai provider not found")
				continue
			}
			agentOpts.Provider = genaiProviders.OpenAI
		default:
			logger.Error(fmt.Errorf("provider not found"), "provider not found")
			continue
		}
		agentOpts.Logger = logger.WithName("agent").WithValues("model", agentOpts.Model)
		agentOpts.RAG = rag
		agents[agentOpts.ID] = agent.NewAgent(agentOpts)
	}
	return agents
}

func initializeSystemAgents(logger logr.Logger, settingsInput settings.Settings, genaiProviders *GenAIProviders) map[int]*agent.Agent {
	agents := make(map[int]*agent.Agent)
	systemAgentOpts := agent.AgentOptions{
		ProviderName: settingsInput.SystemAgent.ProviderName,
		Model:        settingsInput.SystemAgent.Model,
		SystemPrompt: settingsInput.SystemAgent.SystemPrompt,
		Logger:       logger.WithName("system-agent"),
	}
	switch settingsInput.SystemAgent.ProviderName {
	case genai.OLLAMA:
		systemAgentOpts.Provider = genaiProviders.Ollama
	case genai.GEMINI:
		systemAgentOpts.Provider = genaiProviders.Gemini
	case genai.OPENAI:
		systemAgentOpts.Provider = genaiProviders.OpenAI
	}
	systemAgentOpts.PromptTemplate = settingsInput.SystemAgent.CommitTemplate
	agents[settings.CommitAgent] = agent.NewAgent(systemAgentOpts)
	systemAgentOpts.PromptTemplate = settingsInput.SystemAgent.PRTitleTemplate
	agents[settings.PRTitleAgent] = agent.NewAgent(systemAgentOpts)
	systemAgentOpts.PromptTemplate = settingsInput.SystemAgent.PRBodyTemplate
	agents[settings.PRBodyAgent] = agent.NewAgent(systemAgentOpts)
	return agents
}

func mergeAgents(agents map[int]*agent.Agent, systemAgents map[int]*agent.Agent) map[int]*agent.Agent {
	for id, agent := range systemAgents {
		agents[id] = agent
	}
	return agents
}

func initializeWorkflows(settingsInput settings.Settings) map[string]struct {
	Steps               []agent.WorkflowStep
	ValidationFunctions []string
} {
	workflows := make(map[string]struct {
		Steps               []agent.WorkflowStep
		ValidationFunctions []string
	})

	for _, workflow := range settingsInput.Workflows {
		workflows[workflow.Name] = struct {
			Steps               []agent.WorkflowStep
			ValidationFunctions []string
		}{
			Steps:               workflow.Steps,
			ValidationFunctions: workflow.ValidationFunctions,
		}

		if workflow.IsDefault {
			workflows["default"] = struct {
				Steps               []agent.WorkflowStep
				ValidationFunctions []string
			}{
				Steps:               workflow.Steps,
				ValidationFunctions: workflow.ValidationFunctions,
			}
		}
	}
	return workflows
}

// UpdateAgents re-initializes the agents based on the new settings.
func (s *AppState) UpdateAgents() error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	// Re-initialize GenAI providers
	genaiProviders := initializeGenAIProviders(s.Logger, s.Settings)

	// Re-initialize system agents
	systemAgents := initializeSystemAgents(s.Logger, s.Settings, genaiProviders)

	// Re-initialize agents
	agents := initializeAgents(s.Logger, s.Settings, genaiProviders, s.RAG)

	// Merge agents
	agents = mergeAgents(agents, systemAgents)

	// Update the AppState's agents
	s.Agents = agents

	// Update any references to agents in workflows or other parts of the application.
	for workflowName, workflow := range s.Workflows {
		for i, step := range workflow.Steps {
			if agent, ok := s.Agents[step.AgentID]; ok {
				workflow.Steps[i].AgentName = agent.Name // Keep agent name updated
			} else {
				s.Logger.Error(fmt.Errorf("agent not found"), "agent not found", "agentID", step.AgentID)
			}
		}
		s.Workflows[workflowName] = workflow
	}
	return nil
}

// UpdateWorkflows re-initializes the workflows based on the new settings.
func (s *AppState) UpdateWorkflows() error {
	s.Mu.Lock()
	defer s.Mu.Unlock()

	workflows := initializeWorkflows(s.Settings)
	s.Workflows = workflows

	// Update the scheduler with the new workflows.
	for repoPath, repo := range s.Repositories {
		// Remove the existing task.
		s.Scheduler.RemoveTask(repoPath)

		// Add a new task with the updated schedule and workflow.
		defaultWorkflow := s.Workflows["default"]
		err := s.Scheduler.AddTask(repoPath, repo.Schedule, func() {
			err := repo.Sync(s.Agents, defaultWorkflow)
			if err != nil {
				s.Logger.Error(err, "Error syncing repo")
			}
		})
		if err != nil {
			s.Logger.Error(err, "Error adding task to scheduler", "repoPath", repoPath)
		}
	}
	return nil
}

// ReloadSettings reloads settings, agents and workflows
func (s *AppState) ReloadSettings(newSettings settings.Settings) error {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.Settings = newSettings

	if err := s.UpdateAgents(); err != nil {
		return err
	}

	if err := s.UpdateWorkflows(); err != nil {
		return err
	}

	return nil
}
