package repository

import (
	"dev-team/pkg/auth"
	"fmt"
	"genai"
	"log"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// set static model const until agent is implemented
const MODEL = "models/gemini-2.0-flash-exp"

type Repository struct {
	Path         string              `json:"path"`
	Schedule     string              `json:"schedule"`
	LastSync     time.Time           `json:"lastSync"`
	State        *Status             `json:"status,omitempty"`
	Issues       map[int]Issue       `json:"issues,omitempty"`
	PullRequests map[int]PullRequest `json:"pullRequests,omitempty"`
	RemotePath   string              `json:"remotePath,omitempty"`
}

type Changes struct {
	Files   []string
	Commits []string
	Summary string
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
		log.Printf("Error cloning repository: %s into %s: error: %v", repoURL, r.Path, err)
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
	log.Printf("Pushing %s", refSpec)
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
	err := r.UpdateStatus()
	if err != nil {
		log.Printf("Error getting repo status: %v", err)
		return err
	}

	err = r.UpdateIssues(token)
	if err != nil {
		log.Printf("Error updating issues: %v", err)
		return err
	}

	// if there are existing changes, log because we can't start work
	if r.State.HasChanges {
		log.Printf("There are existing changes, skipping sync")
		return fmt.Errorf("there are existing changes, skipping sync")
	}

	if len(r.Issues) == 0 {
		log.Printf("No issues found, skipping sync")
		return fmt.Errorf("no issues found, skipping sync")
	}

	var currentIssue Issue
	for _, issue := range r.Issues {
		currentIssue = issue
		break
	}

	log.Printf("Current issue: %s", currentIssue.ToString())
	log.Println("Starting generation")
	// generate changes for issue
	changes, err := aiService.Generate(
		MODEL,
		currentIssue.ToString(),
	)
	if err != nil {
		log.Printf("Error generating changes: %v", err)
		return err
	}

	log.Printf("Changes: %v", changes)
	/*
		// Commit changes
		err = r.Commit("Commit message")
		if err != nil {
			log.Printf("Error committing changes: %v", err)
			return err
		}

		// Push changes
		err = r.Push()
		if err != nil {
			log.Printf("Error pushing changes: %v", err)
			return err
		}

		summary, err := r.ChangeSummary()
		if err != nil {
			log.Printf("Error getting change summary: %v", err)
			return err
		}

		prTitle, err := genai.Chat(CommitPrompt(summary), aiService)
		if err != nil {
			log.Printf("Error generating PR title: %v", err)
			return err
		}

		prDescription, err := genai.Chat(PRPrompt(summary), aiService)
		if err != nil {
			log.Printf("Error generating PR description: %v", err)
			return err
		}

		err = github.CreateDraftPR(r.Path, token, github.GitHubPRInput{
			Title:               prTitle,
			Branch:              r.State.CurrentBranch,
			Base:                "main",
			Description:         prDescription,
			Draft:               true,
			MaintainerCanModify: true,
		})
		if err != nil {
			log.Printf("Error creating PR: %v", err)
			return err
		}
	*/
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
