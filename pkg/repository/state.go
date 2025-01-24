package repository

import (
	"time"

	"github.com/go-git/go-git/v5"
)

type Status struct {
	HasChanges    bool     `json:"hasChanges"`
	ChangedFiles  []string `json:"changedFiles"`
	CurrentBranch string   `json:"currentBranch"`
	IsClean       bool     `json:"isClean"`
}

func (r *Repository) UpdateStatus() error {
	status, err := r.Status()
	if err != nil {
		return err
	}
	r.State = status
	r.LastSync = time.Now()
	return nil
}

func (r *Repository) Status() (*Status, error) {
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

	head, err := repo.Head()
	if err != nil {
		return nil, err
	}

	changedFiles := []string{}
	for file, fileStatus := range status {
		if fileStatus.Staging != git.Unmodified || fileStatus.Worktree != git.Unmodified {
			changedFiles = append(changedFiles, file)
		}
	}

	return &Status{
		HasChanges:    !status.IsClean(),
		ChangedFiles:  changedFiles,
		CurrentBranch: head.Name().Short(),
		IsClean:       status.IsClean(),
	}, nil
}
