package repository

import (
	"fmt"
	"log"

	"github.com/jbutlerdev/dev-team/pkg/remote/types"
)

type PullRequest struct {
	Number          int        `json:"number"`
	Title           string     `json:"title"`
	Body            string     `json:"body"`
	CreatedAt       string     `json:"created_at"`
	UpdatedAt       string     `json:"updated_at"`
	Labels          []string   `json:"labels"`
	IssueUrl        string     `json:"issue_url"`
	LinkedIssueUrls []string   `json:"linked_issue_urls"`
	Diff            string     `json:"diff"`
	Comments        []*Comment `json:"comments"`
}

type Comment struct {
	ID           int64           `json:"id"`
	Body         string          `json:"body"`
	DiffHunk     string          `json:"diff_hunk,omitempty"`
	HTMLURL      string          `json:"html_url"`
	URL          string          `json:"url"`
	UserID       int64           `json:"user_id"`
	Acknowledged bool            `json:"acknowledged"`
	Reactions    types.Reactions `json:"reactions"`
}

func (p *PullRequest) HasUnresolvedComments() bool {
	for _, comment := range p.Comments {
		if !comment.Acknowledged {
			return true
		}
	}
	return false
}

func (p *PullRequest) FirstUnresolvedComment() *Comment {
	for _, comment := range p.Comments {
		if !comment.Acknowledged {
			return comment
		}
	}
	return nil
}

func (r *Repository) GetPullRequests() []*PullRequest {
	pullRequests := make([]*PullRequest, 0, len(r.PullRequests))
	for _, pullRequest := range r.PullRequests {
		pullRequests = append(pullRequests, pullRequest)
	}
	return pullRequests
}

func (r *Repository) UpdatePullRequests() error {
	if r.RemotePath == "" {
		return fmt.Errorf("repository remote path is not set")
	}
	pullRequests, err := r.Remote.FetchPullRequests(r.RemotePath, "dev-team")
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

func ghPullRequestToPullRequest(pullRequest types.PullRequest) *PullRequest {
	return &PullRequest{
		Number:          pullRequest.Number,
		Title:           pullRequest.Title,
		Body:            pullRequest.Body,
		CreatedAt:       pullRequest.CreatedAt,
		UpdatedAt:       pullRequest.UpdatedAt,
		Labels:          pullRequest.Labels,
		IssueUrl:        pullRequest.IssueURL,
		LinkedIssueUrls: pullRequest.LinkedIssueURLs,
		Diff:            pullRequest.Diff,
		Comments:        ghCommentsToComments(pullRequest.Comments),
	}
}

func ghCommentsToComments(comments []*types.Comment) []*Comment {
	pullRequestComments := make([]*Comment, 0, len(comments))
	for _, comment := range comments {
		pullRequestComments = append(pullRequestComments, &Comment{
			ID:           comment.ID,
			Body:         comment.Body,
			DiffHunk:     comment.DiffHunk,
			HTMLURL:      comment.HTMLURL,
			URL:          comment.URL,
			UserID:       comment.UserID,
			Acknowledged: comment.Reactions.PlusOne > 0,
		})
	}
	return pullRequestComments
}
