package state

import (
	"fmt"
	"sync"

	"github.com/go-logr/logr"
	"github.com/jbutlerdev/dev-team/internal/scheduler"
	"github.com/jbutlerdev/dev-team/internal/settings"
	"github.com/jbutlerdev/dev-team/pkg/agent"
	"github.com/jbutlerdev/dev-team/pkg/remote"
	"github.com/jbutlerdev/dev-team/pkg/repository"
	"github.com/jbutlerdev/genai"
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
	Agents       []*agent.Agent
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
	agents := initializeAgents(logger, settings, genaiProviders)
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
			}),
		},
		Agents: agents,
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

func initializeAgents(logger logr.Logger, settings settings.Settings, genaiProviders *GenAIProviders) []*agent.Agent {
	agents := []*agent.Agent{}
	for _, agentOpts := range settings.Agents {
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
		agents = append(agents, agent.NewAgent(agentOpts))
	}
	return agents
}

/*
	Tools:               []string{"writeFile", "tree", "readFile"},
	ValidationFunctions: []string{"getDeps", "goFmt", "goModTidy", "golangciLint", "goTest"},
*/
