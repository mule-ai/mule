package repository

import (
	"dev-team/pkg/github"
	"fmt"
	"log"
)

type Issue struct {
	ID          int          `json:"id"`
	Title       string       `json:"title"`
	Body        string       `json:"body"`
	Labels      []string     `json:"labels"`
	CreatedAt   string       `json:"created_at"`
	UpdatedAt   string       `json:"updated_at"`
	PullRequest *PullRequest `json:"pull_request"`
}

func (r *Repository) GetIssues() ([]Issue, error) {
	return r.Issues, nil
}

func (r *Repository) UpdateIssues(token string) error {
	if r.ApiUrl == "" {
		return fmt.Errorf("API URL is not set")
	}
	issues, err := github.FetchIssues(r.ApiUrl, "dev-team", token)
	if err != nil {
		log.Printf("Error fetching issues: %v, request: %v", err, r.ApiUrl)
		return err
	}
	for _, issue := range issues {
		r.Issues = append(r.Issues, ghIssueToIssue(issue))
	}
	return nil
}

func ghIssueToIssue(issue github.Issue) Issue {
	return Issue{
		ID:        issue.Number,
		Title:     issue.Title,
		Body:      issue.Body,
		CreatedAt: issue.CreatedAt,
		UpdatedAt: issue.UpdatedAt,
	}
}
