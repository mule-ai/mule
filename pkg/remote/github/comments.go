package github

import (
	"fmt"
	"strings"

	"github.com/google/go-github/v60/github"
	"github.com/jbutlerdev/dev-team/pkg/remote/types"
)

func (p *Provider) FetchComments(owner, repo string, prNumber int) ([]*types.Comment, error) {
	opt := &github.PullRequestListCommentsOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	ghComments, _, err := p.Client.PullRequests.ListComments(p.ctx, owner, repo, prNumber, opt)
	if err != nil {
		return nil, fmt.Errorf("error fetching comments: %v", err)
	}

	var comments []*types.Comment
	for _, comment := range ghComments {
		c := &types.Comment{
			ID:       comment.GetID(),
			Body:     comment.GetBody(),
			DiffHunk: comment.GetDiffHunk(),
			HTMLURL:  comment.GetHTMLURL(),
			URL:      comment.GetURL(),
			UserID:   comment.GetUser().GetID(),
		}
		reactions, err := p.FetchPullRequestCommentReactions(owner, repo, comment.GetID())
		if err != nil {
			return nil, fmt.Errorf("error fetching reactions: %v", err)
		}
		c.Reactions = reactions
		comments = append(comments, c)
	}

	return comments, nil
}

func (p *Provider) FetchPullRequestCommentReactions(owner, repo string, commentID int64) (types.Reactions, error) {
	opt := &github.ListCommentReactionOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}
	// add pulls to repo for proper URL
	repo = repo + "/pulls"
	ghReactions, _, err := p.Client.Reactions.ListCommentReactions(p.ctx, owner, repo, commentID, opt)
	if err != nil {
		return types.Reactions{}, fmt.Errorf("error fetching reactions: %v", err)
	}

	reactions := types.Reactions{}
	for _, ghReaction := range ghReactions {
		reactions.TotalCount++
		switch ghReaction.GetContent() {
		case "+1":
			reactions.PlusOne++
		case "-1":
			reactions.MinusOne++
		case "laugh":
			reactions.Laugh++
		case "confused":
			reactions.Confused++
		case "heart":
			reactions.Heart++
		case "hooray":
			reactions.Hooray++
		case "rocket":
			reactions.Rocket++
		case "eyes":
			reactions.Eyes++
		}
	}
	return reactions, nil
}

func (p *Provider) AddCommentReaction(repoPath, reaction string, commentID int64) error {
	parts := strings.Split(repoPath, "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid repo path format")
	}
	owner := parts[0]
	repo := parts[1]

	_, _, err := p.Client.Reactions.CreateCommentReaction(p.ctx, owner, repo, commentID, reaction)
	if err != nil {
		return fmt.Errorf("error adding reaction: %v", err)
	}
	return nil
}
