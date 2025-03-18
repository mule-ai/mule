package handlers

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/mule-ai/mule/internal/settings"
	"github.com/mule-ai/mule/internal/state"
	"github.com/mule-ai/mule/pkg/remote/types"
)

type LocalPageData struct {
	Repository   interface{}
	Issues       []types.Issue
	PullRequests []types.PullRequest
	Page         string
	Settings     settings.Settings
}

func HandleLocalProviderPage(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "Path parameter is required", http.StatusBadRequest)
		return
	}

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

	issues, err := repo.Remote.FetchIssues(absPath, types.IssueFilterOptions{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pullRequests, err := repo.Remote.FetchPullRequests(absPath, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := LocalPageData{
		Repository:   repo,
		Page:         "local",
		Settings:     state.State.Settings,
		Issues:       issues,
		PullRequests: pullRequests,
	}

	err = templates.ExecuteTemplate(w, "layout.html", data)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func HandleCreateLocalIssue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path  string `json:"path"`
		Title string `json:"title"`
		Body  string `json:"body"`
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

	repo.Mu.Lock()
	defer repo.Mu.Unlock()

	issueNumber, err := repo.Remote.CreateIssue(types.Issue{
		Title:     req.Title,
		Body:      req.Body,
		State:     "open",
		CreatedAt: time.Now().String(),
		Comments:  make([]*types.Comment, 0),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	_, err = w.Write([]byte(strconv.Itoa(issueNumber)))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func HandleAddLocalComment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path         string `json:"path"`
		ResourceID   int    `json:"resourceId"`
		ResourceType string `json:"resourceType"`
		Body         string `json:"body"`
		DiffHunk     string `json:"diffHunk,omitempty"`
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

	repo.Mu.Lock()
	defer repo.Mu.Unlock()

	comment := &types.Comment{
		ID:        time.Now().Unix(),
		Body:      req.Body,
		DiffHunk:  req.DiffHunk,
		Reactions: types.Reactions{},
	}

	switch req.ResourceType {
	case "issue":
		err = repo.Remote.CreateIssueComment(req.Path, req.ResourceID, *comment)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	case "pr":
		err = repo.Remote.CreatePRComment(req.Path, req.ResourceID, *comment)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	w.WriteHeader(http.StatusCreated)
}

func HandleAddLocalReaction(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path      string `json:"path"`
		CommentID int64  `json:"commentId"`
		Reaction  string `json:"reaction"`
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

	repo.Mu.Lock()
	defer repo.Mu.Unlock()

	err = repo.Remote.AddCommentReaction(req.Path, req.Reaction, req.CommentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func HandleGetLocalDiff(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	prNumber := r.URL.Query().Get("pr")
	if path == "" || prNumber == "" {
		http.Error(w, "Path and PR number are required", http.StatusBadRequest)
		return
	}

	prNum, err := strconv.Atoi(prNumber)
	if err != nil {
		http.Error(w, "Invalid PR number", http.StatusBadRequest)
		return
	}

	repo, err := getRepository(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	diff, err := repo.Remote.FetchDiffs("", "", prNum)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	_, err = w.Write([]byte(diff))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func HandleAddLocalLabel(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path        string `json:"path"`
		IssueNumber int    `json:"issueNumber"`
		Label       string `json:"label"`
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

	repo.Mu.Lock()
	defer repo.Mu.Unlock()

	err = repo.Remote.AddLabelToIssue(req.IssueNumber, req.Label)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func HandleUpdateLocalIssueState(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path        string `json:"path"`
		IssueNumber int    `json:"issueNumber"`
		State       string `json:"state"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.State != "open" && req.State != "closed" {
		http.Error(w, "Invalid state. Must be 'open' or 'closed'", http.StatusBadRequest)
		return
	}

	repo, err := getRepository(req.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	repo.Mu.Lock()
	defer repo.Mu.Unlock()

	err = repo.Remote.UpdateIssueState(req.IssueNumber, req.State)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func HandleUpdateLocalPullRequestState(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path     string `json:"path"`
		PRNumber int    `json:"prNumber"`
		State    string `json:"state"`
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

	repo.Mu.Lock()
	defer repo.Mu.Unlock()

	err = repo.Remote.UpdatePullRequestState(req.Path, req.PRNumber, req.State)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func HandleDeleteLocalIssue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path        string `json:"path"`
		IssueNumber int    `json:"issueNumber"`
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

	repo.Mu.Lock()
	defer repo.Mu.Unlock()

	err = repo.Remote.DeleteIssue(req.Path, req.IssueNumber)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func HandleDeleteLocalPullRequest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path     string `json:"path"`
		PRNumber int    `json:"prNumber"`
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

	repo.Mu.Lock()
	defer repo.Mu.Unlock()

	err = repo.Remote.DeletePullRequest(req.Path, req.PRNumber)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func HandleUpdateLocalIssue(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path        string `json:"path"`
		IssueNumber int    `json:"issueNumber"`
		Title       string `json:"title"`
		Body        string `json:"body"`
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

	repo.Mu.Lock()
	defer repo.Mu.Unlock()

	err = repo.Remote.UpdateIssue(req.IssueNumber, req.Title, req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
