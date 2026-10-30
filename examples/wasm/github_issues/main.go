package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

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

func main() {
	ctx := context.Background()
	
	// Get GitHub token from environment
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required")
	}

	// Create GitHub client
	client := github.NewClient(token)

	// Fetch all assigned issues for the current user
	issues, err := client.GetAssignedIssues(ctx, "@me")
	if err != nil {
		log.Fatalf("Failed to fetch assigned issues: %v", err)
	}

	// Convert issues to JSON
	result, err := json.Marshal(issues)
	if err != nil {
		log.Fatalf("Failed to marshal issues: %v", err)
	}

	// Output result
	fmt.Print(string(result))
}