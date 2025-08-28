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
	"github.com/mule-ai/mule/pkg/integration/rss"
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
	RSS     *rss.Config               `json:"rss,omitempty"`
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

	// Initialize memory stores
	var memoryManagerInMemory *memory.Memory // For Discord and other integrations (deprecated)
	var memoryManagerChromeM *memory.Memory  // For Matrix integration (new ChromeM-based)

	// Initialize ChromeM memory for Matrix
	if settings.Memory != nil && settings.Memory.Enabled {
		// Create ChromeM-based memory for Matrix
		chromeMPath := "/tmp/mule_memory.db" // TODO: Make this configurable
		chromeMStore, err := memory.NewChromeMStoreWithEmbedding(chromeMPath, settings.Memory.MaxMessages, memory.NewLocalEmbeddingFunc())
		if err != nil {
			l.Error(err, "Failed to create ChromeM store, falling back to in-memory")
			chromeMStore = nil
		}
		if chromeMStore != nil {
			memoryManagerChromeM = memory.New(settings.Memory, chromeMStore)
			l.Info("ChromeM memory initialized for Matrix", "path", chromeMPath)
		}

		// Keep in-memory for other integrations (deprecated)
		store := memory.NewInMemoryStore(settings.Memory.MaxMessages)
		memoryManagerInMemory = memory.New(settings.Memory, store)
		l.Info("In-memory store initialized for legacy integrations", "max_messages", settings.Memory.MaxMessages)
	} else {
		// Create default managers
		defaultConfig := memory.DefaultConfig()

		// ChromeM for Matrix
		chromeMPath := "/tmp/mule_memory.db"
		chromeMStore, _ := memory.NewChromeMStoreWithEmbedding(chromeMPath, defaultConfig.MaxMessages, memory.NewLocalEmbeddingFunc())
		if chromeMStore != nil {
			memoryManagerChromeM = memory.New(defaultConfig, chromeMStore)
		}

		// In-memory for others
		store := memory.NewInMemoryStore(defaultConfig.MaxMessages)
		memoryManagerInMemory = memory.New(defaultConfig, store)
		l.Info("Default memory managers initialized")
	}

	if settings.Matrix != nil {
		for name, matrixConfig := range settings.Matrix {
			matrixLogger := l.WithName(name + "-matrix-integration")
			matrixInteg := matrix.New(name, matrixConfig, matrixLogger)

			// Use ChromeM memory for Matrix
			if matrixInteg != nil && memoryManagerChromeM != nil {
				matrixInteg.SetMemory(memoryManagerChromeM)
				memoryManagerChromeM.RegisterIntegration(name, name)
			}

			integrations[name] = matrixInteg
		}
	}

	if settings.Discord != nil {
		discordLogger := l.WithName("discord-integration")
		discordInteg := discord.New(settings.Discord, discordLogger)

		// Use deprecated in-memory for Discord (for now)
		if discordInteg != nil && memoryManagerInMemory != nil {
			discordInteg.SetMemory(memoryManagerInMemory)
			memoryManagerInMemory.RegisterIntegration("discord", "discord")
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

	// RSS integration
	if settings.RSS != nil {
		rssInteg := rss.New(settings.RSS, l.WithName("rss-integration"))
		integrations["rss"] = rssInteg

		// If Discord is also enabled, connect them
		if settings.Discord != nil && integrations["discord"] != nil {
			discordInteg, ok := integrations["discord"].(*discord.Discord)
			if ok {
				// Connect Discord messages to RSS feed
				discordInteg.SetRSSIntegration(rssInteg.GetChannel())
				l.Info("Connected Discord to RSS integration")
			}
		}
	}

	// always start the system integration
	integrations["system"] = system.New(settings.System, providers, l.WithName("system-integration"))

	// Add workflow memory integration (new ChromeM-based)
	workflowMemoryConfig := &memory.WorkflowMemoryConfig{
		Enabled:           true,
		DBPath:            "/tmp/mule_workflow_memory.db",
		MaxMessages:       100,
		UseLocalEmbedding: true, // Use local embeddings to avoid API dependency
	}
	workflowMemory, err := memory.NewWorkflowMemoryIntegration("workflow-memory", workflowMemoryConfig, l.WithName("workflow-memory"))
	if err != nil {
		l.Error(err, "Failed to create workflow memory integration")
	} else {
		integrations["workflow-memory"] = workflowMemory
		l.Info("Workflow memory integration initialized with ChromeM", "dbPath", workflowMemoryConfig.DBPath)
	}

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
