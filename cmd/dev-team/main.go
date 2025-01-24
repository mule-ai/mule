package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"dev-team/internal/config"
	"dev-team/internal/settings"
	"dev-team/internal/state"
	"dev-team/pkg/genai"
	"dev-team/pkg/github"
	"dev-team/pkg/repository"

	git "github.com/go-git/go-git/v5"
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
		log.Fatal(err)
	}
}

func main() {
	appState, err := config.LoadConfig()
	if err != nil {
		log.Fatal(err)
	}
	state.State = appState

	r := mux.NewRouter()

	// API routes
	api := r.PathPrefix("/api").Subrouter()
	api.HandleFunc("/repositories", handleListRepositories).Methods("GET")
	api.HandleFunc("/repositories", handleAddRepository).Methods("POST")
	api.HandleFunc("/repositories/update", handleUpdateRepository).Methods("POST")
	api.HandleFunc("/repositories/clone", handleCloneRepository).Methods("POST")
	api.HandleFunc("/repositories/commit", handleCommit).Methods("POST")
	api.HandleFunc("/repositories/push", handlePush).Methods("POST")
	api.HandleFunc("/repositories/pr", handleCreatePR).Methods("POST")
	api.HandleFunc("/settings", handleGetSettings).Methods("GET")
	api.HandleFunc("/settings", handleUpdateSettings).Methods("POST")
	api.HandleFunc("/gemini/models", handleGeminiModels).Methods("GET")
	api.HandleFunc("/github/repositories", handleGitHubRepositories).Methods("GET")
	api.HandleFunc("/github/issues", handleGitHubIssues).Methods("GET")

	// Web routes
	r.HandleFunc("/", handleHome).Methods("GET")
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
	log.Printf("Server starting on http://0.0.0.0:8083")
	log.Fatal(http.ListenAndServe("0.0.0.0:8083", handler))
}

type PageData struct {
	Page         string
	Repositories map[string]*repository.Repository
	Settings     settings.Settings
}

type RepoAddRequest struct {
	RepoURL  string `json:"repoUrl"`
	BasePath string `json:"path"`
	Schedule string `json:"schedule"`
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

func handleListRepositories(w http.ResponseWriter, r *http.Request) {
	state.State.Mu.RLock()
	defer state.State.Mu.RUnlock()

	json.NewEncoder(w).Encode(state.State.Repositories)
}

func handleAddRepository(w http.ResponseWriter, r *http.Request) {
	var req RepoAddRequest
	log.Printf("Adding repository: %v", r.Body)

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	repoName := strings.TrimPrefix(req.RepoURL, "https://github.com/")
	repoName = strings.TrimSuffix(repoName, ".git")
	repoPath := filepath.Join(req.BasePath, repoName)
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}
	repo := repository.Repository{Path: absPath, Schedule: req.Schedule}

	_, err = git.PlainOpen(repo.Path)
	if err != nil {
		http.Error(w, "Invalid git repository path", http.StatusBadRequest)
		return
	}

	log.Printf("Getting repo status for %s", repo.Path)

	updateRepo(&repo, repo.Path)

	log.Printf("Adding scheduler task for %s", repo.Path)

	// Set up scheduler for the repository
	err = state.State.Scheduler.AddTask(repo.Path, repo.Schedule, func() {
		state.State.Mu.RLock()
		aiService := state.State.Settings.GetAIService()
		state.State.Mu.RUnlock()
		err := repo.Sync(aiService, state.State.Settings.GitHubToken)
		if err != nil {
			log.Printf("Error syncing repo: %v", err)
		}
		state.State.Mu.Lock()
		state.State.Repositories[repo.Path] = &repo
		state.State.Mu.Unlock()
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error setting up schedule: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Saving config")
	err = config.SaveConfig()
	if err != nil {
		log.Printf("Error saving config: %v", err)
		http.Error(w, fmt.Sprintf("Error saving config: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	log.Printf("Repository added successfully")
}

func handleUpdateRepository(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	path := req.Path

	absPath, err := filepath.Abs(path)
	if err != nil {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	state.State.Mu.RLock()
	repo, exists := state.State.Repositories[absPath]
	state.State.Mu.RUnlock()

	if !exists {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	// Perform fetch
	err = repo.Fetch()
	if err != nil && err != git.NoErrAlreadyUpToDate {
		log.Printf("Warning: fetch error: %v", err)
	}

	updateRepo(repo, absPath)

	json.NewEncoder(w).Encode(repo.State)
}

func handleCommit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	path := req.Path

	absPath, err := filepath.Abs(path)
	if err != nil {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	state.State.Mu.RLock()
	repo, exists := state.State.Repositories[absPath]
	state.State.Mu.RUnlock()

	if !exists {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	state.State.Mu.RLock()
	aiService := state.State.Settings.GetAIService()
	state.State.Mu.RUnlock()

	summary, err := repo.ChangeSummary()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting change summary: %v", err), http.StatusInternalServerError)
		return
	}

	commitMessage, err := genai.Chat(repository.CommitPrompt(summary), aiService)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error generating commit message: %v", err), http.StatusInternalServerError)
		return
	}

	err = repo.Commit(commitMessage)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error committing changes: %v", err), http.StatusInternalServerError)
		return
	}

	updateRepo(repo, absPath)

	json.NewEncoder(w).Encode(repo.State)
}

func handlePush(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	path := req.Path

	absPath, err := filepath.Abs(path)
	if err != nil {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	state.State.Mu.RLock()
	repo, exists := state.State.Repositories[absPath]
	state.State.Mu.RUnlock()

	if !exists {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	err = repo.Push()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error pushing changes: %v", err), http.StatusInternalServerError)
		return
	}

	updateRepo(repo, absPath)

	w.WriteHeader(http.StatusOK)
}

func handleCreatePR(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	absPath, err := filepath.Abs(req.Path)
	if err != nil {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	state.State.Mu.RLock()
	repo, exists := state.State.Repositories[absPath]
	state.State.Mu.RUnlock()

	if !exists {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}
	state.State.Mu.RLock()
	settings := &state.State.Settings
	state.State.Mu.RUnlock()

	aiService := settings.GetAIService()

	summary, err := repo.ChangeSummary()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting change summary: %v", err), http.StatusInternalServerError)
		return
	}

	prTitle, err := genai.Chat(repository.CommitPrompt(summary), aiService)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error generating PR title: %v", err), http.StatusInternalServerError)
		return
	}

	prDescription, err := genai.Chat(repository.PRPrompt(summary), aiService)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error generating PR description: %v", err), http.StatusInternalServerError)
		return
	}

	state.State.Mu.RLock()
	defer state.State.Mu.RUnlock()

	err = github.CreateDraftPR(absPath, settings.GitHubToken, github.GitHubPRInput{
		Title:               prTitle,
		Branch:              repo.State.CurrentBranch,
		Base:                "main",
		Description:         prDescription,
		Draft:               true,
		MaintainerCanModify: true,
	})
	if err != nil {
		log.Printf("Error creating PR: %v", err)
		http.Error(w, fmt.Sprintf("Error creating PR: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func handleGetSettings(w http.ResponseWriter, r *http.Request) {
	state.State.Mu.RLock()
	defer state.State.Mu.RUnlock()

	json.NewEncoder(w).Encode(state.State.Settings)
}

func handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var settings settings.Settings
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	state.State.Mu.Lock()
	state.State.Settings = settings
	state.State.Mu.Unlock()

	if err := config.SaveConfig(); err != nil {
		http.Error(w, fmt.Sprintf("Error saving config: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func handleGeminiModels(w http.ResponseWriter, r *http.Request) {
	state.State.Mu.RLock()
	settings := state.State.Settings
	state.State.Mu.RUnlock()

	if settings.GeminiAPIKey == "" {
		http.Error(w, "Gemini API key not configured", http.StatusBadRequest)
		return
	}

	models, err := genai.GetGeminiModels(settings.GeminiAPIKey)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching Gemini models: %v", err), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(models)
}

func handleGitHubRepositories(w http.ResponseWriter, r *http.Request) {
	token := r.Header.Get("Authorization")
	if token == "" {
		http.Error(w, "GitHub token not provided", http.StatusBadRequest)
		return
	}
	token = strings.TrimPrefix(token, "Bearer ")

	repos, err := github.FetchRepositories(token)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching repositories: %v", err), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(repos)
}

func handleCloneRepository(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RepoURL  string `json:"repoUrl"`
		BasePath string `json:"basePath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.RepoURL == "" || req.BasePath == "" {
		http.Error(w, "Repository URL and base path are required", http.StatusBadRequest)
		return
	}

	// Create the base path if it doesn't exist
	if err := os.MkdirAll(req.BasePath, 0755); err != nil {
		http.Error(w, fmt.Sprintf("Error creating directory: %v", err), http.StatusInternalServerError)
		return
	}

	// Clone the repository
	repoName := strings.TrimPrefix(req.RepoURL, "https://github.com/")
	repoName = strings.TrimSuffix(repoName, ".git")
	repoPath := filepath.Join(req.BasePath, repoName)
	repo := repository.Repository{Path: repoPath}
	if err := repo.Clone(req.RepoURL); err != nil {
		http.Error(w, fmt.Sprintf("Error cloning repository: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func updateRepo(repo *repository.Repository, absPath string) {
	// Get updated status
	err := repo.UpdateStatus()
	if err != nil {
		log.Printf("Error getting repo status: %v", err)
		return
	}

	state.State.Mu.Lock()
	state.State.Repositories[absPath] = repo
	state.State.Mu.Unlock()
}

func handleGitHubIssues(w http.ResponseWriter, r *http.Request) {
	owner := r.URL.Query().Get("owner")
	repo := r.URL.Query().Get("repo")
	if owner == "" || repo == "" {
		http.Error(w, "Owner and repo parameters are required", http.StatusBadRequest)
		return
	}

	state.State.Mu.RLock()
	token := state.State.Settings.GitHubToken
	state.State.Mu.RUnlock()

	if token == "" {
		http.Error(w, "GitHub token not configured", http.StatusBadRequest)
		return
	}

	issues, err := github.FetchIssues(owner, repo, "dev-team", token)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching issues: %v", err), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(issues)
}
