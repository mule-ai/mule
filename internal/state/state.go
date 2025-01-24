package state

import (
	"dev-team/internal/scheduler"
	"dev-team/internal/settings"
	"dev-team/pkg/repository"
	"sync"
)

var State *AppState

type AppState struct {
	Repositories map[string]*repository.Repository `json:"repositories"`
	Settings     settings.Settings                 `json:"settings"`
	Scheduler    *scheduler.Scheduler
	Mu           sync.RWMutex
}
