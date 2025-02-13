package repository

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/jbutlerdev/dev-team/pkg/auth"
	"github.com/jbutlerdev/dev-team/pkg/remote"
	"github.com/jbutlerdev/dev-team/pkg/remote/types"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/jbutlerdev/genai"
	genaitools "github.com/jbutlerdev/genai/tools"
)

// set static model const until agent is implemented
const MODEL = "models/gemini-2.0-flash"

// const MODEL = "qwen2.5:7b-instruct-q6_K"

type Repository struct {
	Path           string                  `json:"path"`
	RemoteProvider remote.ProviderSettings `json:"remoteProvider"`
	Schedule       string                  `json:"schedule"`
	LastSync       time.Time               `json:"lastSync"`
	State          *Status                 `json:"status,omitempty"`
	RemotePath     string                  `json:"remotePath,omitempty"`
	Issues         map[int]*Issue          `json:"issues,omitempty"`
	PullRequests   map[int]*PullRequest    `json:"pullRequests,omitempty"`
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
			Name:  "GitWatcher",
			Email: "gitwatcher@local",
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

func (r *Repository) Sync(aiService *genai.Provider, token string) error {
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
	err = r.UpdatePullRequests(token)
	if err != nil {
		r.Logger.Error(err, "Error updating pull requests")
		return err
	}

	// get latest issues
	err = r.UpdateIssues(token)
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
		commentResolved, err := r.generateFromIssue(aiService, issue)
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
		err = r.createPR(aiService, issue)
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

	return &Changes{
		Files:   files,
		Commits: commits,
		Summary: fmt.Sprintf("Changed files:\n%v\n\nCommits:\n%v", files, commits),
	}, nil
}

func (r *Repository) generateFromIssue(aiService *genai.Provider, issue *Issue) (bool, error) {
	// generate changes for issue
	toolsToUse, err := genaitools.GetTools([]string{"writeFile", "tree", "readFile"})
	if err != nil {
		r.Logger.Error(err, "Error getting tools")
		return false, err
	}
	for _, tool := range toolsToUse {
		tool.Options["basePath"] = r.Path
	}
	chat := aiService.Chat(MODEL, toolsToUse)

	go func() {
		for response := range chat.Recv {
			r.Logger.Info("Response", "response", response)
		}
	}()
	var unresolvedCommentId int64
	// if issue has not PR, send issue prompt
	if !issue.PrExists() {
		chat.Send <- IssuePrompt(issue.ToString())
	} else {
		// if issue has PR, send PR comment prompt
		pr, hasUnresolvedComments := issue.PRHasUnresolvedComments()
		unresolvedComment := pr.FirstUnresolvedComment()
		if hasUnresolvedComments {
			unresolvedCommentId = unresolvedComment.ID
			chat.Send <- PRCommentPrompt(issue.ToString(), pr.Diff, unresolvedComment.Body, unresolvedComment.DiffHunk)
		} else {
			return false, fmt.Errorf("expected PR with unresolved comments, but none found")
		}
	}

	defer func() {
		chat.Done <- true
	}()
	// block until generation is complete
	<-chat.GenerationComplete
	// validate output
	err = r.validateOutput(&ValidationInput{
		attempts:    10,
		validations: []func(string) (string, error){getDeps, goFmt, goModTidy, golangciLint, goTest},
		send:        chat.Send,
		done:        chat.GenerationComplete,
	})
	if err != nil {
		r.Logger.Error(err, "Error validating output")
		return false, err
	}

	// If we're handling a PR comment, commit and push to the existing branch
	if unresolvedCommentId != 0 {
		return true, r.updatePR(aiService, unresolvedCommentId)
	}
	return false, nil
}

func (r *Repository) updatePR(aiService *genai.Provider, commentId int64) error {
	if commentId == 0 {
		return fmt.Errorf("expected PR comment ID, but none found")
	}
	summary, err := r.ChangeSummary()
	if err != nil {
		r.Logger.Error(err, "Error getting change summary")
		return err
	}
	commitMessage, err := aiService.Generate(MODEL, CommitPrompt(summary))
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
	err = r.Remote.AddCommentReaction(r.Path, "+1", commentId)
	if err != nil {
		r.Logger.Error(err, "Error acknowledging PR comment")
		return err
	}
	return nil
}

// ignore unused code error
func (r *Repository) createPR(aiService *genai.Provider, issue *Issue) error {
	summary, err := r.ChangeSummary()
	if err != nil {
		r.Logger.Error(err, "Error getting change summary")
		return err
	}

	commitMessage, err := aiService.Generate(MODEL, CommitPrompt(summary))
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

	prTitle, err := aiService.Generate(MODEL, CommitPrompt(summary))
	if err != nil {
		r.Logger.Error(err, "Error generating PR title")
		return err
	}

	prDescription, err := aiService.Generate(MODEL, PRPrompt(summary))
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
