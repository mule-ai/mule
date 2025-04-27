package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/go-co-op/gocron"
	"github.com/go-git/go-git/v5"

	"github.com/mule-ai/mule/internal/config"
	"github.com/mule-ai/mule/internal/scheduler"
	"github.com/mule-ai/mule/internal/state"
	"github.com/mule-ai/mule/pkg/log"
	"github.com/mule-ai/mule/pkg/remote"
	"github.com/mule-ai/mule/pkg/repository"
)

// --- Internal Logic Functions ---

func cloneRepositoryInternal(repoURL, absPath string) error {
	log.Infof("Cloning repository %s into %s", repoURL, absPath)
	_, err := repository.CloneRepository(repoURL, absPath)
	if err != nil {
		log.Errorf("Error cloning repository %s: %v", repoURL, err)
		return fmt.Errorf("error cloning repository: %w", err)
	}
	log.Infof("Repository %s cloned successfully to %s", repoURL, absPath)
	return nil
}

func addRepositoryInternal(repoURL, absPath, schedule string) error {
	// Validate schedule format
	_, err := gocron.NewScheduler(time.UTC).Cron(schedule).Tag("validation").Do(func() {})
	if err != nil {
		return fmt.Errorf("invalid schedule format '%s': %w", schedule, err)
	}

	state.State.Mu.Lock()
	defer state.State.Mu.Unlock()

	if _, exists := state.State.Repositories[absPath]; exists {
		return fmt.Errorf("repository already exists at path %s", absPath)
	}

	// Determine default provider based on URL? Or require explicit setting later?
	// For now, default to GitHub if URL contains "github.com", else "local"
	defaultProvider := "local"
	if strings.Contains(repoURL, "github.com") {
		defaultProvider = "github"
	}

	provider, err := remote.NewProvider(defaultProvider, state.State.Settings) // Use default provider initially
	if err != nil {
		return fmt.Errorf("failed to initialize provider '%s': %w", defaultProvider, err)
	}

	repo := repository.NewRepository(absPath, repoURL, schedule, provider)
	state.State.Repositories[absPath] = repo
	scheduler.AddOrUpdateJob(state.State.Scheduler, repo)

	log.Infof("Repository %s added with schedule '%s'", absPath, schedule)

	// Persist changes to config
	cfg := config.Config{
		Repositories: make([]config.RepositoryConfig, 0, len(state.State.Repositories)),
		Settings:     state.State.Settings,
	}
	for _, r := range state.State.Repositories {
		cfg.Repositories = append(cfg.Repositories, config.RepositoryConfig{
			Path:     r.Path,
			URL:      r.URL,
			Schedule: r.Schedule,
			Provider: r.RemoteProvider.Provider,
		})
	}
	if err := config.SaveConfig(cfg); err != nil {
		// Log the error but maybe don't fail the request? Or should we rollback?
		log.Errorf("Failed to save config after adding repository: %v", err)
		// return fmt.Errorf("failed to save config: %w", err) // Or just log
	}

	return nil
}

// Combined clone and add function
func cloneAndAddRepository(repoURL, absPath, schedule string) error {
	err := cloneRepositoryInternal(repoURL, absPath)
	if err != nil {
		return err // Return cloning error
	}
	err = addRepositoryInternal(repoURL, absPath, schedule)
	if err != nil {
		// Consider if we should attempt to remove the cloned repo if adding fails
		log.Errorf("Repository cloned to %s but failed to add to tracking: %v", absPath, err)
		// os.RemoveAll(absPath) // Potential cleanup? Risky if path wasn't empty before.
		return err // Return adding error
	}
	return nil
}

func removeRepositoryInternal(absPath string) error {
	state.State.Mu.Lock()
	defer state.State.Mu.Unlock()

	repo, exists := state.State.Repositories[absPath]
	if !exists {
		return fmt.Errorf("repository not found at path %s", absPath)
	}

	scheduler.RemoveJob(state.State.Scheduler, repo)
	delete(state.State.Repositories, absPath)

	log.Infof("Repository %s removed", absPath)

	// Persist changes
	cfg := config.Config{
		Repositories: make([]config.RepositoryConfig, 0, len(state.State.Repositories)),
		Settings:     state.State.Settings,
	}
	for _, r := range state.State.Repositories {
		cfg.Repositories = append(cfg.Repositories, config.RepositoryConfig{
			Path:     r.Path,
			URL:      r.URL,
			Schedule: r.Schedule,
			Provider: r.RemoteProvider.Provider,
		})
	}
	if err := config.SaveConfig(cfg); err != nil {
		log.Errorf("Failed to save config after removing repository: %v", err)
		// return fmt.Errorf("failed to save config: %w", err) // Or just log
	}

	return nil
}

func syncRepositoryInternal(absPath string) error {
	state.State.Mu.RLock()
	repo, exists := state.State.Repositories[absPath]
	state.State.Mu.RUnlock()

	if !exists {
		return fmt.Errorf("repository not found at path %s", absPath)
	}

	log.Infof("Manual sync requested for repository %s", absPath)
	// Execute the sync job immediately
	scheduler.RunJobNow(state.State.Scheduler, repo)

	// Note: RunJobNow might run async depending on gocron setup.
	// The status update in the UI might not be immediate if the job takes time.
	// Consider ways to poll or use websockets for real-time updates if needed.
	// For HTMX, returning the updated card *after* the sync should be okay if sync is fast.
	// If sync is slow, the card returned might still show old state until the *next* refresh.

	// We need to wait or get status. Let's assume RunJobNow is synchronous for simplicity.
	// Fetch updated state after sync (the sync job should update repo.State, repo.LastSync etc.)
	repo.UpdateState() // Manually update state after sync for immediate reflection

	log.Infof("Manual sync completed for repository %s", absPath)
	return nil
}

func setRepositoryProviderInternal(absPath, providerName string) error {
	state.State.Mu.Lock()
	defer state.State.Mu.Unlock()

	repo, exists := state.State.Repositories[absPath]
	if !exists {
		return fmt.Errorf("repository not found at path %s", absPath)
	}

	newProvider, err := remote.NewProvider(providerName, state.State.Settings)
	if err != nil {
		return fmt.Errorf("failed to initialize provider '%s': %w", providerName, err)
	}

	repo.RemoteProvider = newProvider
	log.Infof("Provider set to '%s' for repository %s", providerName, absPath)

	// Persist changes
	cfg := config.Config{
		Repositories: make([]config.RepositoryConfig, 0, len(state.State.Repositories)),
		Settings:     state.State.Settings,
	}
	for _, r := range state.State.Repositories {
		cfg.Repositories = append(cfg.Repositories, config.RepositoryConfig{
			Path:     r.Path,
			URL:      r.URL,
			Schedule: r.Schedule,
			Provider: r.RemoteProvider.Provider, // Ensure this reflects the change
		})
	}
	if err := config.SaveConfig(cfg); err != nil {
		log.Errorf("Failed to save config after setting provider: %v", err)
		// return fmt.Errorf("failed to save config: %w", err) // Or just log
	}

	return nil
}

// --- Existing HTTP Handlers (Can be removed or kept alongside HTMX handlers) ---

// RepoAddRequest structure for JSON API
type RepoAddRequest struct {
	RepoURL  string `json:"repoUrl"`
	BasePath string `json:"basePath"` // Renamed from path for clarity vs absPath
	Schedule string `json:"schedule"`
}

func HandleListRepositories(w http.ResponseWriter, r *http.Request) {
	state.State.Mu.RLock()
	defer state.State.Mu.RUnlock()

	// Create a slice and sort for consistent ordering
	reposSlice := make([]*repository.Repository, 0, len(state.State.Repositories))
	for _, repo := range state.State.Repositories {
		reposSlice = append(reposSlice, repo)
	}
	sort.Slice(reposSlice, func(i, j int) bool {
		return reposSlice[i].Path < reposSlice[j].Path
	})

	w.Header().Set("Content-Type", "application/json")
	err := json.NewEncoder(w).Encode(reposSlice)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// HandleRepositories combines Add and Delete for the /api/repositories endpoint (JSON)
// GET is handled by HandleListRepositories
// PUT for provider change is added
func HandleRepositories(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost: // Add repository
		handleAddRepository(w, r)
	case http.MethodDelete: // Delete repository
		handleRemoveRepository(w, r)
	case http.MethodPut: // Change provider
		handleSetProvider(w, r)
	default:
		http.Error(w, "Method not allowed for /api/repositories", http.StatusMethodNotAllowed)
	}
}

func HandleAddRepository(w http.ResponseWriter, r *http.Request) {
	var req RepoAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// BasePath in the request is the *directory* where the repo should be cloned INTO.
	// We need to derive the actual repo directory name from the URL.
	repoName := req.RepoURL
	if strings.HasSuffix(repoName, ".git") {
		repoName = strings.TrimSuffix(repoName, ".git")
	}
	parts := strings.Split(repoName, "/")
	if len(parts) > 0 {
		repoName = parts[len(parts)-1]
	}

	absBasePath, err := filepath.Abs(req.BasePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid base path: %v", err), http.StatusBadRequest)
		return
	}

	// The actual path where the repository will reside
	absRepoPath := filepath.Join(absBasePath, repoName)

	err = addRepositoryInternal(req.RepoURL, absRepoPath, req.Schedule)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError) // Use specific codes? 409 for exists?
		return
	}

	w.WriteHeader(http.StatusCreated)
	fmt.Fprintf(w, "Repository %s added successfully", absRepoPath) // Return path in response
}

func HandleUpdateRepository(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// This handler seems intended to just fetch latest git status for an existing repo
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

	w.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(w).Encode(repo.State) // Return the updated state
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
	defer r.Body.Close()

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
	repoName := req.RepoURL
	if strings.HasSuffix(repoName, ".git") {
		repoName = strings.TrimSuffix(repoName, ".git")
	}
	parts := strings.Split(repoName, "/")
	if len(parts) > 0 {
		repoName = parts[len(parts)-1]
	}
	absBasePath, err := filepath.Abs(req.BasePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid base path: %v", err), http.StatusBadRequest)
		return
	}

	// The actual path where the repository will reside
	absRepoPath := filepath.Join(absBasePath, repoName)

	err = cloneRepositoryInternal(req.RepoURL, absRepoPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error cloning repository: %v", err), http.StatusInternalServerError)
		return
	}

	// Note: This only clones, doesn't add to tracked repositories.
	// The AddRepository handler should typically handle both clone and add.
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Repository %s cloned successfully to %s", req.RepoURL, absRepoPath)
}

func handleRemoveRepository(w http.ResponseWriter, r *http.Request) {
	// Get repository path from URL
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "Path parameter is required", http.StatusBadRequest)
		return
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid path: %v", err), http.StatusBadRequest)
		return
	}

	err = removeRepositoryInternal(absPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError) // Use 404 if not found?
		return
	}

	log.Printf("repository deleted %s", absPath)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Repository %s removed successfully", absPath)
}

func HandleSyncRepository(w http.ResponseWriter, r *http.Request) {
	// Get repository path from URL
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "Repository path is required", http.StatusBadRequest)
		return
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid path: %v", err), http.StatusBadRequest)
		return
	}

	err = syncRepositoryInternal(absPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Sync triggered for repository %s", absPath)
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
	defer r.Body.Close()

	if req.Path == "" || req.Provider == "" {
		http.Error(w, "Missing required fields: path, provider", http.StatusBadRequest)
		return
	}

	absPath, err := filepath.Abs(req.Path)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid path: %v", err), http.StatusBadRequest)
		return
	}

	err = setRepositoryProviderInternal(absPath, req.Provider)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Provider set to '%s' for repository %s", req.Provider, absPath)
}

// handleSetProvider is the internal implementation used by HandleRepositories (JSON PUT)
func handleSetProvider(w http.ResponseWriter, r *http.Request) {
	HandleSwitchProvider(w, r) // Reuses the logic
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
		return nil, fmt.Errorf("repository does not exist at %s", absPath)
	}
	return repo, nil
}

func updateRepo(repo *repository.Repository) {
	// Get updated status
	err := repo.UpdateState()
	if err != nil {
		log.Printf("Error getting repo status for %s: %v", repo.Path, err)
		// Don't return, just log the error, state might be partially updated
	}

	// This function updates the repo's in-memory state (like branch, changes)
	// No need to update the map again, as we have the pointer
	// state.State.Mu.Lock()
	// state.State.Repositories[repo.Path] = repo
	// state.State.Mu.Unlock()
}
