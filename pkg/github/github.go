package github

import (
	"context"
	"fmt"
	"log"
	"regexp"
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
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	State     string `json:"state"`
	HTMLURL   string `json:"html_url"`
	SourceURL string `json:"source_url"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type PullRequest struct {
	Number          int       `json:"number"`
	Title           string    `json:"title"`
	Body            string    `json:"body"`
	State           string    `json:"state"`
	HTMLURL         string    `json:"html_url"`
	Labels          []string  `json:"labels"`
	IssueURL        string    `json:"issue_url"`
	CreatedAt       string    `json:"created_at"`
	UpdatedAt       string    `json:"updated_at"`
	LinkedIssueURLs []string  `json:"linked_issue_urls"`
	Diff            string    `json:"diff"`
	Comments        []Comment `json:"comments"`
}

var re = regexp.MustCompile(`<!--(.*?)-->`)

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

	opt := &github.RepositoryListByAuthenticatedUserOptions{
		Sort: "updated",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	repos, _, err := client.Repositories.ListByAuthenticatedUser(ctx, opt)
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
		State:  "open",
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
			SourceURL: issue.GetHTMLURL(),
			CreatedAt: issue.GetCreatedAt().String(),
			UpdatedAt: issue.GetUpdatedAt().String(),
		}
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
			Comments:        []Comment{},
		}
		for i, label := range pullRequest.Labels {
			pr.Labels[i] = label.GetName()
		}

		// Fetch comments for the pull request
		comments, err := FetchComments(ctx, client, owner, repo, pullRequest.GetNumber())
		if err != nil {
			log.Printf("Error fetching comments for PR %d: %v", pullRequest.GetNumber(), err)
			// Don't return, just log the error and continue
		}
		pr.Comments = comments

		diff, err := FetchDiffs(ctx, client, owner, repo, pullRequest.GetNumber())
		if err != nil {
			log.Printf("Error fetching diffs for PR %d: %v", pullRequest.GetNumber(), err)
			// Don't return, just log the error and continue
		}
		pr.Diff = diff

		pullRequests = append(pullRequests, pr)
	}

	return pullRequests, nil
}

func FetchDiffs(ctx context.Context, client *github.Client, owner, repo string, resourceID int) (string, error) {
	diff, _, err := client.PullRequests.GetRaw(ctx, owner, repo, resourceID, github.RawOptions{Type: github.Diff})
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
