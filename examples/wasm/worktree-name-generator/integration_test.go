package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMainFunction(t *testing.T) {
	// Test with the sample input structure you provided
	inputJSON := `{
		"prompt": "{\"title\":\"Feature: Add MCP client support\"}"
	}`

	// Create a buffer to simulate stdin
	inputBuffer := strings.NewReader(inputJSON)

	// Parse the input
	var input Input
	decoder := json.NewDecoder(inputBuffer)
	if err := decoder.Decode(&input); err != nil {
		t.Fatalf("Failed to decode input: %v", err)
	}

	// Parse the prompt field to extract issue data
	var issueData IssueData
	if err := json.Unmarshal([]byte(input.Prompt), &issueData); err != nil {
		// Try alternative structure
		var promptData PromptData
		if err2 := json.Unmarshal([]byte(input.Prompt), &promptData); err2 != nil {
			t.Fatalf("Failed to decode prompt content: %v", err)
		}
		issueData.Title = promptData.Title
	}

	// Validate input
	if issueData.Title == "" {
		t.Fatal("Issue title is required")
	}

	// Generate worktree name
	worktreeName := generateWorktreeName(issueData.Title)

	// Check the result
	expected := "feature-add-mcp-client-support"
	if worktreeName != expected {
		t.Errorf("Generated worktree name = %q; expected %q", worktreeName, expected)
	}

	// Also test with the more complex input structure you provided
	complexInputJSON := `{
		"prompt": "{\"assignee\":{\"name\":\"mule-bot\",\"url\":\"https://github.com/mule-bot\"},\"body\":\"A user should be able to add mcp servers as tools. There should be support for registering and calling multiple servers.\\n\\nWe should have a frontend for registering these endpoints, they should be written out to the existing config file, and the business logic to call the endpoints should also be written.\\n\\nOnce a mcp server is registered, it should be available as a tool that can be added to an Agent.\\n\\nThere is an existing mcp library that can be used.\\nhttps://github.com/mark3labs/mcp-go\\n\\nHere is some example code.\",\"comments\":[{\"body\":\"ack\",\"created_at\":\"2025-12-03T16:24:43Z\",\"updated_at\":\"2025-12-03T16:24:43Z\",\"user\":\"mule-bot\"},{\"body\":\"The issue describes adding MCP server support as tools but lacks specific details needed for implementation. Here are some unanswered questions:\\n\\n1. What does the frontend interface for registering MCP endpoints look like? Any design mockups or requirements?\\n2. How should the MCP servers be stored in the config file? What is the structure/format expected?\\n3. Should there be any validation or health checks when registering an MCP server?\\n4. How should errors be handled when calling MCP server tools?\\n5. Are there any specific authentication or security requirements for connecting to MCP servers?\\n6. Should the system support dynamic discovery of tools from MCP servers, or is it a one-time registration?\\n7. What are the performance expectations, especially when dealing with multiple MCP servers?\\n8. Are there any specific logging or monitoring requirements for MCP tool calls?\",\"created_at\":\"2025-12-03T17:41:04Z\",\"updated_at\":\"2025-12-03T17:41:04Z\",\"user\":\"mule-bot\"}],\"state\":\"open\",\"title\":\"Feature: Add MCP client support\",\"url\":\"https://api.github.com/repos/mule-ai/mule/issues/7\"}"
	}`

	// Create a buffer to simulate stdin
	complexInputBuffer := strings.NewReader(complexInputJSON)

	// Parse the input
	var complexInput Input
	decoder2 := json.NewDecoder(complexInputBuffer)
	if err := decoder2.Decode(&complexInput); err != nil {
		t.Fatalf("Failed to decode complex input: %v", err)
	}

	// Parse the prompt field to extract issue data
	var complexIssueData IssueData
	if err := json.Unmarshal([]byte(complexInput.Prompt), &complexIssueData); err != nil {
		// Try alternative structure
		var complexPromptData PromptData
		if err2 := json.Unmarshal([]byte(complexInput.Prompt), &complexPromptData); err2 != nil {
			t.Fatalf("Failed to decode complex prompt content: %v", err)
		}
		complexIssueData.Title = complexPromptData.Title
	}

	// Validate input
	if complexIssueData.Title == "" {
		t.Fatal("Issue title is required in complex input")
	}

	// Generate worktree name
	complexWorktreeName := generateWorktreeName(complexIssueData.Title)

	// Check the result
	if complexWorktreeName != expected {
		t.Errorf("Generated worktree name from complex input = %q; expected %q", complexWorktreeName, expected)
	}
}