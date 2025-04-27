package main

import (
	"embed"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

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

// Add helper funcs before loading templates
func formatDate(t string) string {
	// Attempt to parse the date string (adjust format if necessary)
	parsedTime, err := time.Parse(time.RFC3339, t) // Assuming RFC3339, adjust if needed
	if err != nil {
		parsedTime, err = time.Parse("2006-01-02 15:04:05.999999999 -0700 MST", t) // Try another common format
		if err != nil {
			return t // Return original string if parsing fails
		}
	}
	return parsedTime.Format("Jan 2, 2006")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	// Find the last space within the limit to avoid cutting words
	lastSpace := strings.LastIndex(s[:maxLen], " ")
	if lastSpace > 0 {
		return s[:lastSpace] + "..."
	}
	return s[:maxLen] + "..."
}

func init() {
	var err error

	// Define template functions
	funcMap := template.FuncMap{
		"add": func(a, b int) int {
			return a + b
		},
		"urlquery":   url.QueryEscape,
		"safeHTML":   func(s string) template.HTML { return template.HTML(s) },
		"formatDate": formatDate, // Register helper func
		"truncate":   truncate,   // Register helper func
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
	mux.HandleFunc("/api/repositories", methodsHandler(map[string]http.HandlerFunc{
		http.MethodGet:    handlers.HandleListRepositories,
		http.MethodPost:   handlers.HandleAddRepository,
		http.MethodDelete: handlers.HandleDeleteRepository,
	}))

	mux.HandleFunc("/api/repositories/clone", methodsHandler(map[string]http.HandlerFunc{
		http.MethodPost: handlers.HandleCloneRepository,
	}))
	mux.HandleFunc("/api/repositories/update", methodsHandler(map[string]http.HandlerFunc{
		http.MethodPost: handlers.HandleUpdateRepository,
	}))
	mux.HandleFunc("/api/repositories/sync", methodsHandler(map[string]http.HandlerFunc{
		http.MethodPost: handlers.HandleSyncRepository,
	}))
	mux.HandleFunc("/api/repositories/provider", methodsHandler(map[string]http.HandlerFunc{
		http.MethodPost: handlers.HandleSwitchProvider,
	}))

	mux.HandleFunc("/api/models", methodsHandler(map[string]http.HandlerFunc{
		http.MethodGet: handlers.HandleModels,
	}))
	mux.HandleFunc("/api/tools", methodsHandler(map[string]http.HandlerFunc{
		http.MethodGet: handlers.HandleTools,
	}))
	mux.HandleFunc("/api/validation-functions", methodsHandler(map[string]http.HandlerFunc{
		http.MethodGet: handlers.HandleValidationFunctions,
	}))
	mux.HandleFunc("/api/template-values", methodsHandler(map[string]http.HandlerFunc{
		http.MethodGet: handlers.HandleTemplateValues,
	}))
	mux.HandleFunc("/api/workflow-output-fields", methodsHandler(map[string]http.HandlerFunc{
		http.MethodGet: handlers.HandleWorkflowOutputFields,
	}))
	mux.HandleFunc("/api/workflow-input-mappings", methodsHandler(map[string]http.HandlerFunc{
		http.MethodGet: handlers.HandleWorkflowInputMappings,
	}))

	// GitHub API routes
	mux.HandleFunc("/api/github/repositories", methodsHandler(map[string]http.HandlerFunc{
		http.MethodGet: handlers.HandleGitHubRepositories,
	}))
	mux.HandleFunc("/api/github/issues", methodsHandler(map[string]http.HandlerFunc{
		http.MethodGet: handlers.HandleGitHubIssues,
	}))

	// Local provider routes
	mux.HandleFunc("/api/local/issues", methodsHandler(map[string]http.HandlerFunc{
		http.MethodPost:   handlers.HandleCreateLocalIssue,
		http.MethodDelete: handlers.HandleDeleteLocalIssue,
	}))

	mux.HandleFunc("/api/local/issues/update", methodsHandler(map[string]http.HandlerFunc{
		http.MethodPost: handlers.HandleUpdateLocalIssue,
	}))
	mux.HandleFunc("/api/local/pullrequests", methodsHandler(map[string]http.HandlerFunc{
		http.MethodDelete: handlers.HandleDeleteLocalPullRequest,
	}))
	mux.HandleFunc("/api/local/comments", methodsHandler(map[string]http.HandlerFunc{
		http.MethodPost: handlers.HandleAddLocalComment,
	}))
	mux.HandleFunc("/api/local/reactions", methodsHandler(map[string]http.HandlerFunc{
		http.MethodPost: handlers.HandleAddLocalReaction,
	}))
	mux.HandleFunc("/api/local/diff", methodsHandler(map[string]http.HandlerFunc{
		http.MethodGet: handlers.HandleGetLocalDiff,
	}))
	mux.HandleFunc("/api/local/labels", methodsHandler(map[string]http.HandlerFunc{
		http.MethodPost: handlers.HandleAddLocalLabel,
	}))
	mux.HandleFunc("/api/local/issues/state", methodsHandler(map[string]http.HandlerFunc{
		http.MethodPost: handlers.HandleUpdateLocalIssueState,
	}))
	mux.HandleFunc("/api/local/pullrequests/state", methodsHandler(map[string]http.HandlerFunc{
		http.MethodPost: handlers.HandleUpdateLocalPullRequestState,
	}))

	// Settings routes
	mux.HandleFunc("/api/settings", methodsHandler(map[string]http.HandlerFunc{
		http.MethodPost: handlers.HandleUpdateSettings,
	}))

	// --- HTMX API Routes ---
	mux.HandleFunc("/api/github/repositories/htmx", handlers.HandleGitHubRepositoriesHTMX)  // GET
	mux.HandleFunc("/api/repositories/htmx", func(w http.ResponseWriter, r *http.Request) { // Add/Delete repo list
		switch r.Method {
		case http.MethodPost:
			handlers.HandleAddRepositoryHTMX(w, r)
		case http.MethodDelete:
			handlers.HandleDeleteRepositoryHTMX(w, r)
		// case http.MethodGet: // Could add GET to fetch the whole list initially if needed
		// 	handlers.HandleRepositoriesListHTMX(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/repositories/sync/htmx", handlers.HandleSyncRepositoryHTMX)     // POST
	mux.HandleFunc("/api/repositories/provider/htmx", handlers.HandleSetProviderHTMX)    // POST
	mux.HandleFunc("/api/repositories/update/htmx", handlers.HandleUpdateRepositoryHTMX) // POST - Placeholder
	mux.HandleFunc("/api/issues/htmx", handlers.HandleShowIssuesHTMX)                    // GET

	// Web routes
	mux.HandleFunc("/", handleHome)
	mux.HandleFunc("GET /settings", handleSettingsPage) // Changed to GET specific
	// HTMX routes for settings page partials/actions
	mux.HandleFunc("GET /settings/tabs/{tab}", handlers.HandleSettingsTab)
	mux.HandleFunc("POST /settings/providers", handlers.HandleAddProvider)              // Adds an item to the list
	mux.HandleFunc("DELETE /settings/providers/{index}", handlers.HandleRemoveProvider) // Removes an item
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
func methodsHandler(handlers map[string]http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		if handler, ok := handlers[r.Method]; ok {
			handler(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// PageData moved to handlers package

func handleHome(w http.ResponseWriter, r *http.Request) {
	// Only handle exact "/" path to avoid conflicts with other routes
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	state.State.Mu.RLock()
	data := handlers.PageData{
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

	// Get current tab from query param or default to general
	currentTab := r.URL.Query().Get("tab")
	if currentTab == "" {
		currentTab = "general"
	}
	data := handlers.PageData{
		Page:       "settings",
		Settings:   state.State.Settings,
		CurrentTab: currentTab,
	}

	err := templates.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
