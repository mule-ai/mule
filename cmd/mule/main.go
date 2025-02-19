package main

import (
	"embed"
	"html/template"
	"net/http"

	"github.com/mule-ai/mule/internal/config"
	"github.com/mule-ai/mule/internal/handlers"
	"github.com/mule-ai/mule/internal/settings"
	"github.com/mule-ai/mule/internal/state"
	"github.com/mule-ai/mule/pkg/log"
	"github.com/mule-ai/mule/pkg/repository"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
)

//go:embed templates
var templatesFS embed.FS

//go:embed templates/static
var staticFS embed.FS

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
	l := log.New("")

	// Create config path
	configPath, err := config.GetHomeConfigPath()
	if err != nil {
		l.Error(err, "Error getting config path")
	}

	// Load config
	appState, err := config.LoadConfig(configPath, l)
	if err != nil {
		l.Error(err, "Error loading config")
	}

	state.State = appState

	r := mux.NewRouter()

	// API routes
	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/repositories", handlers.HandleListRepositories).Methods("GET")
	api.HandleFunc("/repositories", handlers.HandleAddRepository).Methods("POST")
	api.HandleFunc("/repositories", handlers.HandleDeleteRepository).Methods("DELETE")
	api.HandleFunc("/repositories/clone", handlers.HandleCloneRepository).Methods("POST")
	api.HandleFunc("/repositories/update", handlers.HandleUpdateRepository).Methods("POST")
	api.HandleFunc("/repositories/sync", handlers.HandleSyncRepository).Methods("POST")
	api.HandleFunc("/repositories/provider", handlers.HandleSwitchProvider).Methods("POST")
	api.HandleFunc("/models", handlers.HandleModels).Methods("GET")
	api.HandleFunc("/tools", handlers.HandleTools).Methods("GET")
	api.HandleFunc("/validation-functions", handlers.HandleValidationFunctions).Methods("GET")
	api.HandleFunc("/template-values", handlers.HandleTemplateValues).Methods("GET")

	// GitHub API routes
	api.HandleFunc("/github/repositories", handlers.HandleGitHubRepositories).Methods("GET")
	api.HandleFunc("/github/issues", handlers.HandleGitHubIssues).Methods("GET")

	// Local provider routes
	api.HandleFunc("/local/issues", handlers.HandleCreateLocalIssue).Methods("POST")
	api.HandleFunc("/local/issues", handlers.HandleDeleteLocalIssue).Methods("DELETE")
	api.HandleFunc("/local/pullrequests", handlers.HandleDeleteLocalPullRequest).Methods("DELETE")
	api.HandleFunc("/local/comments", handlers.HandleAddLocalComment).Methods("POST")
	api.HandleFunc("/local/reactions", handlers.HandleAddLocalReaction).Methods("POST")
	api.HandleFunc("/local/diff", handlers.HandleGetLocalDiff).Methods("GET")
	api.HandleFunc("/local/labels", handlers.HandleAddLocalLabel).Methods("POST")
	api.HandleFunc("/local/issues/state", handlers.HandleUpdateLocalIssueState).Methods("POST")
	api.HandleFunc("/local/pullrequests/state", handlers.HandleUpdateLocalPullRequestState).Methods("POST")

	// Settings routes
	api.HandleFunc("/settings", handlers.HandleUpdateSettings).Methods("POST")

	// Web routes
	r.HandleFunc("/", handleHome)
	r.HandleFunc("/settings", handleSettingsPage)
	r.HandleFunc("/local-provider", handlers.HandleLocalProviderPage)
	r.HandleFunc("/logs", handlers.HandleLogs)

	// Static files
	staticHandler := http.FileServer(http.FS(staticFS))
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "templates/static/" + r.URL.Path
		staticHandler.ServeHTTP(w, r)
	})))

	// CORS
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Content-Type", "Content-Length", "Accept-Encoding", "X-CSRF-Token", "Authorization"},
	})
	// Start the scheduler
	state.State.Scheduler.Start()
	defer state.State.Scheduler.Stop()

	handler := c.Handler(r)
	go func() {
		for _, repo := range state.State.Repositories {
			err := repo.Sync(state.State.Agents)
			if err != nil {
				l.Error(err, "Error syncing repo")
			}
		}
	}()
	// Start server
	l.Info("Starting server on :8083")
	if err := http.ListenAndServe(":8083", handler); err != nil {
		l.Error(err, "Error starting server")
	}
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
