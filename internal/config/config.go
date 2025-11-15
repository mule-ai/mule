package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/go-logr/logr"
	"github.com/mitchellh/mapstructure"
	"github.com/mule-ai/mule/internal/settings"
	"github.com/mule-ai/mule/internal/state"
	"github.com/mule-ai/mule/pkg/remote"
	"github.com/mule-ai/mule/pkg/repository"
	"github.com/spf13/viper"
)

const DefaultConfigFileName = "config"
const DefaultConfigType = "yaml"
const DefaultConfigDir = ".config/mule"
const DefaultGeneratedConfigFileName = "config-default.yaml"

type Config struct {
	Repositories map[string]*repository.Repository `yaml:"repositories" mapstructure:"repositories"`
	Settings     settings.Settings                 `yaml:"settings" mapstructure:"settings"`
}

func GetHomeConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, DefaultConfigDir, DefaultConfigFileName+"."+DefaultConfigType), nil
}

func manageDefaultConfigFile(configDirPath string, l logr.Logger) error {
	defaultConfigFilePath := filepath.Join(configDirPath, DefaultGeneratedConfigFileName)
	defaultConfig := Config{
		Repositories: make(map[string]*repository.Repository),
		Settings:     settings.DefaultSettings,
	}
	v := viper.New()
	v.Set("repositories", defaultConfig.Repositories)
	v.Set("settings", defaultConfig.Settings)
	if err := os.MkdirAll(filepath.Dir(defaultConfigFilePath), 0755); err != nil {
		return fmt.Errorf("error creating directory for default config %s: %w", defaultConfigFilePath, err)
	}
	if err := v.WriteConfigAs(defaultConfigFilePath); err != nil {
		return fmt.Errorf("error writing default config file %s with viper: %w", defaultConfigFilePath, err)
	}
	l.Info("Ensured default configuration is written using viper", "path", defaultConfigFilePath)
	return nil
}

func LoadConfig(path string, l logr.Logger) (*state.AppState, error) {
	configDirPath := filepath.Dir(path)
	defaultGeneratedConfigPath := filepath.Join(configDirPath, DefaultGeneratedConfigFileName)

	if err := manageDefaultConfigFile(configDirPath, l); err != nil {
		return nil, fmt.Errorf("failed to manage default config file at %s: %w", configDirPath, err)
	}

	mainViper := viper.New()
	mainViper.SetConfigType(DefaultConfigType)

	// Layer 1: Load config-default.yaml
	mainViper.SetConfigFile(defaultGeneratedConfigPath)
	if err := mainViper.ReadInConfig(); err != nil {
		// This is critical because manageDefaultConfigFile should have created it.
		return nil, fmt.Errorf("critical: failed to read generated default config file %s: %w", defaultGeneratedConfigPath, err)
	}
	l.Info("Loaded default config", "path", defaultGeneratedConfigPath)

	// Layer 2: Main user config file (e.g., config.yaml from `path` argument)
	// MergeInConfig will not error if the file doesn't exist, which is desired.
	if _, err := os.Stat(path); err == nil {
		mainViper.SetConfigFile(path)
		if err := mainViper.MergeInConfig(); err != nil {
			l.Error(err, "Error merging main user config file, continuing with defaults/overrides.", "path", path)
		} else {
			l.Info("Merged main user config", "path", path)
		}
	} else if !os.IsNotExist(err) {
		// Log if there's an error other than file not existing
		l.Error(err, "Error checking main user config file, continuing with defaults/overrides.", "path", path)
	}

	// Layer 3: Override files
	overrideFiles, err := filepath.Glob(filepath.Join(configDirPath, "config-override*.yaml"))
	if err != nil {
		l.Error(err, "Error globbing for override files, proceeding without them.", "pattern", filepath.Join(configDirPath, "config-override*.yaml"))
	} else {
		sort.Strings(overrideFiles) // Ensure deterministic order for overrides
		for _, overrideFile := range overrideFiles {
			mainViper.SetConfigFile(overrideFile)
			if err := mainViper.MergeInConfig(); err != nil {
				l.Error(err, "Error merging override config file, skipping this override.", "path", overrideFile)
			} else {
				l.Info("Merged override config file", "path", overrideFile)
			}
		}
	}

	// Final Step: Unmarshal the fully merged map into the Config struct
	var finalConfig Config
	decodeHooks := mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
		mapstructure.StringToSliceHookFunc(","),
	)
	if err := mainViper.Unmarshal(&finalConfig, viper.DecodeHook(decodeHooks)); err != nil {
		return nil, fmt.Errorf("error unmarshalling final merged config to Config struct: %w", err)
	}

	// Debug: Log the number of workflows loaded
	l.Info("Loaded workflows", "count", len(finalConfig.Settings.Workflows))
	for i, workflow := range finalConfig.Settings.Workflows {
		l.Info("Loaded workflow", "index", i, "name", workflow.Name, "id", workflow.ID)
	}

	appState := state.NewState(l, finalConfig.Settings)
	for repoPathVal, repo := range finalConfig.Repositories {
		if repo == nil {
			l.Error(fmt.Errorf("nil repository pointer in final config for key %s", repoPathVal), "Skipping repository initialization")
			continue
		}

		// Default RemoteProvider if not specified in config
		if repo.RemoteProvider.Provider == "" {
			l.Info("Remote provider not specified, defaulting to local", "repository", repoPathVal, "repo.Path", repo.Path)
			repo.RemoteProvider.Provider = remote.ProviderTypeToString(remote.LOCAL)
			// When using a local provider, its path is the same as the repository's main path.
			// The repo.Path field should already be populated from the config or defaults.
			repo.RemoteProvider.Path = repo.Path
		}

		rProviderOpts, errRemote := remote.SettingsToOptions(repo.RemoteProvider)
		if errRemote != nil {
			l.Error(errRemote, "Error setting up remote provider", "path", repoPathVal)
			continue
		}
		rProvider := remote.New(rProviderOpts)
		r := repository.NewRepositoryWithRemote(repo.Path, rProvider)
		errRAG := appState.RAG.AddRepository(repo.Path)
		if errRAG != nil {
			l.Error(errRAG, "Error adding repository to RAG")
		} else {
			l.Info("Added repository to VectorDB", "path", repo.Path)
		}
		r.Logger = l.WithName("repository").WithValues("path", repo.Path)
		r.Schedule = repo.Schedule
		r.RemotePath = repo.RemotePath
		r.RemoteProvider = repo.RemoteProvider
		errStatus := r.UpdateStatus()
		if errStatus != nil {
			l.Error(errStatus, "Error getting repo status")
		}
		appState.Repositories[repoPathVal] = r
		defaultWorkflow := appState.Workflows["default"]
		errTask := appState.Scheduler.AddTask(repoPathVal, repo.Schedule, func() {
			errSync := r.Sync(appState.Agents, defaultWorkflow)
			if errSync != nil {
				l.Error(errSync, "Error syncing repo")
			}
		})
		if errTask != nil {
			l.Error(errTask, "Error setting up schedule for repository", "repository", repoPathVal)
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

	currentConfig := Config{
		Repositories: make(map[string]*repository.Repository),
		Settings:     state.State.Settings,
	}
	if state.State.Repositories == nil {
		state.State.Repositories = make(map[string]*repository.Repository)
	}
	for repoPath, repo := range state.State.Repositories {
		currentConfig.Repositories[repoPath] = repo
	}

	vconfig := viper.New()
	vconfig.Set("repositories", currentConfig.Repositories)
	vconfig.Set("settings", currentConfig.Settings)

	if err := vconfig.WriteConfigAs(path); err != nil {
		return err
	}

	return nil
}
