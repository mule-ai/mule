package repository

import (
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
