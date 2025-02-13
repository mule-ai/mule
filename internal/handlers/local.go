package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"time"

	"github.com/jbutlerdev/dev-team/internal/settings"
	"github.com/jbutlerdev/dev-team/internal/state"
	"github.com/jbutlerdev/dev-team/pkg/remote/types"
	"github.com/jbutlerdev/dev-team/pkg/repository"
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

	comment := &repository.Comment{
		ID:        time.Now().UnixNano(),
		Body:      req.Body,
		Reactions: types.Reactions{},
	}

	switch req.ResourceType {
	case "issue":
		issue, exists := repo.Issues[req.ResourceID]
		if !exists {
			http.Error(w, "Issue not found", http.StatusNotFound)
			return
		}
		issue.Comments = append(issue.Comments, comment)
	case "pr":
		pr, exists := repo.PullRequests[req.ResourceID]
		if !exists {
			http.Error(w, "Pull request not found", http.StatusNotFound)
			return
		}
		pr.Comments = append(pr.Comments, comment)
	default:
		http.Error(w, "Invalid resource type", http.StatusBadRequest)
		return
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

	log.Printf("req: %+v", req)

	// Find the comment in issues or PRs
	found := false
	for _, issue := range repo.Issues {
		for i := range issue.Comments {
			if issue.Comments[i].ID == req.CommentID {
				switch req.Reaction {
				case "+1":
					issue.Comments[i].Reactions.PlusOne++
				case "-1":
					issue.Comments[i].Reactions.MinusOne++
				case "laugh":
					issue.Comments[i].Reactions.Laugh++
				case "confused":
					issue.Comments[i].Reactions.Confused++
				case "heart":
					issue.Comments[i].Reactions.Heart++
				case "hooray":
					issue.Comments[i].Reactions.Hooray++
				case "rocket":
					issue.Comments[i].Reactions.Rocket++
				case "eyes":
					issue.Comments[i].Reactions.Eyes++
				}
				found = true
				break
			}
		}
		if found {
			break
		}
	}

	if !found {
		for _, pr := range repo.PullRequests {
			for i := range pr.Comments {
				if pr.Comments[i].ID == req.CommentID {
					switch req.Reaction {
					case "+1":
						pr.Comments[i].Reactions.PlusOne++
					case "-1":
						pr.Comments[i].Reactions.MinusOne++
					case "laugh":
						pr.Comments[i].Reactions.Laugh++
					case "confused":
						pr.Comments[i].Reactions.Confused++
					case "heart":
						pr.Comments[i].Reactions.Heart++
					case "hooray":
						pr.Comments[i].Reactions.Hooray++
					case "rocket":
						pr.Comments[i].Reactions.Rocket++
					case "eyes":
						pr.Comments[i].Reactions.Eyes++
					}
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}

	if !found {
		http.Error(w, "Comment not found", http.StatusNotFound)
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
