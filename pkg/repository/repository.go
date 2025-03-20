package repository

import (
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/mule-ai/mule/internal/settings"
	"github.com/mule-ai/mule/pkg/agent"
	"github.com/mule-ai/mule/pkg/auth"
	"github.com/mule-ai/mule/pkg/remote"
	"github.com/mule-ai/mule/pkg/remote/types"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
)

/*
TODO:
This package needs a refactor.
Ever since implementing the remote provider interface, this package has
some duplicate types. The types from `remote/types` should be used instead.
*/

type Repository struct {
	Path           string                  `json:"path"`
	RemoteProvider remote.ProviderSettings `json:"remoteProvider"`
	Schedule       string                  `json:"schedule"`
	LastSync       time.Time               `json:"lastSync"`
	State          *Status                 `json:"status,omitempty"`
	RemotePath     string                  `json:"remotePath,omitempty"`
	Issues         map[int]*Issue          `json:"-"`
	PullRequests   map[int]*PullRequest    `json:"-"`
	Mu             sync.RWMutex            `json:"-"`
	Locked         bool                    `json:"locked"`
	Logger         logr.Logger             `json:"-"`
	Remote         remote.Provider         `json:"-"`
}

type Changes struct {
	Files   []string
	Commits []string
	Summary string
}

func NewRepository(path string) *Repository {
	return &Repository{
		Path:         path,
		Issues:       make(map[int]*Issue),
		PullRequests: make(map[int]*PullRequest),
		Mu:           sync.RWMutex{},
		Locked:       false,
		Remote: remote.New(remote.ProviderOptions{
			Type: remote.LOCAL,
			Path: path,
		}),
		RemoteProvider: remote.ProviderSettings{
			Provider: remote.ProviderTypeToString(remote.LOCAL),
			Path:     path,
		},
	}
}

func NewRepositoryWithRemote(path string, remote remote.Provider) *Repository {
	return &Repository{
		Path:         path,
		Issues:       make(map[int]*Issue),
		PullRequests: make(map[int]*PullRequest),
		Mu:           sync.RWMutex{},
		Locked:       false,
		Remote:       remote,
	}
}

func (r *Repository) Clone(repoURL string) error {
	auth, err := auth.GetSSHAuth()
	if err != nil {
		return fmt.Errorf("SSH authentication error: %v", err)
	}

	// set remote path
	r.RemotePath = strings.TrimPrefix(repoURL, "git@github.com:")
	r.RemotePath = strings.TrimSuffix(r.RemotePath, ".git")

	// update url to use ssh
	repoURL = strings.Replace(repoURL, "https://github.com/", "git@github.com:", 1)

	_, err = git.PlainClone(r.Path, false, &git.CloneOptions{
		URL:      repoURL,
		Progress: nil,
		Auth:     auth,
	})
	if err != nil {
		r.Logger.Error(err, "Error cloning repository", "repoURL", repoURL, "path", r.Path)
		return fmt.Errorf("error cloning repository: %v", err)
	}
	return nil
}

func (r *Repository) Upsert(repoURL string) error {
	_, err := git.PlainOpen(r.Path)
	if err == git.ErrRepositoryNotExists {
		return r.Clone(repoURL)
	}
	if err != nil {
		return err
	}

	err = r.Fetch()
	if err != nil {
		return err
	}

	return r.CheckoutBranch("main")
}

func (r *Repository) Commit(message string) error {
	repo, err := git.PlainOpen(r.Path)
	if err != nil {
		return err
	}

	w, err := repo.Worktree()
	if err != nil {
		return err
	}

	status, err := w.Status()
	if err != nil {
		return err
	}

	if status.IsClean() {
		return nil
	}

	// Add all changes
	_, err = w.Add(".")
	if err != nil {
		return err
	}

	_, err = w.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "mule",
			Email: "mule@muleai.io",
			When:  time.Now(),
		},
	})

	return err
}

func (r *Repository) Push() error {
	repo, err := git.PlainOpen(r.Path)
	if err != nil {
		return err
	}

	// Get SSH authentication
	auth, err := auth.GetSSHAuth()
	if err != nil {
		return fmt.Errorf("SSH authentication error: %v", err)
	}

	currentBranch, err := repo.Head()
	if err != nil {
		return err
	}

	refSpecStr := fmt.Sprintf(
		"+%s:refs/heads/%s",
		currentBranch.Name().String(),
		currentBranch.Name().Short(),
	)
	refSpec := config.RefSpec(refSpecStr)
	r.Logger.Info("Pushing", "refSpec", refSpec)
	// Update push options to include SSH auth
	return repo.Push(&git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{refSpec},
		Auth:       auth,
	})
}

func (r *Repository) Fetch() error {
	repo, err := git.PlainOpen(r.Path)
	if err != nil {
		return err
	}

	auth, err := auth.GetSSHAuth()
	if err != nil {
		return fmt.Errorf("SSH authentication error: %v", err)
	}

	err = repo.Fetch(&git.FetchOptions{
		Auth: auth,
	})
	if err != nil && err != git.NoErrAlreadyUpToDate {
		return err
	}
	return nil
}

func (r *Repository) Sync(agents map[int]*agent.Agent, workflow struct {
	Steps               []agent.WorkflowStep
	ValidationFunctions []string
}) error {
	r.Logger.Info("Syncing repository")
	if len(agents) == 0 {
		return fmt.Errorf("no agents provided")
	}
	err := r.lock()
	if err != nil {
		r.Logger.Error(err, "Error locking repository")
		return err
	}
	defer r.unlock()

	err = r.UpdateStatus()
	if err != nil {
		r.Logger.Error(err, "Error getting repo status")
		return err
	}

	// get latest pull requests
	err = r.UpdatePullRequests()
	if err != nil {
		r.Logger.Error(err, "Error updating pull requests")
		return err
	}

	// get latest issues
	err = r.UpdateIssues()
	if err != nil {
		r.Logger.Error(err, "Error updating issues")
		return err
	}
	// add pull requests to issues
	for _, issue := range r.Issues {
		issue.addPullRequests(r.PullRequests)
	}

	// select issue to work on
	for _, issue := range r.Issues {
		// if there are existing changes, log because we can't start work
		if r.State.HasChanges {
			r.Logger.Info("There are existing changes, resetting")
			err = r.Reset()
			if err != nil {
				r.Logger.Error(err, "Error resetting repository")
				return err
			}
		}

		if issue.Completed() {
			r.Logger.Info("Issue already completed", "path", r.Path, "issue", issue.ID)
			continue
		}

		// checkout new branch for issue
		branchName, err := r.createIssueBranch(issue.Title)
		if err != nil {
			return fmt.Errorf("error creating issue branch: %w", err)
		}
		r.State.CurrentBranch = branchName

		r.Logger.Info("Starting generation")
		commentResolved, err := r.generateFromIssue(agents, workflow, issue)
		if err != nil {
			r.Logger.Error(err, "Error generating changes")
			return err
		}
		if commentResolved {
			r.Logger.Info("PR comment resolved, skipping PR creation")
			continue
		}

		// validate that generation resulted in changes
		err = r.UpdateStatus()
		if err != nil {
			r.Logger.Error(err, "Error updating status")
			return err
		}

		if !r.State.HasChanges {
			r.Logger.Info("No changes found, expected changes from AI")
			return fmt.Errorf("no changes found, expected changes from AI")
		}
		err = r.createPR(agents, issue)
		if err != nil {
			r.Logger.Error(err, "Error creating PR")
			return err
		}
	}
	return nil
}

func (r *Repository) ChangeSummary() (string, error) {
	changes, err := r.getChanges()
	if err != nil {
		return "", err
	}
	return changes.Summary, nil
}

func (r *Repository) getChanges() (*Changes, error) {
	repo, err := git.PlainOpen(r.Path)
	if err != nil {
		return nil, err
	}

	w, err := repo.Worktree()
	if err != nil {
		return nil, err
	}

	status, err := w.Status()
	if err != nil {
		return nil, err
	}

	var files []string
	for file := range status {
		files = append(files, file)
	}

	head, err := repo.Head()
	if err != nil {
		return nil, err
	}

	currentBranch := head.Name().Short()
	branchChanges, err := getBranchChanges(repo, currentBranch, "main")
	if err != nil {
		return nil, fmt.Errorf("error getting branch changes: %v", err)
	}

	// Convert commits to messages
	var commits []string
	for _, commit := range branchChanges.Commits {
		commits = append(commits, commit.Message)
	}

	// Add any files from branch changes that aren't already included
	fileSet := make(map[string]struct{})
	for _, file := range files {
		fileSet[file] = struct{}{}
	}
	for _, file := range branchChanges.Files {
		if _, exists := fileSet[file]; !exists {
			files = append(files, file)
		}
	}

	// Get the diff between working directory and main branch
	cmd := exec.Command("git", "-C", r.Path, "diff", "main")
	diffOutput, err := cmd.CombinedOutput()
	if err != nil {
		// If main doesn't exist or other error, just show all changes
		cmd = exec.Command("git", "-C", r.Path, "diff")
		diffOutput, err = cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("error generating diff: %v: %s", err, string(diffOutput))
		}
	}

	summary := fmt.Sprintf("Changed files:\n%v\n\nDiff:\n%s\n\nCommits:\n%v",
		strings.Join(files, "\n"),
		string(diffOutput),
		strings.Join(commits, "\n"))

	return &Changes{
		Files:   files,
		Commits: commits,
		Summary: summary,
	}, nil
}

func (r *Repository) generateFromIssue(agents map[int]*agent.Agent, workflow struct {
	Steps               []agent.WorkflowStep
	ValidationFunctions []string
}, issue *Issue) (bool, error) {
	prompt := ""
	var unresolvedCommentId int64
	var promptInput agent.PromptInput
	// if issue has not PR, send issue prompt
	if !issue.PrExists() {
		promptInput = agent.PromptInput{
			IssueTitle:  issue.Title,
			IssueBody:   issue.Body,
			Commits:     "",
			Diff:        "",
			PRComment:   prompt,
			IsPRComment: false,
		}
	} else {
		// if issue has PR, send PR comment prompt
		pr, hasUnresolvedComments := issue.PRHasUnresolvedComments()
		unresolvedComment := pr.FirstUnresolvedComment()
		if hasUnresolvedComments {
			unresolvedCommentId = unresolvedComment.ID
			promptInput = agent.PromptInput{
				IssueTitle:        issue.Title,
				IssueBody:         issue.Body,
				Commits:           "",
				Diff:              pr.Diff,
				PRComment:         unresolvedComment.Body,
				PRCommentDiffHunk: unresolvedComment.DiffHunk,
				IsPRComment:       true,
			}
		} else {
			return false, fmt.Errorf("expected PR with unresolved comments, but none found")
		}
	}

	// err := agent.RunWorkflow(agents, promptInput, r.Path)
	_, err := agent.ExecuteWorkflow(workflow.Steps, agents, promptInput, r.Path, r.Logger, workflow.ValidationFunctions)
	if err != nil {
		r.Logger.Error(err, "Error running agent")
		return false, err
	}

	// If we're handling a PR comment, commit and push to the existing branch
	if unresolvedCommentId != 0 {
		return true, r.updatePR(agents, unresolvedCommentId)
	}
	return false, nil
}

func (r *Repository) updatePR(agents map[int]*agent.Agent, commentId int64) error {
	if commentId == 0 {
		return fmt.Errorf("expected PR comment ID, but none found")
	}
	summary, err := r.ChangeSummary()
	if err != nil {
		r.Logger.Error(err, "Error getting change summary")
		return err
	}
	commitMessage, err := agents[settings.CommitAgent].Generate("", agent.PromptInput{
		IssueTitle:  "",
		IssueBody:   "",
		Commits:     "",
		Diff:        summary,
		PRComment:   "",
		IsPRComment: false,
	})
	if err != nil {
		r.Logger.Error(err, "Error generating commit message")
		return err
	}

	// Commit changes
	err = r.Commit(commitMessage)
	if err != nil {
		r.Logger.Error(err, "Error committing changes")
		return err
	}

	// Push changes
	err = r.Push()
	if err != nil {
		r.Logger.Error(err, "Error pushing changes")
		return err
	}

	// Add reaction to mark comment as addressed
	err = r.Remote.AddCommentReaction(r.RemotePath, "+1", commentId)
	if err != nil {
		r.Logger.Error(err, "Error acknowledging PR comment")
		return err
	}
	return nil
}

// ignore unused code error
func (r *Repository) createPR(agents map[int]*agent.Agent, issue *Issue) error {
	summary, err := r.ChangeSummary()
	if err != nil {
		r.Logger.Error(err, "Error getting change summary")
		return err
	}

	promptInput := agent.PromptInput{
		IssueTitle:  issue.Title,
		IssueBody:   issue.Body,
		Commits:     "",
		Diff:        summary,
		PRComment:   "",
		IsPRComment: false,
	}

	commitMessage, err := agents[settings.CommitAgent].Generate("", promptInput)
	if err != nil {
		r.Logger.Error(err, "Error generating commit message")
		return err
	}
	// Commit changes
	err = r.Commit(commitMessage)
	if err != nil {
		r.Logger.Error(err, "Error committing changes")
		return err
	}

	// Push changes
	err = r.Push()
	if err != nil {
		r.Logger.Error(err, "Error pushing changes")
		return err
	}

	prTitle, err := agents[settings.PRTitleAgent].Generate("", promptInput)
	if err != nil {
		r.Logger.Error(err, "Error generating PR title")
		return err
	}

	prDescription, err := agents[settings.PRBodyAgent].Generate("", promptInput)
	if err != nil {
		r.Logger.Error(err, "Error generating PR description")
		return err
	}

	// add issue close tag to description
	prDescription = fmt.Sprintf("%s\n\n%s\n<!--%s-->",
		prDescription,
		fmt.Sprintf("Closes #%d", issue.ID),
		issue.SourceURL)
	return r.Remote.CreateDraftPR(r.Path, types.PullRequestInput{
		Title:               prTitle,
		Branch:              r.State.CurrentBranch,
		Base:                "main",
		Description:         prDescription,
		Draft:               true,
		MaintainerCanModify: true,
	})
}

func (r *Repository) lock() error {
	r.Mu.RLock()
	locked := r.Locked
	r.Mu.RUnlock()
	if locked {
		return fmt.Errorf("repository is locked")
	}
	r.Mu.Lock()
	defer r.Mu.Unlock()
	r.Locked = true
	return nil
}

func (r *Repository) unlock() {
	r.Mu.Lock()
	defer r.Mu.Unlock()
	r.Locked = false
}
