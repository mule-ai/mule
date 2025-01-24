package repository

import (
	"fmt"
	"io"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

type BranchChanges struct {
	Files   []string
	Commits []*object.Commit
	Summary string
}

func getBranchChanges(repo *git.Repository, currentBranch string, targetBranch string) (*BranchChanges, error) {
	// Get references
	currentRef, err := repo.Reference(plumbing.NewBranchReferenceName(currentBranch), true)
	if err != nil {
		return nil, fmt.Errorf("error getting current branch ref: %v", err)
	}

	targetRef, err := repo.Reference(plumbing.NewBranchReferenceName(targetBranch), true)
	if err != nil {
		return nil, fmt.Errorf("error getting target branch ref: %v", err)
	}

	// Get commit objects
	currentCommit, err := repo.CommitObject(currentRef.Hash())
	if err != nil {
		return nil, fmt.Errorf("error getting current commit: %v", err)
	}

	targetCommit, err := repo.CommitObject(targetRef.Hash())
	if err != nil {
		return nil, fmt.Errorf("error getting target commit: %v", err)
	}

	// Find common ancestor
	isAncestor := false
	var mergeBase *object.Commit

	// First check if target is ancestor of current
	isAncestor, err = currentCommit.IsAncestor(targetCommit)
	if err != nil {
		return nil, fmt.Errorf("error checking ancestry: %v", err)
	}

	if isAncestor {
		mergeBase = targetCommit
	} else {
		// Then check if current is ancestor of target
		isAncestor, err = targetCommit.IsAncestor(currentCommit)
		if err != nil {
			return nil, fmt.Errorf("error checking ancestry: %v", err)
		}
		if isAncestor {
			mergeBase = currentCommit
		} else {
			// Find the most recent common ancestor
			commits, err := currentCommit.MergeBase(targetCommit)
			if err != nil {
				return nil, fmt.Errorf("error finding merge base: %v", err)
			}
			if len(commits) == 0 {
				return nil, fmt.Errorf("no common ancestor found between branches")
			}
			mergeBase = commits[0]
		}
	}

	// Get commit history from current branch up to merge base
	cIter, err := repo.Log(&git.LogOptions{From: currentRef.Hash()})
	if err != nil {
		return nil, fmt.Errorf("error getting commit history: %v", err)
	}

	var commits []*object.Commit
	var files = make(map[string]struct{})
	var summary strings.Builder

	err = cIter.ForEach(func(c *object.Commit) error {
		// Stop when we reach the merge base
		if c.Hash == mergeBase.Hash {
			return io.EOF
		}

		commits = append(commits, c)
		summary.WriteString("- " + c.Message + "\n")

		// Get files changed in this commit
		stats, err := c.Stats()
		if err != nil {
			return err
		}

		for _, stat := range stats {
			files[stat.Name] = struct{}{}
		}

		return nil
	})

	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("error iterating commits: %v", err)
	}

	// Convert files map to slice
	var filesList []string
	for file := range files {
		filesList = append(filesList, file)
	}

	return &BranchChanges{
		Files:   filesList,
		Commits: commits,
		Summary: summary.String(),
	}, nil
}
