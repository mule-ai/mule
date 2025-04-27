package handlers

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/mule-ai/mule/internal/config"
	"github.com/mule-ai/mule/internal/state"
	"github.com/mule-ai/mule/pkg/repository"
)

// renderTemplate executes a specific template with the given data and writes it to the writer.
func renderTemplate(w http.ResponseWriter, templateName string, data interface{}) {
	buf := &bytes.Buffer{}
	err := templates.ExecuteTemplate(buf, templateName, data)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error rendering template %s: %v", templateName, err), http.StatusInternalServerError)
		return
	}
	_, err = buf.WriteTo(w)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error writing template %s to response: %v", templateName, err), http.StatusInternalServerError)
	}
}

// HandleGitHubRepositoriesHTMX fetches GitHub repositories and returns them as <option> elements.
func HandleGitHubRepositoriesHTMX(w http.ResponseWriter, r *http.Request) {
	state.State.Mu.RLock()
	remote := state.State.Remote
	state.State.Mu.RUnlock()

	if remote == nil || remote.GitHub == nil {
		// Render empty options if GitHub is not configured
		renderTemplate(w, "repositoryOptions", []repository.RemoteRepository{})
		return
	}

	repos, err := remote.GitHub.FetchRepositories()
	if err != nil {
		// Optionally, render an error message within the select?
		// For now, just render empty. User will see error in logs or potentially an alert via htmx:responseError event.
		http.Error(w, fmt.Sprintf("Error fetching repositories: %v", err), http.StatusInternalServerError)
		renderTemplate(w, "repositoryOptions", []repository.RemoteRepository{})
		return
	}

	// Add default option plus fetched repos
	data := struct {
		Repos []repository.RemoteRepository
	}{
		Repos: repos,
	}

	renderTemplate(w, "repositoryOptions", data)
}

// HandleRepositoriesListHTMX returns the full list of repository cards.
func HandleRepositoriesListHTMX(w http.ResponseWriter, r *http.Request) {
	state.State.Mu.RLock()
	// Create a slice from the map for sorting
	reposSlice := make([]*repository.Repository, 0, len(state.State.Repositories))
	for _, repo := range state.State.Repositories {
		reposSlice = append(reposSlice, repo)
	}
	state.State.Mu.RUnlock()

	// Sort by path for consistent order
	sort.Slice(reposSlice, func(i, j int) bool {
		return reposSlice[i].Path < reposSlice[j].Path
	})

	data := struct {
		Repositories []*repository.Repository
	}{
		Repositories: reposSlice,
	}
	renderTemplate(w, "repositoriesList", data) // Target is #repositories
}

// HandleAddRepositoryHTMX adds a repository and returns the updated list of repository cards.
func HandleAddRepositoryHTMX(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}

	repoURL := r.FormValue("remoteRepository")
	basePath := r.FormValue("basePath")
	schedule := r.FormValue("schedule")

	if repoURL == "" || basePath == "" || schedule == "" {
		http.Error(w, "Missing required fields: remoteRepository, basePath, schedule", http.StatusBadRequest)
		return
	}

	absPath, err := filepath.Abs(basePath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid base path: %v", err), http.StatusBadRequest)
		return
	}

	// Perform the clone and add operations (similar to handleAddRepository)
	err = cloneAndAddRepository(repoURL, absPath, schedule)
	if err != nil {
		// Send back an error message that HTMX can display
		// Use HX-Retarget and HX-Reswap headers to target an error message area
		w.Header().Set("HX-Retarget", "#errorMessage") // Assuming an element with id="errorMessage" exists
		w.Header().Set("HX-Reswap", "innerHTML")
		http.Error(w, fmt.Sprintf("Error adding repository: %v", err), http.StatusInternalServerError)
		return
	}

	// Return the updated list of repositories
	HandleRepositoriesListHTMX(w, r)
}

// HandleDeleteRepositoryHTMX deletes a repository. HTMX handles removing the element on success.
func HandleDeleteRepositoryHTMX(w http.ResponseWriter, r *http.Request) {
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

	err = removeRepositoryInternal(absPath) // Reuse logic from HandleRemoveRepository
	if err != nil {
		// Return an error status code; HTMX might show a generic error or trigger htmx:responseError
		http.Error(w, fmt.Sprintf("Error removing repository: %v", err), http.StatusInternalServerError)
		return
	}

	// On success, return 200 OK. HTMX will remove the targeted element.
	w.WriteHeader(http.StatusOK)
}

// HandleSyncRepositoryHTMX triggers a sync and returns the updated repository card.
func HandleSyncRepositoryHTMX(w http.ResponseWriter, r *http.Request) {
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

	err = syncRepositoryInternal(absPath) // Reuse logic from HandleSyncRepository
	if err != nil {
		http.Error(w, fmt.Sprintf("Error syncing repository: %v", err), http.StatusInternalServerError)
		// Optionally return the card in its previous state or an error fragment
		// For now, just return error. The UI might stay in 'syncing' state incorrectly.
		return
	}

	// Fetch the updated repo state
	state.State.Mu.RLock()
	repo, exists := state.State.Repositories[absPath]
	state.State.Mu.RUnlock()

	if !exists {
		http.Error(w, "Repository disappeared after sync", http.StatusInternalServerError)
		return
	}

	// Return the updated card
	renderTemplate(w, "repositoryCard", repo)
}

// HandleSetProviderHTMX sets the provider for a repository and returns the updated card.
func HandleSetProviderHTMX(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		return
	}
	provider := r.FormValue("provider")

	if path == "" || provider == "" {
		http.Error(w, "Path and provider parameters are required", http.StatusBadRequest)
		return
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid path: %v", err), http.StatusBadRequest)
		return
	}

	err = setRepositoryProviderInternal(absPath, provider) // Reuse logic
	if err != nil {
		http.Error(w, fmt.Sprintf("Error setting provider: %v", err), http.StatusInternalServerError)
		return
	}

	// Fetch the updated repo state
	state.State.Mu.RLock()
	repo, exists := state.State.Repositories[absPath]
	state.State.Mu.RUnlock()

	if !exists {
		http.Error(w, "Repository not found after setting provider", http.StatusInternalServerError)
		return
	}

	// Return the updated card
	renderTemplate(w, "repositoryCard", repo)
}

// HandleUpdateRepositoryHTMX updates repository settings (e.g., schedule) and returns the updated card.
// Note: The current 'Update' button in the original JS calls /api/repositories/update which doesn't seem to exist.
// Assuming it's meant to update settings like schedule. Needs clarification.
// For now, let's assume it re-reads the schedule from a potentially updated config or state.
func HandleUpdateRepositoryHTMX(w http.ResponseWriter, r *http.Request) {
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

	// TODO: Implement actual update logic if needed.
	// This might involve reading form data if settings were editable inline.
	// For now, just fetch current state.

	state.State.Mu.RLock()
	repo, exists := state.State.Repositories[absPath]
	state.State.Mu.RUnlock()

	if !exists {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	// Return the (potentially unchanged) card
	renderTemplate(w, "repositoryCard", repo)
}

// HandleShowIssuesHTMX fetches issues and returns an HTML fragment for the modal.
func HandleShowIssuesHTMX(w http.ResponseWriter, r *http.Request) {
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

	state.State.Mu.RLock()
	repo, exists := state.State.Repositories[absPath]
	token := state.State.Settings.GitHubToken // Needed if fetching from GitHub
	state.State.Mu.RUnlock()

	if !exists {
		http.Error(w, "Repository not found", http.StatusNotFound)
		return
	}

	// Check provider and token (if GitHub)
	if repo.RemoteProvider == nil || repo.RemoteProvider.Provider == "" {
		renderTemplate(w, "issuesListFragment", map[string]interface{}{"Error": "Provider not set for repository"})
		return
	}
	if repo.RemoteProvider.Provider == "github" && token == "" {
		renderTemplate(w, "issuesListFragment", map[string]interface{}{"Error": "GitHub token not configured in Settings"})
		return
	}

	// Fetch issues (similar to HandleGitHubIssues)
	err = repo.UpdateIssues() // Update cache/state first
	if err != nil {
		renderTemplate(w, "issuesListFragment", map[string]interface{}{"Error": fmt.Sprintf("Error fetching issues: %v", err)})
		return
	}

	// Fetch the potentially updated issues list from the repo object
	repo.Mu.RLock()
	issues := make([]*repository.Issue, 0, len(repo.Issues))
	for _, issue := range repo.Issues {
		issues = append(issues, issue)
	}
	repo.Mu.RUnlock()

	// Sort issues (e.g., by creation date descending)
	sort.Slice(issues, func(i, j int) bool {
		// Handle potential parsing errors if CreatedAt format varies
		t1, _ := time.Parse(time.RFC3339, issues[i].CreatedAt)
		t2, _ := time.Parse(time.RFC3339, issues[j].CreatedAt)
		return t1.After(t2)
	})

	data := map[string]interface{}{
		"Issues": issues,
		"Error":  nil, // Explicitly set Error to nil if successful
	}
	renderTemplate(w, "issuesListFragment", data)
}

// Helper function to decode URL-encoded path (used if needed, standard library handles query params)
func decodePath(path string) (string, error) {
	decodedPath, err := url.QueryUnescape(path)
	if err != nil {
		return "", fmt.Errorf("failed to decode path: %w", err)
	}
	return decodedPath, nil
}

// ================= Settings Page HTMX Handlers =================

// HandleSettingsTab renders the content for a specific settings tab.
func (h *Handlers) HandleSettingsTab(w http.ResponseWriter, r *http.Request) {
	tab := r.PathValue("tab")
	if tab == "" {
		tab = "general" // Default tab
	}

	h.State.Mu.RLock()
	settingsCopy := *h.State.Settings // Work with a copy
	h.State.Mu.RUnlock()

	// Render only the tab content partial template
	h.RenderTemplatePartial(w, "settings.html", "settings-tab-content", PageData{
		Settings:   &settingsCopy, // Pass the copy
		CurrentTab: tab,
		Page:       "settings", // Needed if partial uses layout elements indirectly
	})
}

// HandleAddProvider adds a new empty provider section and returns the HTML fragment.
func (h *Handlers) HandleAddProvider(w http.ResponseWriter, r *http.Request) {
	// No need to lock/save here, we just render a template for a *new* item
	// The actual addition happens when the main form is saved.
	// However, we need the current count to determine the *next* index.
	h.State.Mu.RLock()
	currentIndexCount := len(h.State.Settings.AIProviders)
	h.State.Mu.RUnlock()

	// Create dummy data for the template
	data := map[string]interface{}{
		"Index":    currentIndexCount,   // The index this new provider *will* have
		"Provider": config.AIProvider{}, // Empty provider data
		"Settings": h.State.Settings,    // Pass full settings if needed by template funcs
	}

	// Render just the new provider item template fragment
	h.RenderTemplatePartial(w, "settings.html", "provider-item", data)
}

// HandleRemoveProvider removes a provider based on its index during form interaction.
// IMPORTANT: This only removes the item from the *client-side* form via HTMX swap.
// The actual removal from config happens on full form submission (/api/settings).
// For a pure HTMX approach without full form resubmit, this handler *would* need
// to modify and save the config, then potentially re-render the list.
// Current approach: HTMX DELETE + swap removes row, final Save button persists.
func (h *Handlers) HandleRemoveProvider(w http.ResponseWriter, r *http.Request) {
	indexStr := r.PathValue("index")
	_, err := strconv.Atoi(indexStr)
	if err != nil {
		h.Logger.Error("invalid provider index for removal", "index", indexStr, "error", err)
		http.Error(w, "Invalid provider index", http.StatusBadRequest)
		return
	}

	// We don't modify server state here based on the current design.
	// Just return 200 OK. HTMX hx-delete + hx-swap="outerHTML" will remove the element.
	w.WriteHeader(http.StatusOK)
}
