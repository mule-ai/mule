package github

import (
	"fmt"
	"log"
	"strings"

	"github.com/google/go-github/v60/github"
	"github.com/jbutlerdev/dev-team/pkg/remote/types"
)

func (p *Provider) CreateIssue(issue types.Issue) (int, error) {
	return 0, nil
}

func (p *Provider) FetchIssues(remotePath string, options types.IssueFilterOptions) ([]types.Issue, error) {
	parts := strings.Split(remotePath, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid remote path format")
	}
	owner := parts[0]
	repo := parts[1]

	labelsFilter := []string{}
	if options.Label != "" {
		labelsFilter = append(labelsFilter, options.Label)
	}

	stateFilter := "open"
	if options.State != "" {
		stateFilter = options.State
	}

	opt := &github.IssueListByRepoOptions{
		Labels: labelsFilter,
		State:  stateFilter,
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	ghIssues, _, err := p.Client.Issues.ListByRepo(p.ctx, owner, repo, opt)
	if err != nil {
		log.Printf("Error fetching issues: %v, request: %v", err, remotePath)
		return nil, fmt.Errorf("error fetching issues: %v", err)
	}

	var issues []types.Issue
	for _, issue := range ghIssues {
		i := types.Issue{
			Number:    issue.GetNumber(),
			Title:     issue.GetTitle(),
			Body:      issue.GetBody(),
			State:     issue.GetState(),
			HTMLURL:   issue.GetHTMLURL(),
			SourceURL: issue.GetHTMLURL(),
			Labels:    []string{},
			CreatedAt: issue.GetCreatedAt().String(),
			UpdatedAt: issue.GetUpdatedAt().String(),
		}
		for _, label := range issue.Labels {
			i.Labels = append(i.Labels, label.GetName())
		}
		issues = append(issues, i)
	}

	return issues, nil
}

func (p *Provider) AddLabelToIssue(issueNumber int, label string) error {
	return nil
}

func (p *Provider) UpdateIssueState(issueNumber int, state string) error {
	return nil
}
