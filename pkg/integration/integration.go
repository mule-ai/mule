package integration

import (
	"github.com/go-logr/logr"
	"github.com/jbutlerdev/genai"
	"github.com/mule-ai/mule/pkg/agent"
	"github.com/mule-ai/mule/pkg/integration/api"
	"github.com/mule-ai/mule/pkg/integration/discord"
	"github.com/mule-ai/mule/pkg/integration/grpc"
	"github.com/mule-ai/mule/pkg/integration/matrix"
	"github.com/mule-ai/mule/pkg/integration/memory"
	"github.com/mule-ai/mule/pkg/integration/system"
	"github.com/mule-ai/mule/pkg/integration/tasks"
	"github.com/mule-ai/mule/pkg/types"
)

type Settings struct {
	Matrix  map[string]*matrix.Config `json:"matrix,omitempty"`
	Tasks   *tasks.Config             `json:"tasks,omitempty"`
	Discord *discord.Config           `json:"discord,omitempty"`
	Memory  *memory.Config            `json:"memory,omitempty"`
	API     *api.Config               `json:"api,omitempty"`
	System  *system.Config            `json:"system,omitempty"`
	GRPC    *grpc.Config              `json:"grpc,omitempty"` // Generic config to avoid import cycles
}

type IntegrationInput struct {
	Settings  *Settings
	Providers map[string]*genai.Provider
	Agents    map[int]*agent.Agent
	Workflows map[string]*agent.Workflow
	Logger    logr.Logger
}

func LoadIntegrations(input IntegrationInput) map[string]types.Integration {
	integrations := map[string]types.Integration{}
	settings := input.Settings
	l := input.Logger
	providers := input.Providers

	// Initialize memory store if enabled
	var memoryManager *memory.Memory
	if settings.Memory != nil && settings.Memory.Enabled {
		store := memory.NewInMemoryStore(settings.Memory.MaxMessages)
		memoryManager = memory.New(settings.Memory, store)
		l.Info("Chat memory initialized", "max_messages", settings.Memory.MaxMessages)
	} else {
		// Create a default memory manager with minimal settings if not explicitly configured
		defaultConfig := memory.DefaultConfig()
		store := memory.NewInMemoryStore(defaultConfig.MaxMessages)
		memoryManager = memory.New(defaultConfig, store)
		l.Info("Default chat memory initialized", "max_messages", defaultConfig.MaxMessages)
	}

	if settings.Matrix != nil {
		for name, matrixConfig := range settings.Matrix {
			matrixLogger := l.WithName(name + "-matrix-integration")
			matrixInteg := matrix.New(name, matrixConfig, matrixLogger)

			// Wrap with memory support if matrix integration was created
			if matrixInteg != nil {
				matrixInteg.SetMemory(memoryManager)
				memoryManager.RegisterIntegration(name, name)
			}

			integrations[name] = matrixInteg
		}
	}

	if settings.Discord != nil {
		discordLogger := l.WithName("discord-integration")
		discordInteg := discord.New(settings.Discord, discordLogger)

		// Wrap with memory support if discord integration was created
		if discordInteg != nil {
			discordInteg.SetMemory(memoryManager)
			memoryManager.RegisterIntegration("discord", "discord")
		}

		integrations["discord"] = discordInteg
	}

	if settings.Tasks != nil {
		integrations["tasks"] = tasks.New(settings.Tasks, l.WithName("tasks-integration"))
	}

	if settings.API != nil {
		integrations["api"] = api.New(settings.API, l.WithName("api-integration"))
	}

	if settings.GRPC != nil {
		integrations["grpc"] = grpc.New(
			grpc.GRPCInput{
				Config:    settings.GRPC,
				Logger:    l.WithName("grpc-integration"),
				Agents:    input.Agents,
				Workflows: input.Workflows,
				Providers: providers,
			},
		)
	}

	// always start the system integration
	integrations["system"] = system.New(settings.System, providers, l.WithName("system-integration"))

	return integrations
}

func UpdateSystemPointers(integrations map[string]types.Integration, input IntegrationInput) map[string]types.Integration {
	newIntegrations := map[string]types.Integration{}
	if input.Workflows == nil {
		input.Workflows = map[string]*agent.Workflow{}
	}
	if input.Agents == nil {
		input.Agents = map[int]*agent.Agent{}
	}
	if input.Providers == nil {
		input.Providers = map[string]*genai.Provider{}
	}
	for name, integration := range integrations {
		switch name {
		case "grpc":
			i, ok := integration.(*grpc.GRPC)
			if !ok || i == nil {
				continue
			}
			i.SetSystemPointers(input.Agents, input.Workflows, input.Providers)
			newIntegrations[integration.Name()] = i
		default:
			newIntegrations[integration.Name()] = integration
		}
	}
	return newIntegrations
}
