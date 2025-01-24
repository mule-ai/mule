package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"

	"github.com/go-git/go-git/v5"
)

type GitHubPRInput struct {
	Title               string `json:"title"`
	Description         string `json:"body"`
	Branch              string `json:"head"`
	Base                string `json:"base"`
	Draft               bool   `json:"draft"`
	MaintainerCanModify bool   `json:"maintainer_can_modify"`
}

type GitHubPRResponse struct {
	Number int `json:"number"`
}

type Repository struct {
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	CloneURL    string `json:"clone_url"`
	SSHURL      string `json:"ssh_url"`
}

type Issue struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	State     string `json:"state"`
	HTMLURL   string `json:"html_url"`
	CreatedAt string `json:"created_at"`
}

func CreateDraftPR(path string, githubToken string, input GitHubPRInput) error {
	repo, err := git.PlainOpen(path)
	if err != nil {
		return err
	}

	// Get remote URL to extract owner and repo name
	remote, err := repo.Remote("origin")
	if err != nil {
		return fmt.Errorf("error getting remote: %v", err)
	}

	remoteURL := remote.Config().URLs[0]
	// Extract owner and repo from SSH URL format (git@github.com:owner/repo.git)
	// or HTTPS URL format (https://github.com/owner/repo.git)
	var owner, repoName string
	if strings.Contains(remoteURL, "git@github.com:") {
		parts := strings.Split(strings.TrimPrefix(remoteURL, "git@github.com:"), "/")
		owner = parts[0]
		repoName = strings.TrimSuffix(parts[1], ".git")
	} else {
		parts := strings.Split(strings.TrimPrefix(remoteURL, "https://github.com/"), "/")
		owner = parts[0]
		repoName = strings.TrimSuffix(parts[1], ".git")
	}

	if githubToken == "" {
		return fmt.Errorf("GitHub token not provided in settings")
	}

	// Create PR using GitHub API
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls", owner, repoName)
	jsonData, err := json.Marshal(input)
	if err != nil {
		return fmt.Errorf("error marshaling PR request: %v", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Authorization", "token "+githubToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("error creating PR: %s", string(body))
	}

	var prResponse GitHubPRResponse
	if err := json.NewDecoder(resp.Body).Decode(&prResponse); err != nil {
		return fmt.Errorf("error decoding PR response: %v", err)
	}

	// include the pr link in the response
	prLink := fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repoName, prResponse.Number)
	log.Printf("PR created successfully: %s", prLink)

	return nil
}

// FetchRepositories gets the list of repositories for the authenticated user
func FetchRepositories(githubToken string) ([]Repository, error) {
	if githubToken == "" {
		return nil, fmt.Errorf("GitHub token not provided in settings")
	}

	url := "https://api.github.com/user/repos?sort=updated&per_page=100"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Authorization", "token "+githubToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error fetching repositories: %s", string(body))
	}

	var repos []Repository
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		return nil, fmt.Errorf("error decoding response: %v", err)
	}

	return repos, nil
}

// FetchIssues gets the list of issues with a specific label for a repository
func FetchIssues(owner, repo, label, githubToken string) ([]Issue, error) {
	if githubToken == "" {
		return nil, fmt.Errorf("GitHub token not provided in settings")
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues?labels=%s&state=all", owner, repo, label)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Authorization", "token "+githubToken)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error making request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("error fetching issues: %s", string(body))
	}

	var issues []Issue
	if err := json.NewDecoder(resp.Body).Decode(&issues); err != nil {
		return nil, fmt.Errorf("error decoding response: %v", err)
	}

	return issues, nil
}
