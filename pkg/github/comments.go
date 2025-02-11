package github

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/go-github/v60/github"
)

type Comment struct {
	ID        int64     `json:"id"`
	Body      string    `json:"body"`
	DiffHunk  string    `json:"diff_hunk,omitempty"`
	HTMLURL   string    `json:"html_url"`
	URL       string    `json:"url"`
	UserID    int64     `json:"user_id"`
	Reactions Reactions `json:"reactions,omitempty"`
}

type Reaction struct {
	ID      int64  `json:"id,omitempty"`
	Content string `json:"content,omitempty"`
}

type Reactions struct {
	TotalCount int `json:"total_count,omitempty"`
	PlusOne    int `json:"+1,omitempty"`
	MinusOne   int `json:"-1,omitempty"`
	Laugh      int `json:"laugh,omitempty"`
	Confused   int `json:"confused,omitempty"`
	Heart      int `json:"heart,omitempty"`
	Hooray     int `json:"hooray,omitempty"`
	Rocket     int `json:"rocket,omitempty"`
	Eyes       int `json:"eyes,omitempty"`
}

func FetchComments(ctx context.Context, client *github.Client, owner, repo string, prNumber int) ([]Comment, error) {
	opt := &github.PullRequestListCommentsOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	ghComments, _, err := client.PullRequests.ListComments(ctx, owner, repo, prNumber, opt)
	if err != nil {
		return nil, fmt.Errorf("error fetching comments: %v", err)
	}

	var comments []Comment
	for _, comment := range ghComments {
		c := Comment{
			ID:       comment.GetID(),
			Body:     comment.GetBody(),
			DiffHunk: comment.GetDiffHunk(),
			HTMLURL:  comment.GetHTMLURL(),
			URL:      comment.GetURL(),
			UserID:   comment.GetUser().GetID(),
		}
		reactions, err := FetchPullRequestCommentReactions(ctx, client, owner, repo, comment.GetID())
		if err != nil {
			return nil, fmt.Errorf("error fetching reactions: %v", err)
		}
		c.Reactions = reactions
		comments = append(comments, c)
	}

	return comments, nil
}

func FetchPullRequestCommentReactions(ctx context.Context, client *github.Client, owner, repo string, commentID int64) (Reactions, error) {
	opt := &github.ListCommentReactionOptions{
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}
	// add pulls to repo for proper URL
	repo = repo + "/pulls"
	ghReactions, _, err := client.Reactions.ListCommentReactions(ctx, owner, repo, commentID, opt)
	if err != nil {
		return Reactions{}, fmt.Errorf("error fetching reactions: %v", err)
	}

	reactions := Reactions{}
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

func AddCommentReaction(repoPath, githubToken, reaction string, commentID int64) error {
	ctx := context.Background()
	client := newGitHubClient(ctx, githubToken)

	parts := strings.Split(repoPath, "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid repo path format")
	}
	owner := parts[0]
	repo := parts[1]

	_, _, err := client.Reactions.CreateCommentReaction(ctx, owner, repo, commentID, reaction)
	if err != nil {
		return fmt.Errorf("error adding reaction: %v", err)
	}
	return nil
}
