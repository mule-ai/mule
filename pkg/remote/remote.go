package remote

import (
	"fmt"

	"github.com/jbutlerdev/dev-team/pkg/remote/github"
	"github.com/jbutlerdev/dev-team/pkg/remote/local"
	"github.com/jbutlerdev/dev-team/pkg/remote/types"
)

const (
	LOCAL  = 0
	GITHUB = 1
)

var stringToIntMap = map[string]int{
	"local":  LOCAL,
	"github": GITHUB,
}

type Provider interface {
	CreateDraftPR(path string, input types.PullRequestInput) error
	CreateIssue(issue types.Issue) (int, error)
	CreateIssueComment(path string, issueNumber int, comment types.Comment) error
	CreatePRComment(path string, prNumber int, comment types.Comment) error
	DeleteIssue(repoPath string, issueNumber int) error
	DeletePullRequest(repoPath string, prNumber int) error
	UpdateIssueState(issueNumber int, state string) error
	AddLabelToIssue(issueNumber int, label string) error
	FetchRepositories() ([]types.Repository, error)
	FetchIssues(remotePath string, options types.IssueFilterOptions) ([]types.Issue, error)
	FetchPullRequests(remotePath, label string) ([]types.PullRequest, error)
	UpdatePullRequestState(remotePath string, prNumber int, state string) error
	FetchDiffs(owner, repo string, resourceID int) (string, error)
	FetchComments(owner, repo string, prNumber int) ([]*types.Comment, error)
	AddCommentReaction(repoPath, reaction string, commentID int64) error
}

type ProviderSettings struct {
	Provider string `json:"provider,omitempty"`
	Path     string `json:"path,omitempty"`
	Token    string `json:"token,omitempty"`
}

type ProviderOptions struct {
	Type        int
	GitHubToken string
	Path        string
}

func New(options ProviderOptions) Provider {
	switch options.Type {
	case LOCAL:
		return local.NewProvider(options.Path)
	case GITHUB:
		return github.NewProvider(options.Path, options.GitHubToken)
	}
	return nil
}

func SettingsToOptions(settings ProviderSettings) (ProviderOptions, error) {
	provider, ok := stringToIntMap[settings.Provider]
	if !ok {
		return ProviderOptions{}, fmt.Errorf("invalid provider: %s", settings.Provider)
	}
	return ProviderOptions{
		Type:        provider,
		GitHubToken: settings.Token,
		Path:        settings.Path,
	}, nil
}
