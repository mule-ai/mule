package github

import (
	"context"
	"strings"

	"github.com/google/go-github/v60/github"
)

type Provider struct {
	Client *github.Client
	ctx    context.Context
	owner  string
	repo   string
}

func NewProvider(path, token string) *Provider {
	parts := strings.Split(path, "/")
	var owner, repo string
	if len(parts) < 2 {
		owner = ""
		repo = ""
	} else {
		owner = parts[0]
		repo = parts[1]
	}
	return &Provider{
		Client: newGitHubClient(context.Background(), token),
		ctx:    context.Background(),
		owner:  owner,
		repo:   repo,
	}
}
