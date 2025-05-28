package main

import (
	"embed"
	"flag"
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/go-logr/logr"
	"github.com/mule-ai/mule/internal/config"
	"github.com/mule-ai/mule/internal/handlers"
	"github.com/mule-ai/mule/internal/settings"
	"github.com/mule-ai/mule/internal/state"
	"github.com/mule-ai/mule/pkg/agent"
	"github.com/mule-ai/mule/pkg/log"
	"github.com/mule-ai/mule/pkg/repository"

	// Import integrations to register them
	_ "github.com/mule-ai/mule/pkg/integration/grpc"

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

	// read flags
	serverMode := flag.Bool("server", false, "run in server mode")
	workflow := flag.String("workflow", "", "workflow to run")
	prompt := flag.String("prompt", "", "prompt to run workflow with")
	flag.Parse()
	if *serverMode {
		server(l)
	} else if *workflow != "" {
		l = log.NewStdoutLogger()
		runWorkflow(l, *workflow, *prompt)
	} else {
		l.Error(nil, "No server mode or workflow specified")
	}
}

func runWorkflow(l logr.Logger, workflowName string, prompt string) {
	if prompt == "" {
		l.Error(nil, "No prompt specified")
		return
	}

	// load workflow
	workflow, ok := state.State.Workflows[workflowName]
	if !ok {
		workflowOptions := make([]string, 0, len(state.State.Workflows))
		for name := range state.State.Workflows {
			workflowOptions = append(workflowOptions, name)
		}
		l.Error(nil, "Workflow not found", "options", workflowOptions)
		return
	}

	l.Info(fmt.Sprintf("Running workflow: %s with prompt: %s", workflowName, prompt))
	// run workflow
	results, err := workflow.ExecuteWorkflow(workflow.Steps, state.State.Agents, agent.PromptInput{
		Message: prompt,
	}, "", l, workflow.ValidationFunctions)
	if err != nil {
		l.Error(err, "Error executing workflow")
	}
	finalResult, ok := results["final"]
	if !ok || finalResult.Error != nil || finalResult.Content == "" {
		l.Error(fmt.Errorf("final result not found"), "Final result not found")
		finalResult.Content = "An error occurred while executing the workflow, please try again."
	}
	l.Info(fmt.Sprintf("Workflow result: %s", finalResult.Content))
}

func server(l logr.Logger) {
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
func methodsHandler(handlers map[string]http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {

		if handler, ok := handlers[r.Method]; ok {
			handler(w, r)
			return
		}
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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
