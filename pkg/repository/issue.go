package repository

import (
	"dev-team/pkg/github"
	"fmt"
	"log"
)

type Issue struct {
	ID           int            `json:"id"`
	Title        string         `json:"title"`
	Body         string         `json:"body"`
	Labels       []string       `json:"labels"`
	CreatedAt    string         `json:"created_at"`
	UpdatedAt    string         `json:"updated_at"`
	HTMLURL      string         `json:"html_url"`
	SourceURL    string         `json:"source_url"`
	State        string         `json:"state"`
	PullRequests []*PullRequest `json:"pull_requests"`
}

func (i *Issue) addPullRequests(pullRequests map[int]*PullRequest) {
	for _, pullRequest := range pullRequests {
		for _, linkedIssueUrl := range pullRequest.LinkedIssueUrls {
			if linkedIssueUrl == i.SourceURL || linkedIssueUrl == i.HTMLURL {
				i.PullRequests = append(i.PullRequests, pullRequest)
			}
		}
	}
}

func (i *Issue) prExists() bool {
	return len(i.PullRequests) > 0
}

func (r *Repository) GetIssues() ([]*Issue, error) {
	issues := make([]*Issue, 0, len(r.Issues))
	for _, issue := range r.Issues {
		issues = append(issues, issue)
	}
	return issues, nil
}

func (r *Repository) UpdateIssues(token string) error {
	if r.RemotePath == "" {
		return fmt.Errorf("repository remote path is not set")
	}
	issues, err := github.FetchIssues(r.RemotePath, "dev-team", token)
	if err != nil {
		log.Printf("Error fetching issues: %v, request: %v", err, r.RemotePath)
		return err
	}
	for _, issue := range issues {
		r.Issues[issue.Number] = ghIssueToIssue(issue)
	}
	return nil
}

func (i *Issue) ToString() string {
	return fmt.Sprintf("Issue: %d\n\n"+
		"Title: %s\n\n"+
		"Body: %s\n\n", i.ID, i.Title, i.Body)
}

func ghIssueToIssue(issue github.Issue) *Issue {
	return &Issue{
		ID:        issue.Number,
		Title:     issue.Title,
		Body:      issue.Body,
		CreatedAt: issue.CreatedAt,
		UpdatedAt: issue.UpdatedAt,
		SourceURL: issue.SourceURL,
		HTMLURL:   issue.HTMLURL,
		State:     issue.State,
	}
}
