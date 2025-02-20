package repository

import (
	"fmt"
	"log"

	"github.com/mule-ai/mule/pkg/remote/types"
)

type Issue struct {
	ID           int            `json:"id"`
	Number       int            `json:"number"`
	Title        string         `json:"title"`
	Body         string         `json:"body"`
	State        string         `json:"state"`
	HTMLURL      string         `json:"html_url"`
	SourceURL    string         `json:"source_url"`
	CreatedAt    string         `json:"created_at"`
	UpdatedAt    string         `json:"updated_at"`
	Comments     []*Comment     `json:"comments"`
	PullRequests []*PullRequest `json:"pull_requests"`
	Labels       []string       `json:"labels"`
}

func (i *Issue) addPullRequests(pullRequests map[int]*PullRequest) {
	i.PullRequests = make([]*PullRequest, 0)
	for _, pullRequest := range pullRequests {
		for _, linkedIssueUrl := range pullRequest.LinkedIssueUrls {
			if linkedIssueUrl == i.SourceURL || linkedIssueUrl == i.HTMLURL {
				i.PullRequests = append(i.PullRequests, pullRequest)
			}
		}
	}
}

func (i *Issue) Completed() bool {
	_, hasUnresolvedComments := i.PRHasUnresolvedComments()
	if hasUnresolvedComments {
		return false
	} else if i.PrExists() {
		return true
	}
	return false
}

func (i *Issue) PrExists() bool {
	return len(i.PullRequests) > 0
}

func (i *Issue) PRHasUnresolvedComments() (*PullRequest, bool) {
	for _, pullRequest := range i.PullRequests {
		if pullRequest.HasUnresolvedComments() {
			return pullRequest, true
		}
	}
	return nil, false
}

func (r *Repository) GetIssues() ([]*Issue, error) {
	issues := make([]*Issue, 0, len(r.Issues))
	for _, issue := range r.Issues {
		issues = append(issues, issue)
	}
	return issues, nil
}

func (r *Repository) UpdateIssues() error {
	if r.RemotePath == "" {
		return fmt.Errorf("repository remote path is not set")
	}
	issues, err := r.Remote.FetchIssues(r.RemotePath, types.IssueFilterOptions{
		State: "open",
		Label: "mule",
	})
	if err != nil {
		log.Printf("Error fetching issues: %v, request: %v", err, r.RemotePath)
		return err
	}
	// reset tracked issues
	r.Issues = make(map[int]*Issue)
	for _, issue := range issues {
		r.Issues[issue.Number] = ghIssueToIssue(issue)
	}
	return nil
}

func (i *Issue) ToString() string {
	return fmt.Sprintf("Issue #%d: %s\n%s", i.Number, i.Title, i.Body)
}

func ghIssueToIssue(issue types.Issue) *Issue {
	return &Issue{
		ID:        issue.Number,
		Number:    issue.Number,
		Title:     issue.Title,
		Body:      issue.Body,
		CreatedAt: issue.CreatedAt,
		UpdatedAt: issue.UpdatedAt,
		SourceURL: issue.SourceURL,
		HTMLURL:   issue.HTMLURL,
		State:     issue.State,
	}
}
