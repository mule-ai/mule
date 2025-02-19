package local

import (
	"os"
	"os/exec"
	"reflect"
	"testing"

	"github.com/jbutlerdev/dev-team/pkg/remote/types"
)

func setup(t *testing.T) *Provider {
	// Create a temporary directory for testing
	tmpDir, err := os.MkdirTemp("", "local-provider-test-*")
	if err != nil {
		t.Fatalf("Failed to create temporary directory: %v", err)
	}
	//t.Cleanup(func() { os.RemoveAll(tmpDir) })

	// Initialize a git repository in the temporary directory
	cmd := exec.Command("git", "init", tmpDir)
	err = cmd.Run()
	if err != nil {
		t.Fatalf("Failed to initialize git repository: %v", err)
	}

	// Create base branch (main)
	cmd = exec.Command("git", "-C", tmpDir, "checkout", "-b", "main")
	err = cmd.Run()
	if err != nil {
		t.Fatalf("Failed to create main branch: %v", err)
	}

	// Create a dummy file and commit it to the repository to generate diff
	cmd = exec.Command("touch", tmpDir+"/dummy_file.txt")
	err = cmd.Run()
	if err != nil {
		t.Fatalf("Failed to create dummy file: %v", err)
	}
	cmd = exec.Command("git", "-C", tmpDir, "add", ".")
	err = cmd.Run()
	if err != nil {
		t.Fatalf("Failed to add dummy file to git: %v", err)
	}
	cmd = exec.Command("git", "-C", tmpDir, "commit", "-m", "Initial commit")
	err = cmd.Run()
	if err != nil {
		t.Fatalf("Failed to commit dummy file: %v", err)
	}

	// Create feature branch (feature/test)
	cmd = exec.Command("git", "-C", tmpDir, "checkout", "-b", "feature/test")
	err = cmd.Run()
	if err != nil {
		t.Fatalf("Failed to create feature/test branch: %v", err)
	}

	// Add another dummy file and commit it to the feature branch
	cmd = exec.Command("touch", tmpDir+"/dummy_file2.txt")
	err = cmd.Run()
	if err != nil {
		t.Fatalf("Failed to create dummy file 2: %v", err)
	}
	cmd = exec.Command("git", "-C", tmpDir, "add", ".")
	err = cmd.Run()
	if err != nil {
		t.Fatalf("Failed to add dummy file 2 to git: %v", err)
	}
	cmd = exec.Command("git", "-C", tmpDir, "commit", "-m", "Second commit")
	err = cmd.Run()
	if err != nil {
		t.Fatalf("Failed to commit dummy file 2: %v", err)
	}

	cmd = exec.Command("git", "-C", tmpDir, "checkout", "main")
	err = cmd.Run()
	if err != nil {
		t.Fatalf("Failed to checkout main branch: %v", err)
	}

	// Initialize the local provider with the temporary directory
	provider := &Provider{
		Path:         tmpDir,
		Issues:       make(map[int]*types.Issue),
		PullRequests: make(map[int]*types.PullRequest),
		IssueCounter: 0,
	}
	return provider
}

func TestNewProvider(t *testing.T) {
	provider := setup(t)

	if provider == nil {
		t.Errorf("NewProvider returned nil")
	}
}

func TestCreateIssue(t *testing.T) {
	provider := setup(t)

	issue := types.Issue{
		Title:  "Test Issue",
		Body:   "Test Issue Body",
		State:  "open",
		Labels: []string{"test"},
	}

	num, err := provider.CreateIssue(issue)

	if err != nil {
		t.Fatalf("Error creating issue: %v", err)
	}

	if num != 1 {
		t.Errorf("Expected issue number 1, got %d", num)
	}

	if len(provider.Issues) != 1 {
		t.Errorf("Expected 1 issue, got %d", len(provider.Issues))
	}

	if provider.Issues[num].Title != "Test Issue" {
		t.Errorf("Expected issue title 'Test Issue', got '%s'", provider.Issues[num].Title)
	}
}

func TestFetchIssues(t *testing.T) {
	provider := setup(t)

	issue1 := types.Issue{
		Title:  "Test Issue 1",
		Body:   "Test Issue Body 1",
		State:  "open",
		Labels: []string{"test", "bug"},
	}
	_, _ = provider.CreateIssue(issue1)

	issue2 := types.Issue{
		Title:  "Test Issue 2",
		Body:   "Test Issue Body 2",
		State:  "closed",
		Labels: []string{"test"},
	}
	_, _ = provider.CreateIssue(issue2)

	options := types.IssueFilterOptions{State: "open"}
	issues, err := provider.FetchIssues("", options)
	if err != nil {
		t.Fatalf("Error fetching issues: %v", err)
	}

	if len(issues) != 1 {
		t.Errorf("Expected 1 open issue, got %d", len(issues))
	}

	if issues[0].Title != "Test Issue 1" {
		t.Errorf("Expected issue title 'Test Issue 1', got '%s'", issues[0].Title)
	}

	options = types.IssueFilterOptions{Label: "bug"}
	issues, err = provider.FetchIssues("", options)
	if err != nil {
		t.Fatalf("Error fetching issues: %v", err)
	}

	if len(issues) != 1 {
		t.Errorf("Expected 1 issue with label 'bug', got %d", len(issues))
	}

	if issues[0].Title != "Test Issue 1" {
		t.Errorf("Expected issue title 'Test Issue 1', got '%s'", issues[0].Title)
	}

	options = types.IssueFilterOptions{}
	issues, err = provider.FetchIssues("", options)
	if err != nil {
		t.Fatalf("Error fetching issues: %v", err)
	}

	if len(issues) != 2 {
		t.Errorf("Expected 2 issues, got %d", len(issues))
	}

}

func TestUpdateIssueState(t *testing.T) {
	provider := setup(t)

	issue := types.Issue{
		Title:  "Test Issue",
		Body:   "Test Issue Body",
		State:  "open",
		Labels: []string{"test"},
	}

	num, _ := provider.CreateIssue(issue)

	err := provider.UpdateIssueState(num, "closed")

	if err != nil {
		t.Fatalf("Error updating issue state: %v", err)
	}

	if provider.Issues[num].State != "closed" {
		t.Errorf("Expected issue state 'closed', got '%s'", provider.Issues[num].State)
	}
}

func TestAddLabelToIssue(t *testing.T) {
	provider := setup(t)

	issue := types.Issue{
		Title:  "Test Issue",
		Body:   "Test Issue Body",
		State:  "open",
		Labels: []string{"test"},
	}

	num, _ := provider.CreateIssue(issue)

	err := provider.AddLabelToIssue(num, "urgent")

	if err != nil {
		t.Fatalf("Error adding label to issue: %v", err)
	}

	if !reflect.DeepEqual(provider.Issues[num].Labels, []string{"test", "urgent"}) {
		t.Errorf("Expected labels '[test urgent]', got '%v'", provider.Issues[num].Labels)
	}
}

func TestCreateDraftPR(t *testing.T) {
	provider := setup(t)

	input := types.PullRequestInput{
		Title:       "Test PR",
		Description: "Test PR Body",
		Base:        "main",
		Branch:      "feature/test",
	}

	err := provider.CreateDraftPR("", input)
	if err != nil {
		t.Fatalf("Error creating draft PR: %v", err)
	}

	if len(provider.PullRequests) != 1 {
		t.Errorf("Expected 1 pull request, got %d", len(provider.PullRequests))
	}

	if provider.PullRequests[1].Title != "Test PR" {
		t.Errorf("Expected PR title 'Test PR', got '%s'", provider.PullRequests[1].Title)
	}
}

func TestFetchPullRequests(t *testing.T) {
	provider := setup(t)

	input := types.PullRequestInput{
		Title:       "Test PR",
		Description: "Test PR Body",
		Base:        "main",
		Branch:      "feature/test",
	}

	err := provider.CreateDraftPR("", input)
	if err != nil {
		t.Fatalf("Error creating draft PR: %v", err)
	}

	prs, err := provider.FetchPullRequests("", "")
	if err != nil {
		t.Fatalf("Error fetching pull requests: %v", err)
	}

	if len(prs) != 1 {
		t.Errorf("Expected 1 pull request, got %d", len(prs))
	}

	if prs[0].Title != "Test PR" {
		t.Errorf("Expected PR title 'Test PR', got '%s'", prs[0].Title)
	}
}

func TestUpdatePullRequestState(t *testing.T) {
	provider := setup(t)

	input := types.PullRequestInput{
		Title:       "Test PR",
		Description: "Test PR Body",
		Base:        "main",
		Branch:      "feature/test",
	}

	err := provider.CreateDraftPR("", input)
	if err != nil {
		t.Fatalf("Error creating draft PR: %v", err)
	}

	err = provider.UpdatePullRequestState("", 1, "open")

	if err != nil {
		t.Fatalf("Error updating pull request state: %v", err)
	}

	if provider.PullRequests[1].State != "open" {
		t.Errorf("Expected PR state 'open', got '%s'", provider.PullRequests[1].State)
	}
}

func TestCreateIssueComment(t *testing.T) {
	provider := setup(t)

	issue := types.Issue{
		Title:  "Test Issue",
		Body:   "Test Issue Body",
		State:  "open",
		Labels: []string{"test"},
	}

	num, _ := provider.CreateIssue(issue)

	comment := types.Comment{
		Body: "Test Comment",
	}

	err := provider.CreateIssueComment("", num, comment)

	if err != nil {
		t.Fatalf("Error creating issue comment: %v", err)
	}

	if len(provider.Issues[num].Comments) != 1 {
		t.Errorf("Expected 1 comment, got %d", len(provider.Issues[num].Comments))
	}

	if provider.Issues[num].Comments[0].Body != "Test Comment" {
		t.Errorf("Expected comment body 'Test Comment', got '%s'", provider.Issues[num].Comments[0].Body)
	}
}

func TestCreatePRComment(t *testing.T) {
	provider := setup(t)

	input := types.PullRequestInput{
		Title:       "Test PR",
		Description: "Test PR Body",
		Base:        "main",
		Branch:      "feature/test",
	}

	err := provider.CreateDraftPR("", input)
	if err != nil {
		t.Fatalf("Error creating draft PR: %v", err)
	}

	comment := types.Comment{
		Body: "Test Comment",
	}

	err = provider.CreatePRComment("", 1, comment)

	if err != nil {
		t.Fatalf("Error creating PR comment: %v", err)
	}

	if len(provider.PullRequests[1].Comments) != 1 {
		t.Errorf("Expected 1 comment, got %d", len(provider.PullRequests[1].Comments))
	}

	if provider.PullRequests[1].Comments[0].Body != "Test Comment" {
		t.Errorf("Expected comment body 'Test Comment', got '%s'", provider.PullRequests[1].Comments[0].Body)
	}
}
