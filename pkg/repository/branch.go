package repository

import (
	"fmt"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"strings"
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

	ref := plumbing.NewHashReference(plumbing.NewBranchReferenceName(branchName), head.Hash())
	return repo.Storer.SetReference(ref)
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
