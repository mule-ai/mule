package config

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"github.com/jbutlerdev/dev-team/internal/scheduler"
	"github.com/jbutlerdev/dev-team/internal/settings"
	"github.com/jbutlerdev/dev-team/internal/state"
	"github.com/jbutlerdev/dev-team/pkg/repository"
)

func LoadConfig() (*state.AppState, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	configDir := filepath.Join(homeDir, ".config", "dev-team")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, err
	}
	configPath := filepath.Join(configDir, "config.json")

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Create default state if config doesn't exist
			appState := &state.AppState{
				Repositories: make(map[string]*repository.Repository),
				Settings: settings.Settings{
					Provider: "gemini",
				},
				Scheduler: scheduler.NewScheduler(),
			}
			state.State = appState
			return appState, SaveConfig()
		}
		return nil, err
	}

	var config struct {
		Repositories map[string]repository.Repository `json:"repositories"`
		Settings     settings.Settings                `json:"settings"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	// Create state from config
	appState := &state.AppState{
		Repositories: make(map[string]*repository.Repository),
		Settings:     config.Settings,
		Scheduler:    scheduler.NewScheduler(),
	}

	// Set up repositories and their schedules
	for path, repo := range config.Repositories {
		r := repository.NewRepository(repo.Path)
		r.Schedule = repo.Schedule
		r.RemotePath = repo.RemotePath
		err := r.UpdateStatus()
		if err != nil {
			log.Printf("Error getting repo status: %v", err)
		}
		appState.Repositories[path] = r
		err = appState.Scheduler.AddTask(path, repo.Schedule, func() {
			err := r.Sync(state.State.GenAI, appState.Settings.GitHubToken)
			if err != nil {
				log.Printf("Error syncing repo: %v", err)
			}
			appState.Mu.Lock()
			appState.Repositories[path] = r
			appState.Mu.Unlock()
		})
		if err != nil {
			log.Printf("Error setting up schedule for %s: %v", path, err)
		}
	}

	return appState, nil
}

func SaveConfig() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	configPath := filepath.Join(homeDir, ".config", "dev-team", "config.json")

	state.State.Mu.RLock()
	defer state.State.Mu.RUnlock()

	// Create config from state
	config := struct {
		Repositories map[string]repository.Repository `json:"repositories"`
		Settings     settings.Settings                `json:"settings"`
	}{
		Repositories: make(map[string]repository.Repository),
		Settings:     state.State.Settings,
	}

	for path, repo := range state.State.Repositories {
		config.Repositories[path] = *repo
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}
