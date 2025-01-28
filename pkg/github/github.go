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
	Title               string
	Description         string
	Branch              string
	Base                string
	Draft               bool
	MaintainerCanModify bool
}

type GitHubPRResponse struct {
	Number int
}

type Repository struct {
	Name        string
	FullName    string
	Description string
	CloneURL    string
	SSHURL      string
}

type Issue struct {
	Number    int
	Title     string
	Body      string
	State     string
	HTMLURL   string
	CreatedAt string
	UpdatedAt string
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

func FetchIssues(baseURL, label, githubToken string) ([]Issue, error) {
	if githubToken == "" {
		return nil, fmt.Errorf("GitHub token not provided in settings")
	}

	ctx := context.Background()
	client := newGitHubClient(ctx, githubToken)

	// Extract owner and repo from baseURL
	// Expected format: https://api.github.com/repos/owner/repo
	parts := strings.Split(strings.TrimPrefix(baseURL, "https://api.github.com/repos/"), "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid baseURL format")
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
		log.Printf("Error fetching issues: %v, request: %v", err, baseURL)
		return nil, fmt.Errorf("error fetching issues: %v", err)
	}

	var issues []Issue
	for _, issue := range ghIssues {
		issues = append(issues, Issue{
			Number:    issue.GetNumber(),
			Title:     issue.GetTitle(),
			Body:      issue.GetBody(),
			State:     issue.GetState(),
			HTMLURL:   issue.GetHTMLURL(),
			CreatedAt: issue.GetCreatedAt().String(),
			UpdatedAt: issue.GetUpdatedAt().String(),
		})
	}

	return issues, nil
}
