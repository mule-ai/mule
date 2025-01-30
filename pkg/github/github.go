package github

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/google/go-github/v60/github"
	"golang.org/x/oauth2"
)

type GitHubPRInput struct {
	Title               string `json:"title"`
	Description         string `json:"description"`
	Branch              string `json:"branch"`
	Base                string `json:"base"`
	Draft               bool   `json:"draft"`
	MaintainerCanModify bool   `json:"maintainer_can_modify"`
}

type GitHubPRResponse struct {
	Number int `json:"number"`
}

type Repository struct {
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	CloneURL    string `json:"clone_url"`
	SSHURL      string `json:"ssh_url"`
}

type Issue struct {
	Number                int      `json:"number"`
	Title                 string   `json:"title"`
	Body                  string   `json:"body"`
	State                 string   `json:"state"`
	HTMLURL               string   `json:"html_url"`
	CreatedAt             string   `json:"created_at"`
	UpdatedAt             string   `json:"updated_at"`
	LinkedPullRequestURLs []string `json:"linked_pull_request_urls"`
}

type PullRequest struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	State     string   `json:"state"`
	HTMLURL   string   `json:"html_url"`
	Labels    []string `json:"labels"`
	IssueURL  string   `json:"issue_url"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

type IssueEvent struct {
	Event     string `json:"event"`
	CreatedAt string `json:"created_at"`
	PRNumber  int    `json:"pr_number,omitempty"`
	PRURL     string `json:"pr_url,omitempty"`
}

func newGitHubClient(ctx context.Context, token string) *github.Client {
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	return github.NewClient(tc)
}

func CreateDraftPR(path string, githubToken string, input GitHubPRInput) error {
	ctx := context.Background()
	client := newGitHubClient(ctx, githubToken)

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

	if githubToken == "" {
		return fmt.Errorf("GitHub token not provided in settings")
	}

	newPR := &github.NewPullRequest{
		Title:               github.String(input.Title),
		Head:                github.String(input.Branch),
		Base:                github.String(input.Base),
		Body:                github.String(input.Description),
		Draft:               github.Bool(input.Draft),
		MaintainerCanModify: github.Bool(input.MaintainerCanModify),
	}

	pr, _, err := client.PullRequests.Create(ctx, owner, repoName, newPR)
	if err != nil {
		return fmt.Errorf("error creating PR: %v", err)
	}

	prLink := pr.GetHTMLURL()
	log.Printf("PR created successfully: %s", prLink)

	return nil
}

func FetchRepositories(githubToken string) ([]Repository, error) {
	if githubToken == "" {
		return nil, fmt.Errorf("GitHub token not provided in settings")
	}

	ctx := context.Background()
	client := newGitHubClient(ctx, githubToken)

	opt := &github.RepositoryListOptions{
		Sort: "updated",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	repos, _, err := client.Repositories.List(ctx, "", opt)
	if err != nil {
		return nil, fmt.Errorf("error fetching repositories: %v", err)
	}

	var result []Repository
	for _, repo := range repos {
		result = append(result, Repository{
			Name:        repo.GetName(),
			FullName:    repo.GetFullName(),
			Description: repo.GetDescription(),
			CloneURL:    repo.GetCloneURL(),
			SSHURL:      repo.GetSSHURL(),
		})
	}

	return result, nil
}

func FetchIssues(remotePath, label, githubToken string) ([]Issue, error) {
	if githubToken == "" {
		return nil, fmt.Errorf("GitHub token not provided in settings")
	}

	ctx := context.Background()
	client := newGitHubClient(ctx, githubToken)

	// Extract owner and repo from remote path
	// Expected format: owner/repo
	parts := strings.Split(remotePath, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid remote path format")
	}
	owner := parts[0]
	repo := parts[1]

	opt := &github.IssueListByRepoOptions{
		Labels: []string{label},
		State:  "all",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	ghIssues, _, err := client.Issues.ListByRepo(ctx, owner, repo, opt)
	if err != nil {
		log.Printf("Error fetching issues: %v, request: %v", err, remotePath)
		return nil, fmt.Errorf("error fetching issues: %v", err)
	}

	var issues []Issue
	for _, issue := range ghIssues {
		i := Issue{
			Number:    issue.GetNumber(),
			Title:     issue.GetTitle(),
			Body:      issue.GetBody(),
			State:     issue.GetState(),
			HTMLURL:   issue.GetHTMLURL(),
			CreatedAt: issue.GetCreatedAt().String(),
			UpdatedAt: issue.GetUpdatedAt().String(),
		}
		urls, err := GetLinkedPullRequestURLs(remotePath, i.Number, githubToken)
		if err != nil {
			log.Printf("Error fetching linked pull request URLs: %v", err)
		}
		i.LinkedPullRequestURLs = urls
		issues = append(issues, i)
	}

	return issues, nil
}

func FetchPullRequests(remotePath, label, githubToken string) ([]PullRequest, error) {
	if githubToken == "" {
		return nil, fmt.Errorf("GitHub token not provided in settings")
	}

	ctx := context.Background()
	client := newGitHubClient(ctx, githubToken)

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

	ghPullRequests, _, err := client.PullRequests.List(ctx, owner, repo, opt)
	if err != nil {
		log.Printf("Error fetching pull requests: %v, request: %v", err, remotePath)
		return nil, fmt.Errorf("error fetching pull requests: %v", err)
	}

	var pullRequests []PullRequest
	for _, pullRequest := range ghPullRequests {
		pr := PullRequest{
			Number:    pullRequest.GetNumber(),
			Title:     pullRequest.GetTitle(),
			Body:      pullRequest.GetBody(),
			State:     pullRequest.GetState(),
			Labels:    make([]string, 0, len(pullRequest.Labels)),
			HTMLURL:   pullRequest.GetHTMLURL(),
			IssueURL:  pullRequest.GetIssueURL(),
			CreatedAt: pullRequest.GetCreatedAt().String(),
			UpdatedAt: pullRequest.GetUpdatedAt().String(),
		}
		for i, label := range pullRequest.Labels {
			pr.Labels[i] = label.GetName()
		}
		pullRequests = append(pullRequests, pr)
	}

	return pullRequests, nil
}

func FetchIssueEvents(remotePath string, issueNumber int, githubToken string) ([]IssueEvent, error) {
	if githubToken == "" {
		return nil, fmt.Errorf("GitHub token not provided in settings")
	}

	ctx := context.Background()
	client := newGitHubClient(ctx, githubToken)

	parts := strings.Split(remotePath, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid remote path format")
	}
	owner := parts[0]
	repo := parts[1]

	// First get the issue to check for PR links
	issue, _, err := client.Issues.Get(ctx, owner, repo, issueNumber)
	if err != nil {
		return nil, fmt.Errorf("error fetching issue: %v", err)
	}

	events, _, err := client.Issues.ListIssueEvents(ctx, owner, repo, issueNumber, &github.ListOptions{
		PerPage: 100,
	})
	if err != nil {
		return nil, fmt.Errorf("error fetching issue events: %v", err)
	}

	var issueEvents []IssueEvent

	// If the issue has PR links, add it as first event
	if issue.PullRequestLinks != nil {
		// Extract PR number from the URL
		url := issue.PullRequestLinks.GetURL()
		var prNumber int
		fmt.Sscanf(url, "https://api.github.com/repos/%s/%s/pulls/%d", &owner, &repo, &prNumber)

		issueEvents = append(issueEvents, IssueEvent{
			Event:     "pull_request_linked",
			CreatedAt: issue.GetCreatedAt().String(),
			PRNumber:  prNumber,
			PRURL:     issue.PullRequestLinks.GetHTMLURL(),
		})
	}

	// Add all other events
	for _, event := range events {
		e := IssueEvent{
			Event:     *event.Event,
			CreatedAt: event.GetCreatedAt().String(),
		}
		issueEvents = append(issueEvents, e)
	}

	return issueEvents, nil
}

func GetLinkedPullRequestURLs(remotePath string, issueNumber int, githubToken string) ([]string, error) {
	events, err := FetchIssueEvents(remotePath, issueNumber, githubToken)
	if err != nil {
		return nil, fmt.Errorf("error fetching issue events: %v", err)
	}
	log.Printf("Issue events: %+v", events)

	// Use a map to deduplicate URLs
	urlMap := make(map[string]struct{})
	for _, event := range events {
		if event.PRURL != "" {
			urlMap[event.PRURL] = struct{}{}
		}
	}

	// Convert map keys to slice
	urls := make([]string, 0, len(urlMap))
	for url := range urlMap {
		urls = append(urls, url)
	}

	return urls, nil
}
