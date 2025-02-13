package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/jbutlerdev/dev-team/internal/scheduler"
	"github.com/jbutlerdev/dev-team/internal/settings"
	"github.com/jbutlerdev/dev-team/internal/state"
	"github.com/jbutlerdev/dev-team/pkg/remote"
	"github.com/jbutlerdev/dev-team/pkg/repository"
)

const ConfigPath = ".config/dev-team/config.json"

type Config struct {
	Repositories map[string]*repository.Repository `json:"repositories"`
	Settings     settings.Settings                 `json:"settings"`
}

func LoadConfig(path string, l logr.Logger) (*state.AppState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Create default state if config doesn't exist
			appState := &state.AppState{
				Repositories: make(map[string]*repository.Repository),
				Settings: settings.Settings{
					Provider: "gemini",
				},
				Scheduler: scheduler.NewScheduler(l),
			}
			state.State = appState
			return appState, SaveConfig(path)
		}
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	// Create state from config
	appState := &state.AppState{
		Repositories: make(map[string]*repository.Repository),
		Settings:     config.Settings,
		Scheduler:    scheduler.NewScheduler(l),
	}

	// Set up repositories and their schedules
	for path, repo := range config.Repositories {
		rProviderOpts, err := remote.SettingsToOptions(repo.RemoteProvider)
		if err != nil {
			l.Error(err, "Error setting up remote provider", "path", path)
			continue
		}
		rProvider := remote.New(rProviderOpts)
		r := repository.NewRepositoryWithRemote(repo.Path, rProvider)
		r.Logger = l.WithName("repository").WithValues("path", repo.Path)
		r.Schedule = repo.Schedule
		r.RemotePath = repo.RemotePath
		r.RemoteProvider = repo.RemoteProvider
		err = r.UpdateStatus()
		if err != nil {
			l.Error(err, "Error getting repo status")
		}
		appState.Repositories[path] = r
		err = appState.Scheduler.AddTask(path, repo.Schedule, func() {
			err := r.Sync(state.State.GenAI, appState.Settings.GitHubToken)
			if err != nil {
				l.Error(err, "Error syncing repo")
			}
			appState.Mu.Lock()
			appState.Repositories[path] = r
			appState.Mu.Unlock()
		})
		if err != nil {
			l.Error(err, "Error setting up schedule for %s", path)
		}
	}

	return appState, nil
}

func SaveConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	state.State.Mu.RLock()
	defer state.State.Mu.RUnlock()

	// Create config from state
	config := Config{
		Repositories: make(map[string]*repository.Repository),
		Settings:     state.State.Settings,
	}

	for path, repo := range state.State.Repositories {
		config.Repositories[path] = repo
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}
