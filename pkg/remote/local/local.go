package local

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/jbutlerdev/dev-team/pkg/remote/types"
)

const (
	dataPath    = ".config/dev-team/local-provider.json"
	filterLabel = "dev-team"
)

var re = regexp.MustCompile(`<!--(.*?)-->`)

type Provider struct {
	Path         string                     `json:"path"`
	Issues       map[int]*types.Issue       `json:"issues"`
	PullRequests map[int]*types.PullRequest `json:"pullRequests"`
	IssueCounter int                        `json:"issueCounter"`
}

func NewProvider(path string) *Provider {
	// load provider from file
	provider, err := loadProvider(path)
	if err != nil {
		log.Printf("error loading provider: %v", err)
	}
	return provider
}

func (p *Provider) CreateDraftPR(path string, input types.PullRequestInput) error {
	p.IssueCounter++
	linkedIssueURLs := getLinkedIssueURLs(input.Description)
	p.PullRequests[p.IssueCounter] = &types.PullRequest{
		Number:          p.IssueCounter,
		Title:           input.Title,
		Body:            input.Description,
		State:           "draft",
		BaseBranch:      input.Base,
		Branch:          input.Branch,
		LinkedIssueURLs: linkedIssueURLs,
	}

	return p.Save()
}

func (p *Provider) CreateIssue(issue types.Issue) (int, error) {
	p.IssueCounter++
	issue.Number = p.IssueCounter
	issue.SourceURL = fmt.Sprintf("%s/issues/%d", p.Path, p.IssueCounter)
	p.Issues[p.IssueCounter] = &issue

	return p.IssueCounter, p.Save()
}

func (p *Provider) DeleteIssue(repoPath string, issueNumber int) error {
	_, ok := p.Issues[issueNumber]
	if !ok {
		return fmt.Errorf("issue %d not found", issueNumber)
	}
	delete(p.Issues, issueNumber)
	return p.Save()
}

func (p *Provider) DeletePullRequest(repoPath string, prNumber int) error {
	_, ok := p.PullRequests[prNumber]
	if !ok {
		return fmt.Errorf("pull request %d not found", prNumber)
	}
	delete(p.PullRequests, prNumber)
	return p.Save()
}

func (p *Provider) UpdateIssueState(issueNumber int, state string) error {
	issue, ok := p.Issues[issueNumber]
	if !ok {
		return fmt.Errorf("issue %d not found", issueNumber)
	}
	issue.State = state
	return p.Save()
}

func (p *Provider) FetchIssues(remotePath string, options types.IssueFilterOptions) ([]types.Issue, error) {
	issues := make([]types.Issue, 0, len(p.Issues))
	for _, issue := range p.Issues {
		if options.Label != "" && !slices.Contains(issue.Labels, options.Label) {
			continue
		}
		if options.State != "" && issue.State != options.State {
			continue
		}
		issues = append(issues, *issue)
	}
	sort.Slice(issues, func(i, j int) bool {
		return issues[i].Number < issues[j].Number
	})
	return issues, nil
}

func (p *Provider) AddLabelToIssue(issueNumber int, label string) error {
	issue, ok := p.Issues[issueNumber]
	if !ok {
		return fmt.Errorf("issue %d not found", issueNumber)
	}
	issue.Labels = append(issue.Labels, label)
	p.Issues[issueNumber] = issue
	return p.Save()
}

func (p *Provider) FetchPullRequests(remotePath, label string) ([]types.PullRequest, error) {
	pullRequests := make([]types.PullRequest, 0, len(p.PullRequests))
	for _, pullRequest := range p.PullRequests {
		pullRequests = append(pullRequests, *pullRequest)
	}
	sort.Slice(pullRequests, func(i, j int) bool {
		return pullRequests[i].Number < pullRequests[j].Number
	})
	return pullRequests, nil
}

func (p *Provider) UpdatePullRequestState(remotePath string, prNumber int, state string) error {
	pullRequest, ok := p.PullRequests[prNumber]
	if !ok {
		return fmt.Errorf("pull request %d not found", prNumber)
	}
	pullRequest.State = state
	p.PullRequests[prNumber] = pullRequest
	return p.Save()
}

func (p *Provider) FetchDiffs(owner, repo string, resourceID int) (string, error) {
	pr, ok := p.PullRequests[resourceID]
	if !ok {
		return "", fmt.Errorf("pull request %d not found", resourceID)
	}

	// Create a temporary directory for the diff operation
	tmpDir, err := os.MkdirTemp("", "dev-team-diff-*")
	if err != nil {
		return "", fmt.Errorf("error creating temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Run git diff command
	cmd := exec.Command("git", "-C", p.Path, "diff", pr.BaseBranch+".."+pr.Branch)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("error generating diff: %v: %s", err, string(output))
	}

	return string(output), nil
}

func (p *Provider) FetchComments(owner, repo string, prNumber int) ([]*types.Comment, error) {
	pr, ok := p.PullRequests[prNumber]
	if !ok {
		return nil, fmt.Errorf("pull request %d not found", prNumber)
	}
	return pr.Comments, nil
}

func (p *Provider) AddCommentReaction(repoPath, reaction string, commentID int64) error {
	for _, pr := range p.PullRequests {
		for _, comment := range pr.Comments {
			if comment.ID == commentID {
				comment.Reactions = addReactionToReactions(comment.Reactions, reaction)
				return p.Save()
			}
		}
	}
	for _, issue := range p.Issues {
		for _, comment := range issue.Comments {
			if comment.ID == commentID {
				comment.Reactions = addReactionToReactions(comment.Reactions, reaction)
				return p.Save()
			}
		}
	}
	return fmt.Errorf("comment %d not found", commentID)
}

func (p *Provider) FetchRepositories() ([]types.Repository, error) {
	return nil, nil
}

func (p *Provider) CreateIssueComment(remotePath string, issueNumber int, comment types.Comment) error {
	issue, ok := p.Issues[issueNumber]
	if !ok {
		return fmt.Errorf("issue %d not found", issueNumber)
	}
	issue.Comments = append(issue.Comments, &comment)
	p.Issues[issueNumber] = issue
	return p.Save()
}

func (p *Provider) CreatePRComment(remotePath string, prNumber int, comment types.Comment) error {
	pr, ok := p.PullRequests[prNumber]
	if !ok {
		return fmt.Errorf("pull request %d not found", prNumber)
	}

	// If there's a diff hunk, validate it exists in the PR diff
	if comment.DiffHunk != "" {
		diff, err := p.FetchDiffs("", "", prNumber)
		if err != nil {
			return fmt.Errorf("error validating diff hunk: %v", err)
		}
		if !strings.Contains(diff, comment.DiffHunk) {
			return fmt.Errorf("diff hunk not found in PR diff")
		}
	}

	pr.Comments = append(pr.Comments, &comment)
	p.PullRequests[prNumber] = pr
	return p.Save()
}

func addReactionToReactions(reactions types.Reactions, reaction string) types.Reactions {
	reactions.TotalCount++
	switch reaction {
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
	return reactions
}

func (p *Provider) Save() error {
	// validate data path
	path, err := validatePath(dataPath)
	if err != nil {
		return fmt.Errorf("error validating data path: %v", err)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("error creating file: %v", err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(p)
	if err != nil {
		return fmt.Errorf("error encoding data: %v", err)
	}
	return nil
}

func loadProvider(path string) (*Provider, error) {
	// validate data path
	configPath, err := validatePath(dataPath)
	if err != nil {
		return nil, fmt.Errorf("error validating data path: %v", err)
	}

	// read file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("error reading file: %v", err)
	}

	// unmarshal data
	var provider Provider
	err = json.Unmarshal(data, &provider)
	if err != nil {
		p := &Provider{
			Path:         path,
			Issues:       make(map[int]*types.Issue),
			PullRequests: make(map[int]*types.PullRequest),
			IssueCounter: 0,
		}
		return p, fmt.Errorf("error unmarshalling data, creating blank provider: %v", err)
	}

	return &provider, nil
}

func validatePath(path string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("error getting user home directory: %v", err)
	}
	fullPath := filepath.Join(home, path)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("error getting absolute path: %v", err)
	}

	dir := filepath.Dir(absPath)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		err = os.MkdirAll(dir, 0755)
		if err != nil {
			return "", fmt.Errorf("error creating directory: %v", err)
		}
	}

	// create file if it doesn't exist
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		_, err = os.Create(absPath)
		if err != nil {
			return "", fmt.Errorf("error creating file: %v", err)
		}
	}
	return absPath, nil
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
