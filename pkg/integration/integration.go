package integration

import (
	"github.com/go-logr/logr"
	"github.com/mule-ai/mule/pkg/integration/discord"
	"github.com/mule-ai/mule/pkg/integration/matrix"
	"github.com/mule-ai/mule/pkg/integration/tasks"
)

type Settings struct {
	Matrix  *matrix.Config  `json:"matrix,omitempty"`
	Tasks   *tasks.Config   `json:"tasks,omitempty"`
	Discord *discord.Config `json:"discord,omitempty"`
}

type Integration interface {
	Call(name string, data any) (any, error)
	GetChannel() chan any
	Name() string
	Send(message any) error
	RegisterTrigger(trigger string, data any, channel chan any)
}

func LoadIntegrations(settings Settings, l logr.Logger) map[string]Integration {
	integrations := map[string]Integration{}

	if settings.Matrix != nil {
		integrations["matrix"] = matrix.New(settings.Matrix, l.WithName("matrix-integration"))
	}

	if settings.Discord != nil {
		integrations["discord"] = discord.New(settings.Discord, l.WithName("discord-integration"))
	}

	if settings.Tasks != nil {
		integrations["tasks"] = tasks.New(settings.Tasks, l.WithName("tasks-integration"))
	}

	return integrations
}
