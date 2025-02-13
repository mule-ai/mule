package github

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/google/go-github/v60/github"
	"github.com/jbutlerdev/dev-team/pkg/remote/types"
	"golang.org/x/oauth2"
)

type GitHubPRResponse struct {
	Number int `json:"number"`
}

var re = regexp.MustCompile(`<!--(.*?)-->`)

func newGitHubClient(ctx context.Context, token string) *github.Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc)
}

func (p *Provider) CreateDraftPR(path string, input types.PullRequestInput) error {
	repo, err := git.PlainOpen(path)
	if err != nil {
		return err
	}

	remote, err := repo.Remote("origin")
	if err != nil {
		return fmt.Errorf("error getting remote: %v", err)
	}

	remoteURL := remote.Config().URLs[0]
	var owner, repoName string
	if strings.Contains(remoteURL, "git@github.com:") {
		parts := strings.Split(strings.TrimPrefix(remoteURL, "git@github.com:"), "/")
		owner = parts[0]
		repoName = strings.TrimSuffix(parts[1], ".git")
	} else {
		parts := strings.Split(strings.TrimPrefix(remoteURL, "https://github.com/"), "/")
		owner = parts[0]
		repoName = strings.TrimSuffix(parts[1], ".git")
	}

	newPR := &github.NewPullRequest{
		Title:               github.String(input.Title),
		Head:                github.String(input.Branch),
		Base:                github.String(input.Base),
		Body:                github.String(input.Description),
		Draft:               github.Bool(input.Draft),
		MaintainerCanModify: github.Bool(input.MaintainerCanModify),
	}

	pr, _, err := p.Client.PullRequests.Create(p.ctx, owner, repoName, newPR)
	if err != nil {
		return fmt.Errorf("error creating PR: %v", err)
	}

	prLink := pr.GetHTMLURL()
	log.Printf("PR created successfully: %s", prLink)

	return nil
}

func (p *Provider) FetchRepositories() ([]types.Repository, error) {
	opt := &github.RepositoryListByAuthenticatedUserOptions{
		Sort: "updated",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	repos, _, err := p.Client.Repositories.ListByAuthenticatedUser(p.ctx, opt)
	if err != nil {
		return nil, fmt.Errorf("error fetching repositories: %v", err)
	}

	var result []types.Repository
	for _, repo := range repos {
		result = append(result, types.Repository{
			Name:        repo.GetName(),
			FullName:    repo.GetFullName(),
			Description: repo.GetDescription(),
			CloneURL:    repo.GetCloneURL(),
			SSHURL:      repo.GetSSHURL(),
		})
	}

	return result, nil
}

func (p *Provider) FetchPullRequests(remotePath, label string) ([]types.PullRequest, error) {
	parts := strings.Split(remotePath, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid remote path format")
	}
	owner := parts[0]
	repo := parts[1]

	opt := &github.PullRequestListOptions{
		State: "open",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	ghPullRequests, _, err := p.Client.PullRequests.List(p.ctx, owner, repo, opt)
	if err != nil {
		log.Printf("Error fetching pull requests: %v, request: %v", err, remotePath)
		return nil, fmt.Errorf("error fetching pull requests: %v", err)
	}

	var pullRequests []types.PullRequest
	for _, pullRequest := range ghPullRequests {
		pr := types.PullRequest{
			Number:          pullRequest.GetNumber(),
			Title:           pullRequest.GetTitle(),
			Body:            pullRequest.GetBody(),
			State:           pullRequest.GetState(),
			Labels:          make([]string, 0, len(pullRequest.Labels)),
			HTMLURL:         pullRequest.GetHTMLURL(),
			IssueURL:        pullRequest.GetIssueURL(),
			CreatedAt:       pullRequest.GetCreatedAt().String(),
			UpdatedAt:       pullRequest.GetUpdatedAt().String(),
			LinkedIssueURLs: getLinkedIssueURLs(pullRequest.GetBody()),
			Comments:        make([]*types.Comment, 0),
		}
		for i, label := range pullRequest.Labels {
			pr.Labels[i] = label.GetName()
		}

		// Fetch comments for the pull request
		comments, err := p.FetchComments(owner, repo, pullRequest.GetNumber())
		if err != nil {
			log.Printf("Error fetching comments for PR %d: %v", pullRequest.GetNumber(), err)
			// Don't return, just log the error and continue
		}
		pr.Comments = comments

		diff, err := p.FetchDiffs(owner, repo, pullRequest.GetNumber())
		if err != nil {
			log.Printf("Error fetching diffs for PR %d: %v", pullRequest.GetNumber(), err)
			// Don't return, just log the error and continue
		}
		pr.Diff = diff

		pullRequests = append(pullRequests, pr)
	}

	return pullRequests, nil
}

func (p *Provider) UpdatePullRequestState(remotePath string, prNumber int, state string) error {
	return nil
}

func (p *Provider) FetchDiffs(owner, repo string, resourceID int) (string, error) {
	diff, _, err := p.Client.PullRequests.GetRaw(p.ctx, owner, repo, resourceID, github.RawOptions{Type: github.Diff})
	if err != nil {
		return "", fmt.Errorf("failed to get pull request diff: %w", err)
	}
	return diff, nil
}

func getLinkedIssueURLs(body string) []string {
	// URLs are in HTML comments
	matches := re.FindAllString(body, -1)
	urls := make([]string, len(matches))
	for i, match := range matches {
		match = strings.TrimPrefix(match, "<!--")
		match = strings.TrimSuffix(match, "-->")
		urls[i] = match
	}
	return urls
}
