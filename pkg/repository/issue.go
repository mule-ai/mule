package repository

import (
	"dev-team/pkg/github"
	"fmt"
	"log"
)

type Issue struct {
	ID                    int      `json:"id"`
	Title                 string   `json:"title"`
	Body                  string   `json:"body"`
	Labels                []string `json:"labels"`
	CreatedAt             string   `json:"created_at"`
	UpdatedAt             string   `json:"updated_at"`
	SourceURL             string   `json:"source_url"`
	LinkedPullRequestURLs []string `json:"linked_pull_request_urls"`
	State                 string   `json:"state"`
}

func (i *Issue) prExists() bool {
	return len(i.LinkedPullRequestURLs) > 0
}

func (r *Repository) GetIssues() ([]Issue, error) {
	issues := make([]Issue, 0, len(r.Issues))
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

func ghIssueToIssue(issue github.Issue) Issue {
	return Issue{
		ID:                    issue.Number,
		Title:                 issue.Title,
		Body:                  issue.Body,
		CreatedAt:             issue.CreatedAt,
		UpdatedAt:             issue.UpdatedAt,
		SourceURL:             issue.HTMLURL,
		LinkedPullRequestURLs: issue.LinkedPullRequestURLs,
		State:                 issue.State,
	}
}
