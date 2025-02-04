package state

import (
	"sync"

	"github.com/jbutlerdev/dev-team/internal/scheduler"
	"github.com/jbutlerdev/dev-team/internal/settings"
	"github.com/jbutlerdev/dev-team/pkg/repository"
	"github.com/jbutlerdev/genai"
)

var State *AppState

type AppState struct {
	Repositories map[string]*repository.Repository `json:"repositories"`
	Settings     settings.Settings                 `json:"settings"`
	Scheduler    *scheduler.Scheduler
	Mu           sync.RWMutex
	GenAI        *genai.Provider
}
