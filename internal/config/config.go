package config

import (
	"dev-team/internal/scheduler"
	"dev-team/internal/settings"
	"dev-team/internal/state"
	"dev-team/pkg/repository"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
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
					OllamaServer: "http://localhost:11434",
					OllamaModel:  "llama2",
				},
				Scheduler: scheduler.NewScheduler(),
			}
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
		r := &repository.Repository{
			Path:     repo.Path,
			Schedule: repo.Schedule,
		}
		err := r.UpdateStatus()
		if err != nil {
			log.Printf("Error getting repo status: %v", err)
		}
		appState.Repositories[path] = r
		err = appState.Scheduler.AddTask(path, repo.Schedule, func() {
			appState.Mu.RLock()
			aiService := appState.Settings.GetAIService()
			appState.Mu.RUnlock()
			err := repo.Sync(aiService, appState.Settings.GitHubToken)
			if err != nil {
				log.Printf("Error syncing repo: %v", err)
			}
			appState.Mu.Lock()
			appState.Repositories[path] = &repo
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
		config.Repositories[path] = repository.Repository{
			Path:     repo.Path,
			Schedule: repo.Schedule,
		}
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configPath, data, 0644)
}
