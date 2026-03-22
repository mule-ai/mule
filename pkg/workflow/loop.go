package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mule-ai/mule/pkg/github"
	"github.com/mule-ai/mule/pkg/log"
)

type Issue struct {
	ID     string `json:"id"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
	URL    string `json:"url"`
	Repository struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
}

func RunIssueLoop(ctx context.Context) error {
	// Get GitHub token from environment
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		return fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}

	// Create GitHub client
	client := github.NewClient(token)

	// Fetch all assigned issues for the current user
	log.Info("Fetching all assigned issues...")
	issues, err := client.GetAssignedIssues(ctx, "@me")
	if err != nil {
		return fmt.Errorf("failed to fetch assigned issues: %w", err)
	}

	log.Info("Found %d assigned issues", len(issues))

	// Process each issue
	for _, issue := range issues {
		if err := processIssue(ctx, issue); err != nil {
			log.Error("Failed to process issue #%d: %v", issue.Number, err)
			continue
		}
	}

	return nil
}

func processIssue(ctx context.Context, issue Issue) error {
	log.Info("Processing issue #%d: %s", issue.Number, issue.Title)
	
	// Set up working directory for this issue's repository
	repoPath, err := setupWorkingDirectory(issue)
	if err != nil {
		return fmt.Errorf("failed to set up working directory: %w", err)
	}
	
	// Change to the repository directory
	if err := os.Chdir(repoPath); err != nil {
		return fmt.Errorf("failed to change to repository directory: %w", err)
	}
	
	// Process the issue (this would typically involve running workflows)
	log.Info("Processing issue in repository: %s/%s", issue.Repository.Owner.Login, issue.Repository.Name)
	
	return nil
}

func setupWorkingDirectory(issue Issue) (string, error) {
	// Create a base directory for repositories if it doesn't exist
	baseDir := filepath.Join(os.Getenv("HOME"), ".mule", "repos")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create base directory: %w", err)
	}
	
	// Create repository path
	repoPath := filepath.Join(baseDir, issue.Repository.Owner.Login, issue.Repository.Name)
	
	// Check if repository already exists
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		// Clone repository (mock implementation)
		log.Info("Cloning repository %s/%s to %s", issue.Repository.Owner.Login, issue.Repository.Name, repoPath)
		if err := os.MkdirAll(repoPath, 0755); err != nil {
			return "", fmt.Errorf("failed to create repository directory: %w", err)
		}
	} else {
		log.Info("Using existing repository at %s", repoPath)
	}
	
	return repoPath, nil
}