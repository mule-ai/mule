package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mule-ai/mule/internal/settings"
	"github.com/mule-ai/mule/internal/state"
	"github.com/mule-ai/mule/pkg/log"
	"github.com/mule-ai/mule/pkg/repository"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	testDir := t.TempDir()
	mainConfigUserPath := filepath.Join(testDir, "config.yaml")
	defaultGeneratedPath := filepath.Join(testDir, DefaultGeneratedConfigFileName)
	l := log.NewStdoutLogger()

	// 1. Test when main config file (config.yaml) doesn't exist
	// Expect DefaultSettings to be loaded, and config-default.yaml to be written.
	appStateNoMain, errNoMain := LoadConfig(mainConfigUserPath, l)
	assert.NoError(t, errNoMain, "LoadConfig should not error if main config is missing")
	require.NotNil(t, appStateNoMain, "AppState should not be nil if main config is missing")
	assert.FileExists(t, defaultGeneratedPath, "config-default.yaml should be written even if main config is missing")
	// Check that settings are defaults
	assert.Equal(t, settings.DefaultSettings, appStateNoMain.Settings, "Settings should be DefaultSettings when main config is missing")
	assert.Empty(t, appStateNoMain.Repositories, "Repositories should be empty when main config is missing and no overrides")

	// Ensure config.yaml was NOT created by LoadConfig in this case
	_, errStat := os.Stat(mainConfigUserPath)
	assert.True(t, os.IsNotExist(errStat), "config.yaml should NOT be created by LoadConfig if it was missing")

	// 2. Test when main config file (config.yaml) exists
	// Create a minimal config.yaml for this part of the test
	minimalUserConfigContent := `
settings:
  githubToken: "user_token_from_config_yaml"
repositories:
  myrepo:
    path: "/path/to/myrepo"
`
	require.NoError(t, os.WriteFile(mainConfigUserPath, []byte(minimalUserConfigContent), 0644), "Failed to write minimal config.yaml for testing")

	appStateWithMain, errWithMain := LoadConfig(mainConfigUserPath, l)
	assert.NoError(t, errWithMain, "LoadConfig should not error if main config exists")
	require.NotNil(t, appStateWithMain, "AppState should not be nil if main config exists")
	assert.FileExists(t, defaultGeneratedPath, "config-default.yaml should still be written if main config exists")

	// Check that settings are from the minimal config.yaml
	assert.Equal(t, "user_token_from_config_yaml", appStateWithMain.Settings.GitHubToken, "GitHubToken should be from user's config.yaml")
	require.Contains(t, appStateWithMain.Repositories, "myrepo", "Repositories should contain 'myrepo' from user's config.yaml")
	assert.Equal(t, "/path/to/myrepo", appStateWithMain.Repositories["myrepo"].Path)

	// Clean up the created config.yaml for subsequent tests if any (though t.TempDir handles testDir cleanup)
	assert.NoError(t, os.Remove(mainConfigUserPath))
}

func TestSaveConfig(t *testing.T) {
	configPath := filepath.Join("testdata", "config.yaml")

	// Ensure testdata directory exists for the test
	if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
		t.Fatalf("Error creating testdata directory: %v", err)
	}
	// Clean up config file after test
	defer func() {
		if err := os.RemoveAll(filepath.Dir(configPath)); err != nil {
			t.Errorf("Failed to remove testdata directory: %v", err)
		}
	}() // Remove the whole testdata directory

	// Create a dummy state
	// Using a new AppState to avoid conflicts with global state.State if tests run in parallel or affect each other.
	dummyAppState := &state.AppState{
		Repositories: make(map[string]*repository.Repository),
		Settings:     settings.DefaultSettings, // Assuming settings.DefaultSettings is available and valid
		// Scheduler might not be needed for just saving config, but initializing for completeness if state.State was directly used.
	}
	// Temporarily set the global state for SaveConfig to use, as SaveConfig reads from state.State
	originalState := state.State
	state.State = dummyAppState
	defer func() { state.State = originalState }() // Restore original state

	// Save the config
	if err := SaveConfig(configPath); err != nil {
		t.Fatalf("Error saving config: %v", err)
	}

	// Check if the file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Errorf("Config file was not created: %v", err)
	}
}

func TestManageDefaultConfigFile(t *testing.T) {
	testDir := t.TempDir() // Creates a temporary directory for the test
	l := log.NewStdoutLogger()

	defaultConfigFilePath := filepath.Join(testDir, DefaultGeneratedConfigFileName)

	// 1. Test initial creation
	err := manageDefaultConfigFile(testDir, l)
	assert.NoError(t, err, "manageDefaultConfigFile should not return an error on initial creation")
	assert.FileExists(t, defaultConfigFilePath, "config-default.yaml should be created")

	// Verify content of the created default config file using viper
	var loadedDefaultConfig Config
	v := viper.New()
	v.SetConfigFile(defaultConfigFilePath)
	err = v.ReadInConfig()
	assert.NoError(t, err, "Failed to read created default config file with viper")
	err = v.Unmarshal(&loadedDefaultConfig)
	assert.NoError(t, err, "Failed to unmarshal created default config file with viper")

	assert.Equal(t, settings.DefaultSettings, loadedDefaultConfig.Settings, "Settings in default config file should match settings.DefaultSettings")
	assert.Empty(t, loadedDefaultConfig.Repositories, "Repositories in default config file should be empty")

	// 2. Test overwriting (ensuring it re-writes with correct defaults if file is changed)
	// Modify the file on disk with some different content (can be simple invalid YAML or just different structure)
	modifiedContent := "settings:\n  githubToken: tampered_token\n  aiProviders: []\nrepositories: {}\n"
	err = os.WriteFile(defaultConfigFilePath, []byte(modifiedContent), 0644)
	assert.NoError(t, err, "Failed to write modified content to default config file for testing overwrite")

	// Call manageDefaultConfigFile again
	err = manageDefaultConfigFile(testDir, l)
	assert.NoError(t, err, "manageDefaultConfigFile should not return an error when overwriting")

	// Reload and verify it's back to defaults using viper
	var overwrittenConfig Config
	vOverwrite := viper.New()
	vOverwrite.SetConfigFile(defaultConfigFilePath)
	err = vOverwrite.ReadInConfig()
	assert.NoError(t, err, "Failed to read overwritten default config file with viper")
	err = vOverwrite.Unmarshal(&overwrittenConfig)
	assert.NoError(t, err, "Failed to unmarshal overwritten default config file with viper")

	assert.Equal(t, settings.DefaultSettings, overwrittenConfig.Settings, "Settings in overwritten default config file should match settings.DefaultSettings")
	assert.Empty(t, overwrittenConfig.Repositories, "Repositories in overwritten default config file should be empty")
}

func TestConfigOverrides(t *testing.T) {
	testDir := t.TempDir()
	l := log.NewStdoutLogger()

	mainConfigPath := filepath.Join(testDir, "config.yaml")
	defaultConfigPath := filepath.Join(testDir, DefaultGeneratedConfigFileName) // For checking its presence

	// --- Scenario 1: Main config + one override ---
	mainConfigContent1 := `
settings:
  githubToken: "main_token"
  aiProviders:
    - name: "ollama_main"
      provider: "ollama"
      server: "http://main:11434"
  agents:
    - id: 1
      name: "agent_main"
      model: "model_main"
repositories:
  repo1:
    path: "/path/to/repo1_main"
`
	overrideConfigContent1 := `
settings:
  githubToken: "override_token_1"
  aiProviders:
    - name: "ollama_override_1" # This should add to existing, not replace if structure allows merging by key/ID
      provider: "ollama"
      server: "http://override1:11434"
  agents:
    - id: 1 # This should override agent_main due to matching ID if viper merges slices of maps by a key
      name: "agent_override_1"
      model: "model_override_1"
    - id: 2
      name: "new_agent_override_1"
      model: "new_model_override_1"
`
	require.NoError(t, os.WriteFile(mainConfigPath, []byte(mainConfigContent1), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "config-override-A.yaml"), []byte(overrideConfigContent1), 0644))

	appState1, err1 := LoadConfig(mainConfigPath, l)
	assert.NoError(t, err1)
	require.NotNil(t, appState1)
	assert.FileExists(t, defaultConfigPath) // Check default is also written

	// Check merged settings
	assert.Equal(t, "override_token_1", appState1.Settings.GitHubToken, "Scenario 1: GitHubToken should be from override")

	// Viper's default merge strategy for slices is to replace.
	// For complex slice merging (e.g., by ID on AIProviders/Agents), custom logic or specific viper features might be needed.
	// For now, assume replace for slices unless a specific merge key is supported by viper implicitly for mapstructure.
	// Based on typical viper behavior, the aiProviders slice from override will likely replace the main one.
	assert.Len(t, appState1.Settings.AIProviders, 1, "Scenario 1: AIProviders slice should be from override")
	if len(appState1.Settings.AIProviders) == 1 {
		assert.Equal(t, "ollama_override_1", appState1.Settings.AIProviders[0].Name)
	}
	assert.Len(t, appState1.Settings.Agents, 2, "Scenario 1: Agents slice should be from override")
	// Find agent by ID to confirm merge/override behavior for slices of structs. This is tricky with viper.
	// Default viper MergeConfigMap usually replaces slices entirely.
	foundAgent1 := false
	for _, agent := range appState1.Settings.Agents {
		if agent.ID == 1 && agent.Name == "agent_override_1" {
			foundAgent1 = true
			break
		}
	}
	assert.True(t, foundAgent1, "Scenario 1: Agent ID 1 should be overridden")

	// Check repositories. Since the override file does not contain a 'repositories' key,
	// the repositories from the main config should persist.
	_, repo1Exists := appState1.Repositories["repo1"]
	assert.True(t, repo1Exists, "Scenario 1: Repo1 from main config should exist as override did not touch repositories key.")
	if repo1Exists {
		assert.Equal(t, "/path/to/repo1_main", appState1.Repositories["repo1"].Path, "Scenario 1: Repo1 path should be from main config.")
	}

	// Re-cleaning for next scenario to avoid interference
	require.NoError(t, os.Remove(mainConfigPath))
	require.NoError(t, os.Remove(filepath.Join(testDir, "config-override-A.yaml")))

	// --- Scenario 2: No main config, only default + override ---
	// manageDefaultConfigFile will be called by LoadConfig, creating config-default.yaml
	overrideConfigContent2 := `
settings:
  githubToken: "override_only_token"
  aiProviders:
    - name: "ollama_override_only"
      provider: "ollama"
      server: "http://override_only:11434"
`
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "config-override-B.yaml"), []byte(overrideConfigContent2), 0644))

	// mainConfigPath does not exist here
	appState2, err2 := LoadConfig(mainConfigPath, l) // mainConfigPath is still testDir/config.yaml
	assert.NoError(t, err2)
	require.NotNil(t, appState2)
	assert.FileExists(t, defaultConfigPath) // Default config should still be written

	assert.Equal(t, "override_only_token", appState2.Settings.GitHubToken, "Scenario 2: GitHubToken from override, base from DefaultSettings")
	assert.Len(t, appState2.Settings.AIProviders, 1, "Scenario 2: AIProviders should be from override")
	if len(appState2.Settings.AIProviders) == 1 {
		assert.Equal(t, "ollama_override_only", appState2.Settings.AIProviders[0].Name)
	}
	// Check that other default settings are present (e.g., default agents)
	assert.NotEmpty(t, appState2.Settings.Agents, "Scenario 2: Default agents should be present")
	assert.Equal(t, settings.DefaultSettings.Agents[0].Model, appState2.Settings.Agents[0].Model, "Scenario 2: Default agent model should match")

	require.NoError(t, os.Remove(filepath.Join(testDir, "config-override-B.yaml")))

	// --- Scenario 3: Main config + multiple overrides (testing merge order if keys conflict) ---
	// For simplicity, use different keys in overrides or assume last merge wins for conflicting simple keys.
	mainConfigContent3 := `
settings:
  githubToken: "main_token_3"
  systemAgent:
    model: "main_sys_agent_model"
`
	overrideConfigContent3A := `
settings:
  githubToken: "override_3A_token"
  aiProviders:
    - name: "provider_A"
`
	overrideConfigContent3B := `
settings:
  githubToken: "override_3B_token"
  systemAgent:
    model: "override_sys_agent_model_3B"
`
	require.NoError(t, os.WriteFile(mainConfigPath, []byte(mainConfigContent3), 0644))
	// Create override files. Order of globbing might not be guaranteed, but viper's MergeConfigMap usually has last one win.
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "config-override-X.yaml"), []byte(overrideConfigContent3A), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "config-override-Y.yaml"), []byte(overrideConfigContent3B), 0644))

	appState3, err3 := LoadConfig(mainConfigPath, l)
	assert.NoError(t, err3)
	require.NotNil(t, appState3)

	// Assuming Y is merged after X due to typical glob order (or specific sort if added)
	// If sort.Strings was used and X comes before Y alphabetically, Y wins.
	assert.Equal(t, "override_3B_token", appState3.Settings.GitHubToken, "Scenario 3: GitHubToken should be from last override (Y)")
	assert.Equal(t, "override_sys_agent_model_3B", appState3.Settings.SystemAgent.Model, "Scenario 3: SystemAgent Model should be from override (Y)")
	assert.Len(t, appState3.Settings.AIProviders, 1, "Scenario 3: AIProviders should be from override (X)")
	if len(appState3.Settings.AIProviders) == 1 {
		assert.Equal(t, "provider_A", appState3.Settings.AIProviders[0].Name)
	}

	require.NoError(t, os.Remove(mainConfigPath))
	require.NoError(t, os.Remove(filepath.Join(testDir, "config-override-X.yaml")))
	require.NoError(t, os.Remove(filepath.Join(testDir, "config-override-Y.yaml")))
}
