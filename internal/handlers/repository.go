package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-git/go-git/v5"

	"github.com/mule-ai/mule/internal/config"
	"github.com/mule-ai/mule/internal/state"
	"github.com/mule-ai/mule/pkg/remote"
	"github.com/mule-ai/mule/pkg/repository"
)

type RepoAddRequest struct {
	RepoURL  string `json:"repoUrl"`
	BasePath string `json:"path"`
	Schedule string `json:"schedule"`
}

func HandleListRepositories(w http.ResponseWriter, r *http.Request) {
	state.State.Mu.RLock()
	defer state.State.Mu.RUnlock()

	err := json.NewEncoder(w).Encode(state.State.Repositories)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
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
	repo := repository.NewRepository(absPath)
	repo.Schedule = req.Schedule
	repo.RemotePath = repoName

	_, err = git.PlainOpen(repo.Path)
	if err != nil {
		http.Error(w, "Invalid git repository path", http.StatusBadRequest)
		return
	}

	log.Printf("Getting repo status for %s", repo.Path)

	updateRepo(repo)

	log.Printf("Adding scheduler task for %s", repo.Path)

	// Set up scheduler for the repository
	err = state.State.Scheduler.AddTask(repo.Path, repo.Schedule, func() {
		err := repo.Sync(state.State.Agents)
		if err != nil {
			log.Printf("Error syncing repo: %v", err)
		}
		state.State.Mu.Lock()
		state.State.Repositories[repo.Path] = repo
		state.State.Mu.Unlock()
	})
	if err != nil {
		http.Error(w, fmt.Sprintf("Error setting up schedule: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Saving config")
	// Create config path
	configPath, err := config.GetHomeConfigPath()
	if err != nil {
		log.Printf("Error getting config path: %v", err)
	}
	err = config.SaveConfig(configPath)
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

	err = json.NewEncoder(w).Encode(repo.State)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
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
	repo := repository.NewRepository(repoPath)
	if err := repo.Upsert(req.RepoURL); err != nil {
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

	// Create config path
	configPath, err := config.GetHomeConfigPath()
	if err != nil {
		log.Printf("Error getting config path: %v", err)
	}
	err = config.SaveConfig(configPath)
	if err != nil {
		log.Printf("Error getting user home directory: %v", err)
	}
	configPath := filepath.Join(homeDir, config.ConfigPath)
	err = config.SaveConfig(configPath)
	if err != nil {
		log.Printf("Error saving config: %v", err)
		http.Error(w, fmt.Sprintf("Error saving config: %v", err), http.StatusInternalServerError)
		return
	}

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

	err = repo.Sync(state.State.Agents)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func HandleSwitchProvider(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path     string `json:"path"`
		Provider string `json:"provider"`
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

	// Update the provider
	repo.RemoteProvider.Provider = req.Provider
	repo.RemoteProvider.Path = req.Path
	repo.RemoteProvider.Token = state.State.Settings.GitHubToken

	// Create new remote provider
	options, err := remote.SettingsToOptions(repo.RemoteProvider)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error converting settings: %v", err), http.StatusInternalServerError)
		return
	}
	repo.Remote = remote.New(options)

	// Update state
	state.State.Mu.Lock()
	state.State.Repositories[repo.Path] = repo
	state.State.Mu.Unlock()

	// Save config
	configPath, err := config.GetHomeConfigPath()
	if err != nil {
		log.Printf("Error getting config path: %v", err)
		http.Error(w, fmt.Sprintf("Error getting config path: %v", err), http.StatusInternalServerError)
		return
	}
	err = config.SaveConfig(configPath)
	if err != nil {
		log.Printf("Error saving config: %v", err)
		http.Error(w, fmt.Sprintf("Error saving config: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func getRepository(path string) (*repository.Repository, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		log.Printf("error getting absolute path: %v", err)
		return nil, err
	}

	state.State.Mu.RLock()
	repo, exists := state.State.Repositories[absPath]
	state.State.Mu.RUnlock()

	if !exists {
		log.Printf("repository does not exist: %s", absPath)
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
