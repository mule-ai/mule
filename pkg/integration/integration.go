package integration

import (
	"github.com/go-logr/logr"
	"github.com/jbutlerdev/genai"
	"github.com/mule-ai/mule/pkg/integration/api"
	"github.com/mule-ai/mule/pkg/integration/discord"
	"github.com/mule-ai/mule/pkg/integration/matrix"
	"github.com/mule-ai/mule/pkg/integration/memory"
	"github.com/mule-ai/mule/pkg/integration/system"
	"github.com/mule-ai/mule/pkg/integration/tasks"
)

type Settings struct {
	Matrix  *matrix.Config  `json:"matrix,omitempty"`
	Tasks   *tasks.Config   `json:"tasks,omitempty"`
	Discord *discord.Config `json:"discord,omitempty"`
	Memory  *memory.Config  `json:"memory,omitempty"`
	API     *api.Config     `json:"api,omitempty"`
	System  *system.Config  `json:"system,omitempty"`
}

type IntegrationInput struct {
	Settings  *Settings
	Providers map[string]*genai.Provider
	Logger    logr.Logger
}

type Integration interface {
	Call(name string, data any) (any, error)
	GetChannel() chan any
	Name() string
	RegisterTrigger(trigger string, data any, channel chan any)

	// Chat memory methods
	GetChatHistory(channelID string, limit int) (string, error)
	ClearChatHistory(channelID string) error
}

func LoadIntegrations(input IntegrationInput) map[string]Integration {
	integrations := map[string]Integration{}
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
		matrixLogger := l.WithName("matrix-integration")
		matrixInteg := matrix.New(settings.Matrix, matrixLogger)

		// Wrap with memory support if matrix integration was created
		if matrixInteg != nil {
			matrixInteg.SetMemory(memoryManager)
			memoryManager.RegisterIntegration("matrix", "matrix")
		}

		integrations["matrix"] = matrixInteg
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

	// always start the system integration
	integrations["system"] = system.New(settings.System, providers, l.WithName("system-integration"))

	return integrations
}
