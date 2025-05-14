package integration

import (
	"github.com/go-logr/logr"
	"github.com/mule-ai/mule/pkg/integration/matrix"
)

type Settings struct {
	Matrix *matrix.Config `json:"matrix,omitempty"`
}

type Integration interface {
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

	return integrations
}
