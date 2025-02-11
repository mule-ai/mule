package repository

import (
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

func (r *Repository) CreateBranch(branchName string) error {
	repo, err := git.PlainOpen(r.Path)
	if err != nil {
		return err
	}

	head, err := repo.Head()
	if err != nil {
		return err
	}
	// check if branch exists
	_, err = repo.Reference(plumbing.NewBranchReferenceName(branchName), true)
	if err == plumbing.ErrReferenceNotFound {
		// branch doesn't exist, create it
		ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branchName), head.Hash())
		return repo.Storer.SetReference(ref)
	} else if err != nil {
		return err
	}
	// branch exists, nothing to do
	return nil
}

func (r *Repository) CheckoutBranch(branchName string) error {
	repo, err := git.PlainOpen(r.Path)
	if err != nil {
		return err
	}

	w, err := repo.Worktree()
	if err != nil {
		return err
	}

	return w.Checkout(&git.CheckoutOptions{
		Branch: plumbing.NewBranchReferenceName(branchName),
	})
}

func (r *Repository) createIssueBranch(issueTitle string) (string, error) {
	branchName := strings.ToLower(strings.ReplaceAll(issueTitle, " ", "-"))
	if len(branchName) > 100 {
		branchName = branchName[:100]
	}

	err := r.Fetch()
	if err != nil {
		return "", fmt.Errorf("error fetching before creating branch: %w", err)
	}

	err = r.CheckoutBranch("main")
	if err != nil {
		return "", fmt.Errorf("error checking out main before creating branch: %w", err)
	}

	err = r.CreateBranch(branchName)
	if err != nil {
		return "", fmt.Errorf("error creating branch: %w", err)
	}

	err = r.CheckoutBranch(branchName)
	if err != nil {
		return "", fmt.Errorf("error checking out new branch: %w", err)
	}

	return branchName, nil
}

func (r *Repository) Reset() error {
	repo, err := git.PlainOpen(r.Path)
	if err != nil {
		return err
	}

	w, err := repo.Worktree()
	if err != nil {
		return err
	}

	return w.Reset(&git.ResetOptions{Mode: git.HardReset})
}
