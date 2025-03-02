package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/mule-ai/mule/internal/scheduler"
	"github.com/mule-ai/mule/internal/settings"
	"github.com/mule-ai/mule/internal/state"
	"github.com/mule-ai/mule/pkg/remote"
	"github.com/mule-ai/mule/pkg/repository"
)

const ConfigPath = ".config/mule/config.json"

type Config struct {
	Repositories map[string]*repository.Repository `json:"repositories"`
	Settings     settings.Settings                 `json:"settings"`
}

func GetHomeConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ConfigPath), nil
}

func LoadConfig(path string, l logr.Logger) (*state.AppState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Create default state if config doesn't exist
			appState := &state.AppState{
				Repositories: make(map[string]*repository.Repository),
				Settings:     settings.Settings{},
				Scheduler:    scheduler.NewScheduler(l),
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
	appState := state.NewState(l, config.Settings)

	// Set up repositories and their schedules
	for path, repo := range config.Repositories {
		rProviderOpts, err := remote.SettingsToOptions(repo.RemoteProvider)
		if err != nil {
			l.Error(err, "Error setting up remote provider", "path", path)
			continue
		}
		rProvider := remote.New(rProviderOpts)
		r := repository.NewRepositoryWithRemote(repo.Path, rProvider)
		err = appState.RAG.AddRepository(repo.Path)
		if err != nil {
			l.Error(err, "Error adding repository to RAG")
		}
		l.Info("Added repository to VectorDB", "path", repo.Path)
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
			err := r.Sync(appState.Agents)
			if err != nil {
				l.Error(err, "Error syncing repo")
			}
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
