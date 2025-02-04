package repository

import (
	"fmt"
	"log"

	"github.com/jbutlerdev/dev-team/pkg/github"
)

type PullRequest struct {
	Number          int      `json:"number"`
	Title           string   `json:"title"`
	Body            string   `json:"body"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
	Labels          []string `json:"labels"`
	IssueUrl        string   `json:"issue_url"`
	LinkedIssueUrls []string `json:"linked_issue_urls"`
}

type Comment struct {
	ID        string `json:"id"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

func (r *Repository) GetPullRequests() []*PullRequest {
	pullRequests := make([]*PullRequest, 0, len(r.PullRequests))
	for _, pullRequest := range r.PullRequests {
		pullRequests = append(pullRequests, pullRequest)
	}
	return pullRequests
}

func (r *Repository) UpdatePullRequests(token string) error {
	if r.RemotePath == "" {
		return fmt.Errorf("repository remote path is not set")
	}
	pullRequests, err := github.FetchPullRequests(r.RemotePath, "dev-team", token)
	if err != nil {
		log.Printf("Error fetching pull requests: %v, request: %v", err, r.RemotePath)
		return err
	}
	// reset tracked pull requests
	r.PullRequests = make(map[int]*PullRequest)
	for _, pullRequest := range pullRequests {
		r.PullRequests[pullRequest.Number] = ghPullRequestToPullRequest(pullRequest)
	}
	return nil
}

func ghPullRequestToPullRequest(pullRequest github.PullRequest) *PullRequest {
	return &PullRequest{
		Number:          pullRequest.Number,
		Title:           pullRequest.Title,
		Body:            pullRequest.Body,
		CreatedAt:       pullRequest.CreatedAt,
		UpdatedAt:       pullRequest.UpdatedAt,
		Labels:          pullRequest.Labels,
		IssueUrl:        pullRequest.IssueURL,
		LinkedIssueUrls: pullRequest.LinkedIssueURLs,
	}
}
