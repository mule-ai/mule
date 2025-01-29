package handlers

import (
	"dev-team/internal/config"
	"dev-team/internal/state"
	"dev-team/pkg/github"
	"dev-team/pkg/repository"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"
)

type RepoAddRequest struct {
	RepoURL  string `json:"repoUrl"`
	BasePath string `json:"path"`
	Schedule string `json:"schedule"`
}

func HandleListRepositories(w http.ResponseWriter, r *http.Request) {
	state.State.Mu.RLock()
	defer state.State.Mu.RUnlock()

	json.NewEncoder(w).Encode(state.State.Repositories)
}

func HandleAddRepository(w http.ResponseWriter, r *http.Request) {
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
	repo := repository.Repository{
		Path:         absPath,
		Schedule:     req.Schedule,
		RemotePath:   repoName,
		Issues:       make(map[int]repository.Issue),
		PullRequests: make(map[int]repository.PullRequest),
	}

	_, err = git.PlainOpen(repo.Path)
	if err != nil {
		http.Error(w, "Invalid git repository path", http.StatusBadRequest)
		return
	}

	log.Printf("Getting repo status for %s", repo.Path)

	updateRepo(&repo)

	log.Printf("Adding scheduler task for %s", repo.Path)

	// Set up scheduler for the repository
	err = state.State.Scheduler.AddTask(repo.Path, repo.Schedule, func() {
		err := repo.Sync(state.State.GenAI, state.State.Settings.GitHubToken)
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

func HandleUpdateRepository(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	repo, err := getRepository(req.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Perform fetch
	err = repo.Fetch()
	if err != nil && err != git.NoErrAlreadyUpToDate {
		log.Printf("Warning: fetch error: %v", err)
	}

	updateRepo(repo)

	json.NewEncoder(w).Encode(repo.State)
}

func HandleCommit(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	repo, err := getRepository(req.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	summary, err := repo.ChangeSummary()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting change summary: %v", err), http.StatusInternalServerError)
		return
	}

	commitMessage, err := state.State.GenAI.Generate(
		state.State.Settings.Model,
		repository.CommitPrompt(summary),
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error generating commit message: %v", err), http.StatusInternalServerError)
		return
	}

	err = repo.Commit(commitMessage)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error committing changes: %v", err), http.StatusInternalServerError)
		return
	}

	updateRepo(repo)

	json.NewEncoder(w).Encode(repo.State)
}

func HandlePush(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	repo, err := getRepository(req.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	err = repo.Push()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error pushing changes: %v", err), http.StatusInternalServerError)
		return
	}

	updateRepo(repo)

	w.WriteHeader(http.StatusOK)
}

func HandleCreatePR(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	repo, err := getRepository(req.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	state.State.Mu.RLock()
	settings := &state.State.Settings
	state.State.Mu.RUnlock()

	summary, err := repo.ChangeSummary()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error getting change summary: %v", err), http.StatusInternalServerError)
		return
	}

	prTitle, err := state.State.GenAI.Generate(
		settings.Model,
		repository.CommitPrompt(summary),
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error generating PR title: %v", err), http.StatusInternalServerError)
		return
	}

	prDescription, err := state.State.GenAI.Generate(
		settings.Model,
		repository.PRPrompt(summary),
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error generating PR description: %v", err), http.StatusInternalServerError)
		return
	}

	err = github.CreateDraftPR(repo.Path, settings.GitHubToken, github.GitHubPRInput{
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

func HandleCloneRepository(w http.ResponseWriter, r *http.Request) {
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

func HandleDeleteRepository(w http.ResponseWriter, r *http.Request) {
	// Get repository path from URL
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "Path parameter is required", http.StatusBadRequest)
		return
	}

	repo, err := getRepository(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	state.State.Mu.Lock()
	delete(state.State.Repositories, repo.Path)
	state.State.Scheduler.RemoveTask(repo.Path)
	state.State.Mu.Unlock()

	config.SaveConfig()

	log.Printf("repository deleted %s", repo.Path)
	w.WriteHeader(http.StatusOK)
}

func HandleSyncRepository(w http.ResponseWriter, r *http.Request) {
	// Get repository path from URL
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "Repository path is required", http.StatusBadRequest)
		return
	}

	repo, err := getRepository(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	token := state.State.Settings.GitHubToken

	err = repo.Sync(state.State.GenAI, token)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func getRepository(path string) (*repository.Repository, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	state.State.Mu.RLock()
	repo, exists := state.State.Repositories[absPath]
	state.State.Mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("repository does not exist")
	}
	return repo, nil
}

func updateRepo(repo *repository.Repository) {
	// Get updated status
	err := repo.UpdateStatus()
	if err != nil {
		log.Printf("Error getting repo status: %v", err)
		return
	}

	state.State.Mu.Lock()
	state.State.Repositories[repo.Path] = repo
	state.State.Mu.Unlock()
}
