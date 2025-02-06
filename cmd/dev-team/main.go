package main

import (
	"embed"
	"html/template"
	"net/http"
	"os"
	"path/filepath"

	"github.com/jbutlerdev/dev-team/internal/config"
	"github.com/jbutlerdev/dev-team/internal/handlers"
	"github.com/jbutlerdev/dev-team/internal/settings"
	"github.com/jbutlerdev/dev-team/internal/state"
	"github.com/jbutlerdev/dev-team/pkg/log"
	"github.com/jbutlerdev/dev-team/pkg/repository"

	"github.com/jbutlerdev/genai"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
)

//go:embed templates
var templatesFS embed.FS

var templates *template.Template

func init() {
	var err error
	templates, err = template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		panic(err)
	}
	handlers.InitTemplates(templates)
}

func main() {
	// Initialize log
	l := log.New("dev-team.log")

	// Create config path
	homeDir, err := os.UserHomeDir()
	if err != nil {
		l.Error(err, "Error getting user home directory")
	}
	configDir := filepath.Join(homeDir, ".config", "dev-team")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		l.Error(err, "Error creating config directory")
	}
	configPath := filepath.Join(configDir, "config.json")

	// Load config
	appState, err := config.LoadConfig(configPath, l)
	if err != nil {
		l.Error(err, "Error loading config")
	}

	// Initialize GenAI provider
	genaiProvider, err := genai.NewProviderWithLog(
		appState.Settings.Provider,
		appState.Settings.APIKey,
		l.WithName("genai-provider"),
	)
	if err != nil {
		l.Error(err, "Error initializing GenAI provider")
	}
	appState.GenAI = genaiProvider
	state.State = appState

	r := mux.NewRouter()

	// API routes
	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/repositories", handlers.HandleListRepositories).Methods("GET")
	api.HandleFunc("/repositories", handlers.HandleAddRepository).Methods("POST")
	api.HandleFunc("/repositories", handlers.HandleDeleteRepository).Methods("DELETE")
	api.HandleFunc("/repositories/update", handlers.HandleUpdateRepository).Methods("POST")
	api.HandleFunc("/repositories/clone", handlers.HandleCloneRepository).Methods("POST")
	api.HandleFunc("/repositories/commit", handlers.HandleCommit).Methods("POST")
	api.HandleFunc("/repositories/push", handlers.HandlePush).Methods("POST")
	api.HandleFunc("/repositories/pr", handlers.HandleCreatePR).Methods("POST")
	api.HandleFunc("/repositories/sync", handlers.HandleSyncRepository).Methods("POST")
	api.HandleFunc("/settings", handlers.HandleGetSettings).Methods("GET")
	api.HandleFunc("/settings", handlers.HandleUpdateSettings).Methods("POST")
	api.HandleFunc("/gemini/models", handlers.HandleGeminiModels).Methods("GET")
	api.HandleFunc("/github/repositories", handlers.HandleGitHubRepositories).Methods("GET")
	api.HandleFunc("/github/issues", handlers.HandleGitHubIssues).Methods("GET")

	// Web routes
	r.HandleFunc("/", handleHome).Methods("GET")
	r.HandleFunc("/logs", handlers.HandleLogs).Methods("GET")
	r.HandleFunc("/settings", handleSettingsPage).Methods("GET")

	// Configure CORS for API routes
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"*"},
	})

	// Start the scheduler
	state.State.Scheduler.Start()
	defer state.State.Scheduler.Stop()

	handler := c.Handler(r)
	go func() {
		for _, repo := range state.State.Repositories {
			err := repo.Sync(state.State.GenAI, appState.Settings.GitHubToken)
			if err != nil {
				l.Error(err, "Error syncing repo")
			}
		}
	}()
	l.Info("Server starting on http://0.0.0.0:8083")
	l.Error(http.ListenAndServe("0.0.0.0:8083", handler), "Error starting server")
}

type PageData struct {
	Page         string
	Repositories map[string]*repository.Repository
	Settings     settings.Settings
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	state.State.Mu.RLock()
	data := PageData{
		Page:         "home",
		Repositories: state.State.Repositories,
		Settings:     state.State.Settings,
	}
	state.State.Mu.RUnlock()

	err := templates.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleSettingsPage(w http.ResponseWriter, r *http.Request) {
	state.State.Mu.RLock()
	data := PageData{
		Page:         "settings",
		Repositories: state.State.Repositories,
		Settings:     state.State.Settings,
	}
	state.State.Mu.RUnlock()

	err := templates.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
