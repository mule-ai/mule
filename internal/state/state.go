package state

import (
	"fmt"
	"os"
	"sync"

	"github.com/go-logr/logr"
	"github.com/jbutlerdev/genai"
	"github.com/mule-ai/mule/internal/scheduler"
	"github.com/mule-ai/mule/internal/settings"
	"github.com/mule-ai/mule/pkg/agent"
	"github.com/mule-ai/mule/pkg/integration"
	"github.com/mule-ai/mule/pkg/rag"
	"github.com/mule-ai/mule/pkg/remote"
	"github.com/mule-ai/mule/pkg/repository"
	"github.com/mule-ai/mule/pkg/types"
)

var State *AppState

type AppState struct {
	Repositories map[string]*repository.Repository `json:"repositories"`
	Settings     settings.Settings                 `json:"settings"`
	Scheduler    *scheduler.Scheduler
	Mu           sync.RWMutex
	Logger       logr.Logger
	GenAI        map[string]*genai.Provider
	Remote       *RemoteProviders
	Agents       map[int]*agent.Agent
	RAG          *rag.Store
	Workflows    map[string]*agent.Workflow
	Integrations map[string]types.Integration
}

type RemoteProviders struct {
	GitHub remote.Provider
	Local  remote.Provider
}

func NewState(logger logr.Logger, settings settings.Settings) *AppState {
	rag := rag.NewStore(logger.WithName("rag"))
	initializeEnvironmentVariables(settings.Environment)
	genaiProviders := initializeGenAIProviders(logger, settings)
	systemAgents := initializeSystemAgents(logger, settings, genaiProviders)
	agents := initializeAgents(logger, settings, genaiProviders, rag)
	agents = mergeAgents(agents, systemAgents)
	integrations := integration.LoadIntegrations(integration.IntegrationInput{
		Settings:  &settings.Integration,
		Providers: genaiProviders,
		Agents:    agents,
		Logger:    logger,
	})
	workflows := initializeWorkflows(settings, agents, logger, integrations)
	integrations = integration.UpdateSystemPointers(integrations, integration.IntegrationInput{
		Agents:    agents,
		Workflows: workflows,
		Providers: genaiProviders,
	})
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
		Agents:       agents,
		RAG:          rag,
		Workflows:    workflows,
		Integrations: integrations,
	}
}

func initializeEnvironmentVariables(environmentVariables []settings.EnvironmentVariable) {
	for _, environmentVariable := range environmentVariables {
		os.Setenv(environmentVariable.Name, environmentVariable.Value)
	}
}

func initializeGenAIProviders(logger logr.Logger, settings settings.Settings) map[string]*genai.Provider {
	providers := make(map[string]*genai.Provider)
	for _, providerConfig := range settings.AIProviders {
		genaiProvider, err := genai.NewProviderWithLog(providerConfig.Provider, genai.ProviderOptions{
			APIKey:  providerConfig.APIKey,
			BaseURL: providerConfig.Server,
			Name:    providerConfig.Name,
			Log:     logger.WithName(providerConfig.Provider),
		})
		if err != nil {
			logger.Error(err, "Error creating provider", "providerName", providerConfig.Name, "providerType", providerConfig.Provider)
			continue
		}
		if providerConfig.Name == "" {
			logger.Error(fmt.Errorf("provider name cannot be empty"), "Error initializing provider", "providerType", providerConfig.Provider)
			continue
		}
		providers[providerConfig.Name] = genaiProvider
	}
	return providers
}

func initializeAgents(logger logr.Logger, settingsInput settings.Settings, genaiProviders map[string]*genai.Provider, rag *rag.Store) map[int]*agent.Agent {
	agents := make(map[int]*agent.Agent)
	for _, agentOpts := range settingsInput.Agents {
		if provider, ok := genaiProviders[agentOpts.ProviderName]; ok {
			agentOpts.Provider = provider
		} else {
			logger.Error(fmt.Errorf("provider instance not found for name: %s", agentOpts.ProviderName), "provider not found")
			continue
		}
		agentOpts.Logger = logger.WithName("agent").WithValues("model", agentOpts.Model, "providerName", agentOpts.ProviderName)
		agentOpts.RAG = rag
		agents[agentOpts.ID] = agent.NewAgent(agentOpts)
	}
	return agents
}

func initializeSystemAgents(logger logr.Logger, settingsInput settings.Settings, genaiProviders map[string]*genai.Provider) map[int]*agent.Agent {
	agents := make(map[int]*agent.Agent)

	providerInstance, ok := genaiProviders[settingsInput.SystemAgent.ProviderName]
	if !ok {
		logger.Error(fmt.Errorf("system agent provider instance not found for name: %s", settingsInput.SystemAgent.ProviderName), "system agent provider not found")
	}

	systemAgentOptsBase := agent.AgentOptions{
		ProviderName: settingsInput.SystemAgent.ProviderName,
		Provider:     providerInstance,
		Model:        settingsInput.SystemAgent.Model,
		SystemPrompt: settingsInput.SystemAgent.SystemPrompt,
		Logger:       logger.WithName("system-agent").WithValues("providerName", settingsInput.SystemAgent.ProviderName),
	}

	commitAgentOpts := systemAgentOptsBase
	commitAgentOpts.PromptTemplate = settingsInput.SystemAgent.CommitTemplate
	agents[settings.CommitAgent] = agent.NewAgent(commitAgentOpts)

	prTitleAgentOpts := systemAgentOptsBase
	prTitleAgentOpts.PromptTemplate = settingsInput.SystemAgent.PRTitleTemplate
	agents[settings.PRTitleAgent] = agent.NewAgent(prTitleAgentOpts)

	prBodyAgentOpts := systemAgentOptsBase
	prBodyAgentOpts.PromptTemplate = settingsInput.SystemAgent.PRBodyTemplate
	agents[settings.PRBodyAgent] = agent.NewAgent(prBodyAgentOpts)

	return agents
}

func mergeAgents(agents map[int]*agent.Agent, systemAgents map[int]*agent.Agent) map[int]*agent.Agent {
	for id, agent := range systemAgents {
		agents[id] = agent
	}
	return agents
}

func initializeWorkflows(settingsInput settings.Settings, agents map[int]*agent.Agent, logger logr.Logger, integrations map[string]types.Integration) map[string]*agent.Workflow {
	workflows := make(map[string]*agent.Workflow)

	// First pass: create all workflows
	for _, workflow := range settingsInput.Workflows {
		workflows[workflow.Name] = agent.NewWorkflow(workflow, agents, integrations, logger.WithName("workflow").WithValues("name", workflow.Name))
		if workflow.IsDefault {
			workflows["default"] = workflows[workflow.Name]
		}
	}

	// Second pass: set workflow references for sub-workflow execution
	for _, workflow := range workflows {
		workflow.SetWorkflowReferences(workflows)
	}

	// Third pass: register triggers
	for _, workflow := range settingsInput.Workflows {
		err := workflows[workflow.Name].RegisterTriggers(integrations)
		if err != nil {
			logger.Error(err, "Error registering triggers for workflow", "workflowName", workflow.Name)
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
				workflow.Steps[i].AgentName = agent.Name
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

	workflows := initializeWorkflows(s.Settings, s.Agents, s.Logger, s.Integrations)
	s.Workflows = workflows

	// Update the scheduler with the new workflows.
	for repoPath, repo := range s.Repositories {
		s.Scheduler.RemoveTask(repoPath)

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
