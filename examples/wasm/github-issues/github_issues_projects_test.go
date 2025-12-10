package main

import (
	"encoding/json"
	"testing"
)

// Label represents a GitHub label
type Label struct {
	ID          int    `json:"id"`
	NodeID      string `json:"node_id"`
	URL         string `json:"url"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Color       string `json:"color"`
	Default     bool   `json:"default"`
}

// GitHubUser represents a GitHub user
type GitHubUser struct {
	Login     string `json:"login"`
	ID        int    `json:"id"`
	AvatarURL string `json:"avatar_url,omitempty"`
	URL       string `json:"url,omitempty"`
}

// GitHubComment represents a GitHub issue comment
type GitHubComment struct {
	ID        int        `json:"id"`
	Body      string     `json:"body"`
	User      GitHubUser `json:"user"`
	CreatedAt string     `json:"created_at"`
	UpdatedAt string     `json:"updated_at"`
	URL       string     `json:"url,omitempty"`
}

// GitHubProject represents GitHub project information
type GitHubProject struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// GitHubField represents a field in a GitHub project with its value
type GitHubField struct {
	Name  string      `json:"name"`
	Type  string      `json:"type"`
	Value interface{} `json:"value,omitempty"`
}

// GitHubIssue represents an extended GitHub issue structure with project information
type GitHubIssue struct {
	ID            int             `json:"id"`
	Number        int             `json:"number"`
	Title         string          `json:"title"`
	State         string          `json:"state"`
	URL           string          `json:"url"`
	Body          string          `json:"body"`
	CommentsCount int             `json:"comments"`
	Labels        []Label         `json:"labels"`
	Assignee      *GitHubUser     `json:"assignee,omitempty"`
	Assignees     []GitHubUser    `json:"assignees,omitempty"`
	Comments      []GitHubComment `json:"comments_data,omitempty"`
	Project       *GitHubProject  `json:"project,omitempty"`
	Fields        []GitHubField   `json:"fields,omitempty"`
}

// Test that our filtering logic works correctly
func TestFilterDeletedCommentsLogic(t *testing.T) {
	// Simulate comment data that might come from GitHub API
	comments := []struct {
		ID   int
		Body string
	}{
		{1, "This is a valid comment"},
		{2, ""}, // Empty body - should be filtered out
		{3, "Another valid comment"},
		{4, ""}, // Another empty comment
		{5, "Final valid comment"},
	}

	// Apply our filtering logic
	filteredComments := make([]struct {
		ID   int
		Body string
	}, 0, len(comments))

	for _, comment := range comments {
		// Skip comments with empty bodies (our implementation)
		if comment.Body != "" {
			filteredComments = append(filteredComments, comment)
		}
	}

	// Check that we have the expected number of comments
	expectedCount := 3 // Only non-empty comments
	if len(filteredComments) != expectedCount {
		t.Errorf("Expected %d comments, got %d", expectedCount, len(filteredComments))
	}

	// Check that the correct comments remain
	if filteredComments[0].ID != 1 || filteredComments[0].Body != "This is a valid comment" {
		t.Errorf("First comment was not preserved correctly")
	}

	if filteredComments[1].ID != 3 || filteredComments[1].Body != "Another valid comment" {
		t.Errorf("Third comment was not preserved correctly")
	}

	if filteredComments[2].ID != 5 || filteredComments[2].Body != "Final valid comment" {
		t.Errorf("Fifth comment was not preserved correctly")
	}
}

// Test that our Label struct works correctly
func TestLabelStruct(t *testing.T) {
	// Create a sample label
	label := Label{
		ID:          123456789,
		NodeID:      "MDU6TGFiZWwxMjM0NTY3ODk=",
		URL:         "https://api.github.com/repos/owner/repo/labels/bug",
		Name:        "bug",
		Description: "Something isn't working",
		Color:       "e11d21",
		Default:     true,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(label)
	if err != nil {
		t.Fatalf("Failed to marshal label: %v", err)
	}

	// Unmarshal back
	var unmarshaledLabel Label
	err = json.Unmarshal(jsonData, &unmarshaledLabel)
	if err != nil {
		t.Fatalf("Failed to unmarshal label: %v", err)
	}

	// Check that all fields match
	if label.ID != unmarshaledLabel.ID {
		t.Errorf("ID mismatch: expected %d, got %d", label.ID, unmarshaledLabel.ID)
	}
	if label.NodeID != unmarshaledLabel.NodeID {
		t.Errorf("NodeID mismatch: expected %s, got %s", label.NodeID, unmarshaledLabel.NodeID)
	}
	if label.URL != unmarshaledLabel.URL {
		t.Errorf("URL mismatch: expected %s, got %s", label.URL, unmarshaledLabel.URL)
	}
	if label.Name != unmarshaledLabel.Name {
		t.Errorf("Name mismatch: expected %s, got %s", label.Name, unmarshaledLabel.Name)
	}
	if label.Description != unmarshaledLabel.Description {
		t.Errorf("Description mismatch: expected %s, got %s", label.Description, unmarshaledLabel.Description)
	}
	if label.Color != unmarshaledLabel.Color {
		t.Errorf("Color mismatch: expected %s, got %s", label.Color, unmarshaledLabel.Color)
	}
	if label.Default != unmarshaledLabel.Default {
		t.Errorf("Default mismatch: expected %t, got %t", label.Default, unmarshaledLabel.Default)
	}
}

// Test that our GitHubIssue struct with Labels works correctly
func TestGitHubIssueWithLabelsStruct(t *testing.T) {
	// Create a sample issue with labels
	issue := GitHubIssue{
		ID:            123456,
		Number:        1,
		Title:         "Test issue",
		State:         "open",
		URL:           "https://github.com/owner/repo/issues/1",
		Body:          "This is a test issue",
		CommentsCount: 5,
		Labels: []Label{
			{
				ID:          123456789,
				NodeID:      "MDU6TGFiZWwxMjM0NTY3ODk=",
				URL:         "https://api.github.com/repos/owner/repo/labels/bug",
				Name:        "bug",
				Description: "Something isn't working",
				Color:       "e11d21",
				Default:     true,
			},
			{
				ID:          987654321,
				NodeID:      "MDU6TGFiZWw5ODc2NTQzMjE=",
				URL:         "https://api.github.com/repos/owner/repo/labels/help%20wanted",
				Name:        "help wanted",
				Description: "Extra attention is needed",
				Color:       "008672",
				Default:     true,
			},
		},
		Assignee: &GitHubUser{
			Login:     "testuser",
			ID:        12345,
			AvatarURL: "https://avatars.githubusercontent.com/u/12345?v=4",
			URL:       "https://api.github.com/users/testuser",
		},
		Assignees: []GitHubUser{
			{
				Login:     "testuser",
				ID:        12345,
				AvatarURL: "https://avatars.githubusercontent.com/u/12345?v=4",
				URL:       "https://api.github.com/users/testuser",
			},
		},
		Comments: []GitHubComment{
			{
				ID:   789012,
				Body: "Test comment",
				User: GitHubUser{
					Login:     "commenter",
					ID:        67890,
					AvatarURL: "https://avatars.githubusercontent.com/u/67890?v=4",
					URL:       "https://api.github.com/users/commenter",
				},
				CreatedAt: "2023-01-01T12:00:00Z",
				UpdatedAt: "2023-01-01T12:00:00Z",
				URL:       "https://api.github.com/repos/owner/repo/issues/comments/789012",
			},
		},
		Project: &GitHubProject{
			ID:    "PN_kwDOAHzBVc4AAhFq",
			Title: "Test Project",
			URL:   "https://github.com/users/owner/projects/1",
		},
		Fields: []GitHubField{
			{
				Name:  "Status",
				Type:  "TEXT",
				Value: "In Progress",
			},
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(issue)
	if err != nil {
		t.Fatalf("Failed to marshal issue: %v", err)
	}

	// Unmarshal back
	var unmarshaledIssue GitHubIssue
	err = json.Unmarshal(jsonData, &unmarshaledIssue)
	if err != nil {
		t.Fatalf("Failed to unmarshal issue: %v", err)
	}

	// Check that all fields match
	if issue.ID != unmarshaledIssue.ID {
		t.Errorf("ID mismatch: expected %d, got %d", issue.ID, unmarshaledIssue.ID)
	}
	if issue.Number != unmarshaledIssue.Number {
		t.Errorf("Number mismatch: expected %d, got %d", issue.Number, unmarshaledIssue.Number)
	}
	if issue.Title != unmarshaledIssue.Title {
		t.Errorf("Title mismatch: expected %s, got %s", issue.Title, unmarshaledIssue.Title)
	}
	if issue.State != unmarshaledIssue.State {
		t.Errorf("State mismatch: expected %s, got %s", issue.State, unmarshaledIssue.State)
	}
	if issue.URL != unmarshaledIssue.URL {
		t.Errorf("URL mismatch: expected %s, got %s", issue.URL, unmarshaledIssue.URL)
	}
	if issue.Body != unmarshaledIssue.Body {
		t.Errorf("Body mismatch: expected %s, got %s", issue.Body, unmarshaledIssue.Body)
	}
	if issue.CommentsCount != unmarshaledIssue.CommentsCount {
		t.Errorf("CommentsCount mismatch: expected %d, got %d", issue.CommentsCount, unmarshaledIssue.CommentsCount)
	}

	// Check labels
	if len(issue.Labels) != len(unmarshaledIssue.Labels) {
		t.Fatalf("Labels count mismatch: expected %d, got %d", len(issue.Labels), len(unmarshaledIssue.Labels))
	}

	for i, label := range issue.Labels {
		unmarshaledLabel := unmarshaledIssue.Labels[i]
		if label.ID != unmarshaledLabel.ID {
			t.Errorf("Label[%d] ID mismatch: expected %d, got %d", i, label.ID, unmarshaledLabel.ID)
		}
		if label.NodeID != unmarshaledLabel.NodeID {
			t.Errorf("Label[%d] NodeID mismatch: expected %s, got %s", i, label.NodeID, unmarshaledLabel.NodeID)
		}
		if label.URL != unmarshaledLabel.URL {
			t.Errorf("Label[%d] URL mismatch: expected %s, got %s", i, label.URL, unmarshaledLabel.URL)
		}
		if label.Name != unmarshaledLabel.Name {
			t.Errorf("Label[%d] Name mismatch: expected %s, got %s", i, label.Name, unmarshaledLabel.Name)
		}
		if label.Description != unmarshaledLabel.Description {
			t.Errorf("Label[%d] Description mismatch: expected %s, got %s", i, label.Description, unmarshaledLabel.Description)
		}
		if label.Color != unmarshaledLabel.Color {
			t.Errorf("Label[%d] Color mismatch: expected %s, got %s", i, label.Color, unmarshaledLabel.Color)
		}
		if label.Default != unmarshaledLabel.Default {
			t.Errorf("Label[%d] Default mismatch: expected %t, got %t", i, label.Default, unmarshaledLabel.Default)
		}
	}

	// Check assignee
	if issue.Assignee == nil && unmarshaledIssue.Assignee != nil {
		t.Errorf("Assignee mismatch: expected nil, got %+v", unmarshaledIssue.Assignee)
	} else if issue.Assignee != nil && unmarshaledIssue.Assignee == nil {
		t.Errorf("Assignee mismatch: expected %+v, got nil", issue.Assignee)
	} else if issue.Assignee != nil && unmarshaledIssue.Assignee != nil {
		if issue.Assignee.Login != unmarshaledIssue.Assignee.Login {
			t.Errorf("Assignee.Login mismatch: expected %s, got %s", issue.Assignee.Login, unmarshaledIssue.Assignee.Login)
		}
		if issue.Assignee.ID != unmarshaledIssue.Assignee.ID {
			t.Errorf("Assignee.ID mismatch: expected %d, got %d", issue.Assignee.ID, unmarshaledIssue.Assignee.ID)
		}
	}

	// Check assignees
	if len(issue.Assignees) != len(unmarshaledIssue.Assignees) {
		t.Fatalf("Assignees count mismatch: expected %d, got %d", len(issue.Assignees), len(unmarshaledIssue.Assignees))
	}

	// Check comments
	if len(issue.Comments) != len(unmarshaledIssue.Comments) {
		t.Fatalf("Comments count mismatch: expected %d, got %d", len(issue.Comments), len(unmarshaledIssue.Comments))
	}

	// Check project
	if issue.Project == nil && unmarshaledIssue.Project != nil {
		t.Errorf("Project mismatch: expected nil, got %+v", unmarshaledIssue.Project)
	} else if issue.Project != nil && unmarshaledIssue.Project == nil {
		t.Errorf("Project mismatch: expected %+v, got nil", issue.Project)
	} else if issue.Project != nil && unmarshaledIssue.Project != nil {
		if issue.Project.ID != unmarshaledIssue.Project.ID {
			t.Errorf("Project.ID mismatch: expected %s, got %s", issue.Project.ID, unmarshaledIssue.Project.ID)
		}
		if issue.Project.Title != unmarshaledIssue.Project.Title {
			t.Errorf("Project.Title mismatch: expected %s, got %s", issue.Project.Title, unmarshaledIssue.Project.Title)
		}
		if issue.Project.URL != unmarshaledIssue.Project.URL {
			t.Errorf("Project.URL mismatch: expected %s, got %s", issue.Project.URL, unmarshaledIssue.Project.URL)
		}
	}

	// Check fields
	if len(issue.Fields) != len(unmarshaledIssue.Fields) {
		t.Fatalf("Fields count mismatch: expected %d, got %d", len(issue.Fields), len(unmarshaledIssue.Fields))
	}
}
