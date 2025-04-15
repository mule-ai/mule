package main

import (
	"embed"
	"html/template"
	"net/http"
	"strings"

	"github.com/mule-ai/mule/internal/config"
	"github.com/mule-ai/mule/internal/handlers"
	"github.com/mule-ai/mule/internal/settings"
	"github.com/mule-ai/mule/internal/state"
	"github.com/mule-ai/mule/pkg/log"
	"github.com/mule-ai/mule/pkg/repository"

	"github.com/rs/cors"
)

//go:embed templates
var templatesFS embed.FS

//go:embed templates/static
var staticFS embed.FS

var templates *template.Template

func init() {
	var err error

	// Define template functions
	funcMap := template.FuncMap{
		"add": func(a, b int) int {
			return a + b
		},
	}

	templates, err = template.New("").Funcs(funcMap).ParseFS(templatesFS, "templates/*.html")
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

	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/repositories", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handlers.HandleListRepositories(w, r)
		case http.MethodPost:
			handlers.HandleAddRepository(w, r)
		case http.MethodDelete:
			handlers.HandleDeleteRepository(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/repositories/clone", methodHandler(http.MethodPost, handlers.HandleCloneRepository))
	mux.HandleFunc("/api/repositories/update", methodHandler(http.MethodPost, handlers.HandleUpdateRepository))
	mux.HandleFunc("/api/repositories/sync", methodHandler(http.MethodPost, handlers.HandleSyncRepository))
	mux.HandleFunc("/api/repositories/provider", methodHandler(http.MethodPost, handlers.HandleSwitchProvider))

	mux.HandleFunc("/api/models", methodHandler(http.MethodGet, handlers.HandleModels))
	mux.HandleFunc("/api/tools", methodHandler(http.MethodGet, handlers.HandleTools))
	mux.HandleFunc("/api/validation-functions", methodHandler(http.MethodGet, handlers.HandleValidationFunctions))
	mux.HandleFunc("/api/template-values", methodHandler(http.MethodGet, handlers.HandleTemplateValues))
	mux.HandleFunc("/api/workflow-output-fields", methodHandler(http.MethodGet, handlers.HandleWorkflowOutputFields))
	mux.HandleFunc("/api/workflow-input-mappings", methodHandler(http.MethodGet, handlers.HandleWorkflowInputMappings))

	// GitHub API routes
	mux.HandleFunc("/api/github/repositories", methodHandler(http.MethodGet, handlers.HandleGitHubRepositories))
	mux.HandleFunc("/api/github/issues", methodHandler(http.MethodGet, handlers.HandleGitHubIssues))

	// Local provider routes
	mux.HandleFunc("/api/local/issues", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			handlers.HandleCreateLocalIssue(w, r)
		case http.MethodDelete:
			handlers.HandleDeleteLocalIssue(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	mux.HandleFunc("/api/local/issues/update", methodHandler(http.MethodPost, handlers.HandleUpdateLocalIssue))
	mux.HandleFunc("/api/local/pullrequests", methodHandler(http.MethodDelete, handlers.HandleDeleteLocalPullRequest))
	mux.HandleFunc("/api/local/comments", methodHandler(http.MethodPost, handlers.HandleAddLocalComment))
	mux.HandleFunc("/api/local/reactions", methodHandler(http.MethodPost, handlers.HandleAddLocalReaction))
	mux.HandleFunc("/api/local/diff", methodHandler(http.MethodGet, handlers.HandleGetLocalDiff))
	mux.HandleFunc("/api/local/labels", methodHandler(http.MethodPost, handlers.HandleAddLocalLabel))
	mux.HandleFunc("/api/local/issues/state", methodHandler(http.MethodPost, handlers.HandleUpdateLocalIssueState))
	mux.HandleFunc("/api/local/pullrequests/state", methodHandler(http.MethodPost, handlers.HandleUpdateLocalPullRequestState))

	// Settings routes
	mux.HandleFunc("/api/settings", methodHandler(http.MethodPost, handlers.HandleUpdateSettings))

	// Web routes
	mux.HandleFunc("/", handleHome)
	mux.HandleFunc("/settings", handleSettingsPage)
	mux.HandleFunc("/local-provider", handlers.HandleLocalProviderPage)
	mux.HandleFunc("/logs", handlers.HandleLogs)

	// Static files
	staticHandler := http.FileServer(http.FS(staticFS))
	mux.HandleFunc("/static/", func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = "templates/static/" + strings.TrimPrefix(r.URL.Path, "/static/")
		staticHandler.ServeHTTP(w, r)
	})

	// CORS
	c := cors.New(cors.Options{
		AllowedOrigins: []string{"*"},
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Accept", "Content-Type", "Content-Length", "Accept-Encoding", "X-CSRF-Token", "Authorization"},
	})

	// Start the scheduler
	state.State.Scheduler.Start()
	defer state.State.Scheduler.Stop()

	handler := c.Handler(mux)

	defaultWorkflow := state.State.Workflows["default"]
	go func() {
		for _, repo := range state.State.Repositories {
			err := repo.Sync(state.State.Agents, defaultWorkflow)
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

// Helper function to handle specific HTTP methods
func methodHandler(method string, handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != method {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		handler(w, r)
	}
}

type PageData struct {
	Page         string
	Repositories map[string]*repository.Repository
	Settings     settings.Settings
}

func handleHome(w http.ResponseWriter, r *http.Request) {
	// Only handle exact "/" path to avoid conflicts with other routes
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

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
	defer state.State.Mu.RUnlock()

	data := PageData{
		Page:     "settings",
		Settings: state.State.Settings,
	}

	err := templates.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
