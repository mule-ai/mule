//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"unsafe"
)

// Input represents the expected input structure for GitHub issues
type Input struct {
	RepoURL string `json:"repo_url"`
	Token   string `json:"token"`
	Filters FilterConfig `json:"filters,omitempty"`
}

// FilterConfig represents the filter configuration for GitHub issues
type FilterConfig struct {
	State         string `json:"state,omitempty"`         // open, closed, all
	Assignee      string `json:"assignee,omitempty"`      // username, "none", "*", or "@me" for token owner
	Labels        string `json:"labels,omitempty"`        // comma-separated label names
	Sort          string `json:"sort,omitempty"`          // created, updated, comments
	Direction     string `json:"direction,omitempty"`     // asc, desc
	PerPage       int    `json:"per_page,omitempty"`      // results per page (max 100)
	Page          int    `json:"page,omitempty"`          // page number
	FetchComments bool   `json:"fetch_comments,omitempty"` // whether to fetch comments for issues
}

// GitHubIssue represents an extended GitHub issue structure with project information
type GitHubIssue struct {
	ID             int            `json:"id"`
	Number         int            `json:"number"`
	Title          string         `json:"title"`
	State          string         `json:"state"`
	URL            string         `json:"url"`
	Body           string         `json:"body"`
	CommentsCount  int            `json:"comments"`
	Assignee       *GitHubUser    `json:"assignee,omitempty"`
	Assignees      []GitHubUser   `json:"assignees,omitempty"`
	Comments       []GitHubComment `json:"comments_data,omitempty"`
	Project        *GitHubProject `json:"project,omitempty"`
	Fields         []GitHubField  `json:"fields,omitempty"`
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

// Output represents the output structure
type Output struct {
	Issues []GitHubIssue `json:"issues"`
	Count  int           `json:"count"`
}

// http_request_with_headers is the enhanced host function for making HTTP requests with headers
// It's imported from the host environment
//
//go:wasmimport env http_request_with_headers
func http_request_with_headers(methodPtr, methodSize, urlPtr, urlSize, bodyPtr, bodySize, headersPtr, headersSize uintptr) uintptr

// get_last_response_body gets the last response body
//go:wasmimport env get_last_response_body
func get_last_response_body(bufferPtr, bufferSize uintptr) uint32

// get_last_response_status gets the last response status code
//go:wasmimport env get_last_response_status
func get_last_response_status() uint32

func main() {
	// Read input from stdin
	var input Input
	decoder := json.NewDecoder(os.Stdin)
	if err := decoder.Decode(&input); err != nil {
		outputError(fmt.Errorf("failed to decode input: %w", err))
		return
	}

	// Validate input
	if input.RepoURL == "" {
		outputError(fmt.Errorf("repo_url is required"))
		return
	}

	if input.Token == "" {
		outputError(fmt.Errorf("token is required"))
		return
	}

	// Extract owner and repo from URL
	owner, repo, err := parseGitHubURL(input.RepoURL)
	if err != nil {
		outputError(fmt.Errorf("invalid repo_url: %w", err))
		return
	}

	// First, fetch basic issues using REST API with filters
	issues, err := fetchBasicIssues(owner, repo, input.Token, input.Filters)
	if err != nil {
		outputError(fmt.Errorf("failed to fetch issues: %w", err))
		return
	}

	// Debug: Print basic issues count
	fmt.Fprintf(os.Stderr, "Fetched %d basic issues\n", len(issues))

	// If comments are requested, fetch them for each issue
	if shouldFetchComments(input.Filters) {
		issues, err = fetchCommentsForIssues(issues, owner, repo, input.Token)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to fetch comments: %v\n", err)
		}
	}

	// Then, enrich issues with project data using GraphQL API
	enrichedIssues, err := enrichIssuesWithProjectData(issues, owner, repo, input.Token)
	if err != nil {
		// If enrichment fails, return basic issues
		fmt.Fprintf(os.Stderr, "Warning: failed to enrich issues with project data: %v\n", err)
	} else {
		issues = enrichedIssues
	}

	// Create output
	output := Output{
		Issues: issues,
		Count:  len(issues),
	}

	// Serialize output to JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(output); err != nil {
		outputError(fmt.Errorf("failed to encode output: %w", err))
		return
	}
}

// fetchBasicIssues fetches basic issue information using the GitHub REST API
func fetchBasicIssues(owner, repo, token string, filters FilterConfig) ([]GitHubIssue, error) {
	// Construct GitHub API URL with query parameters
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", owner, repo)

	// Add query parameters based on filters
	params := []string{}
	if filters.State != "" {
		params = append(params, "state="+filters.State)
	}
	if filters.Assignee != "" {
		// Special handling for "@me" to use the authenticated user
		assignee := filters.Assignee
		if assignee == "@me" {
			// Get the authenticated user's login
			login, err := getAuthenticatedUserLogin(token)
			if err != nil {
				return nil, fmt.Errorf("failed to get authenticated user: %w", err)
			}
			assignee = login
		}
		params = append(params, "assignee="+assignee)
	}
	if filters.Labels != "" {
		params = append(params, "labels="+filters.Labels)
	}
	if filters.Sort != "" {
		params = append(params, "sort="+filters.Sort)
	}
	if filters.Direction != "" {
		params = append(params, "direction="+filters.Direction)
	}
	if filters.PerPage > 0 {
		perPage := filters.PerPage
		if perPage > 100 {
			perPage = 100 // Max allowed by GitHub API
		}
		params = append(params, fmt.Sprintf("per_page=%d", perPage))
	}
	if filters.Page > 0 {
		params = append(params, fmt.Sprintf("page=%d", filters.Page))
	}

	if len(params) > 0 {
		apiURL += "?" + joinParams(params, "&")
	}

	// Prepare headers
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token),
		"Accept":        "application/vnd.github.v3+json",
		"User-Agent":    "Mule-AI-WASM-Module",
	}

	// Convert method to bytes
	method := "GET"
	methodBytes := []byte(method)
	methodPtr := uintptr(unsafe.Pointer(&methodBytes[0]))
	methodSize := uintptr(len(methodBytes))

	// Convert URL to bytes
	urlBytes := []byte(apiURL)
	urlPtr := uintptr(unsafe.Pointer(&urlBytes[0]))
	urlSize := uintptr(len(urlBytes))

	// No body for GET request
	var bodyPtr, bodySize uintptr = 0, 0

	// Convert headers to JSON
	headersBytes, err := json.Marshal(headers)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal headers: %w", err)
	}
	headersPtr := uintptr(unsafe.Pointer(&headersBytes[0]))
	headersSize := uintptr(len(headersBytes))

	// Make HTTP request using the enhanced host function
	errorCode := http_request_with_headers(
		methodPtr, methodSize,
		urlPtr, urlSize,
		bodyPtr, bodySize,
		headersPtr, headersSize)

	// Check for errors
	if errorCode != 0 {
		errorMsg := getErrorMessage(errorCode)
		return nil, fmt.Errorf("HTTP request failed: %s", errorMsg)
	}

	// Get response status
	statusCode := int(get_last_response_status())
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("GitHub API request failed with status: %d", statusCode)
	}

	// Get response body
	// Allocate a buffer for the response body (500KB max)
	buffer := make([]byte, 512000)
	bufferPtr := uintptr(unsafe.Pointer(&buffer[0]))
	bufferSize := uintptr(len(buffer))

	bodySizeRet := get_last_response_body(bufferPtr, bufferSize)
	if bodySizeRet == 0 {
		return nil, fmt.Errorf("empty response from GitHub API")
	}

	if bodySizeRet > uint32(len(buffer)) {
		return nil, fmt.Errorf("response too large")
	}

	// Parse response body as JSON
	responseBody := string(buffer[:bodySizeRet])
	var issues []GitHubIssue
	if err := json.Unmarshal([]byte(responseBody), &issues); err != nil {
		// Try to parse as error response
		var errorResp map[string]interface{}
		if err2 := json.Unmarshal([]byte(responseBody), &errorResp); err2 == nil {
			if message, ok := errorResp["message"].(string); ok {
				return nil, fmt.Errorf("GitHub API error: %s", message)
			}
		}
		return nil, fmt.Errorf("failed to parse GitHub API response: %w", err)
	}

	return issues, nil
}

// enrichIssuesWithProjectData enriches issues with project and field information using GraphQL
func enrichIssuesWithProjectData(issues []GitHubIssue, owner, repo, token string) ([]GitHubIssue, error) {
	// Prepare a GraphQL query to fetch project data for all issues
	// Using a more comprehensive query that includes both open and closed issues
	query := `
query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    issues(first: 100, states: [OPEN, CLOSED]) {
      nodes {
        id
        number
        title
        assignees(first: 10) {
          nodes {
            login
            id
            avatarUrl
            url
          }
        }
        projectItems(first: 10) {
          nodes {
            id
            project {
              id
              title
              url
            }
            fieldValues(first: 20) {
              nodes {
                ... on ProjectV2ItemFieldTextValue {
                  text
                  field {
                    ... on ProjectV2FieldCommon {
                      id
                      name
                    }
                  }
                }
                ... on ProjectV2ItemFieldNumberValue {
                  number
                  field {
                    ... on ProjectV2FieldCommon {
                      id
                      name
                    }
                  }
                }
                ... on ProjectV2ItemFieldDateValue {
                  date
                  field {
                    ... on ProjectV2FieldCommon {
                      id
                      name
                    }
                  }
                }
                ... on ProjectV2ItemFieldSingleSelectValue {
                  name
                  field {
                    ... on ProjectV2FieldCommon {
                      id
                      name
                    }
                  }
                }
                ... on ProjectV2ItemFieldIterationValue {
                  title
                  field {
                    ... on ProjectV2FieldCommon {
                      id
                      name
                    }
                  }
                }
              }
            }
          }
        }
      }
    }
  }
}`

	// Prepare variables
	variables := map[string]interface{}{
		"owner": owner,
		"name":  repo,
	}

	// Prepare request body
	requestBody := map[string]interface{}{
		"query":     query,
		"variables": variables,
	}
	
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal GraphQL request: %w", err)
	}

	// Prepare headers
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token),
		"Content-Type":  "application/json",
		"User-Agent":    "Mule-AI-WASM-Module",
	}

	// Convert method to bytes
	method := "POST"
	methodBytes := []byte(method)
	methodPtr := uintptr(unsafe.Pointer(&methodBytes[0]))
	methodSize := uintptr(len(methodBytes))

	// Convert URL to bytes (GraphQL endpoint)
	urlBytes := []byte("https://api.github.com/graphql")
	urlPtr := uintptr(unsafe.Pointer(&urlBytes[0]))
	urlSize := uintptr(len(urlBytes))

	// Convert body to bytes
	bodyPtr := uintptr(unsafe.Pointer(&bodyBytes[0]))
	bodySize := uintptr(len(bodyBytes))

	// Convert headers to JSON
	headersBytes, err := json.Marshal(headers)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal headers: %w", err)
	}
	headersPtr := uintptr(unsafe.Pointer(&headersBytes[0]))
	headersSize := uintptr(len(headersBytes))

	// Make HTTP request using the enhanced host function
	errorCode := http_request_with_headers(
		methodPtr, methodSize,
		urlPtr, urlSize,
		bodyPtr, bodySize,
		headersPtr, headersSize)

	// Check for errors
	if errorCode != 0 {
		errorMsg := getErrorMessage(errorCode)
		return nil, fmt.Errorf("GraphQL HTTP request failed: %s", errorMsg)
	}

	// Get response status
	statusCode := int(get_last_response_status())
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("GitHub GraphQL API request failed with status: %d", statusCode)
	}

	// Get response body
	// Allocate a buffer for the response body (1MB max)
	buffer := make([]byte, 1048576)
	bufferPtr := uintptr(unsafe.Pointer(&buffer[0]))
	bufferSize := uintptr(len(buffer))

	bodySizeRet := get_last_response_body(bufferPtr, bufferSize)
	if bodySizeRet == 0 {
		return nil, fmt.Errorf("empty response from GitHub GraphQL API")
	}

	if bodySizeRet > uint32(len(buffer)) {
		return nil, fmt.Errorf("response too large")
	}

	// Parse response body as JSON
	responseBody := string(buffer[:bodySizeRet])
	
	// Debug: Print raw response for troubleshooting
	fmt.Fprintf(os.Stderr, "GraphQL Response Length: %d\n", len(responseBody))
	
	// Parse GraphQL response
	var graphqlResponse map[string]interface{}
	if err := json.Unmarshal([]byte(responseBody), &graphqlResponse); err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL response: %w", err)
	}

	// Check for GraphQL errors
	if errors, ok := graphqlResponse["errors"]; ok {
		// Print errors to stderr for debugging
		errorsJSON, _ := json.Marshal(errors)
		fmt.Fprintf(os.Stderr, "GraphQL Errors: %s\n", string(errorsJSON))
		return nil, fmt.Errorf("GraphQL errors: %v", errors)
	}

	// Extract data from response
	data, ok := graphqlResponse["data"].(map[string]interface{})
	if !ok {
		// Debug the response structure
		responseKeys := make([]string, 0)
		for k := range graphqlResponse {
			responseKeys = append(responseKeys, k)
		}
		fmt.Fprintf(os.Stderr, "GraphQL response keys: %v\n", responseKeys)
		return nil, fmt.Errorf("unexpected GraphQL response structure")
	}

	repository, ok := data["repository"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected repository structure in GraphQL response")
	}

	issuesData, ok := repository["issues"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected issues structure in GraphQL response")
	}

	nodes, ok := issuesData["nodes"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected nodes structure in GraphQL response")
	}

	// Debug: Print number of nodes
	fmt.Fprintf(os.Stderr, "Found %d issue nodes in GraphQL response\n", len(nodes))

	// Create a map of issue numbers to project data for quick lookup
	projectDataMap := make(map[int]map[string]interface{})
	for _, node := range nodes {
		issueNode, ok := node.(map[string]interface{})
		if !ok {
			continue
		}

		number, ok := issueNode["number"].(float64)
		if !ok {
			continue
		}

		projectDataMap[int(number)] = issueNode
	}

	// Debug: Print project data map size
	fmt.Fprintf(os.Stderr, "Mapped project data for %d issues\n", len(projectDataMap))

	// Enrich issues with project data
	enrichedIssues := make([]GitHubIssue, len(issues))
	for i, issue := range issues {
		enrichedIssues[i] = issue

		// Look up project data for this issue
		if projectData, exists := projectDataMap[issue.Number]; exists {
			// Debug: Found matching issue
			fmt.Fprintf(os.Stderr, "Found project data for issue #%d (%s)\n", issue.Number, issue.Title)

			// Extract assignees
			if assigneesData, ok := projectData["assignees"].(map[string]interface{}); ok {
				if assigneeNodes, ok := assigneesData["nodes"].([]interface{}); ok {
					assignees := make([]GitHubUser, 0, len(assigneeNodes))
					for _, assigneeNode := range assigneeNodes {
						if assigneeData, ok := assigneeNode.(map[string]interface{}); ok {
							user := GitHubUser{
								Login:     getStringValue(assigneeData, "login"),
								ID:        getIntValue(assigneeData, "id"),
								AvatarURL: getStringValue(assigneeData, "avatarUrl"),
								URL:       getStringValue(assigneeData, "url"),
							}
							if user.Login != "" {
								assignees = append(assignees, user)
							}
						}
					}
					enrichedIssues[i].Assignees = assignees
					// Set primary assignee if available
					if len(assignees) > 0 {
						enrichedIssues[i].Assignee = &assignees[0]
					}
				}
			}

			// Extract project items
			if projectItems, ok := projectData["projectItems"].(map[string]interface{}); ok {
				if nodes, ok := projectItems["nodes"].([]interface{}); ok && len(nodes) > 0 {
					// Debug: Found project items
					fmt.Fprintf(os.Stderr, "Found %d project items for issue #%d\n", len(nodes), issue.Number)

					// Take the first project item (assuming one project per issue for simplicity)
					if firstItem, ok := nodes[0].(map[string]interface{}); ok {
						// Extract project information
						if project, ok := firstItem["project"].(map[string]interface{}); ok {
							projectId := getStringValue(project, "id")
							projectTitle := getStringValue(project, "title")
							projectUrl := getStringValue(project, "url")
							
							// Debug: Print project info
							fmt.Fprintf(os.Stderr, "Project Info - ID: %s, Title: %s, URL: %s\n", projectId, projectTitle, projectUrl)
							
							if projectId != "" {
								enrichedIssues[i].Project = &GitHubProject{
									ID:    projectId,
									Title: projectTitle,
									URL:   projectUrl,
								}
							}
						}

						// Extract field values
						if fieldValues, ok := firstItem["fieldValues"].(map[string]interface{}); ok {
							if fieldNodes, ok := fieldValues["nodes"].([]interface{}); ok {
								fields := make([]GitHubField, 0, len(fieldNodes))
								for _, fieldNode := range fieldNodes {
									if fieldData, ok := fieldNode.(map[string]interface{}); ok {
										field := GitHubField{}
										
										// Extract field name
										if fieldInfo, ok := fieldData["field"].(map[string]interface{}); ok {
											field.Name = getStringValue(fieldInfo, "name")
										}
										
										// Extract value based on type
										if text, exists := fieldData["text"]; exists {
											field.Type = "TEXT"
											field.Value = text
										} else if number, exists := fieldData["number"]; exists {
											field.Type = "NUMBER"
											field.Value = number
										} else if date, exists := fieldData["date"]; exists {
											field.Type = "DATE"
											field.Value = date
										} else if name, exists := fieldData["name"]; exists {
											// This could be single select or iteration
											field.Type = "TEXT"
											field.Value = name
										}
										
										if field.Name != "" {
											fields = append(fields, field)
											// Debug: Print field info
											fmt.Fprintf(os.Stderr, "Field - Name: %s, Type: %s, Value: %v\n", field.Name, field.Type, field.Value)
										}
									}
								}
								enrichedIssues[i].Fields = fields
							}
						}
					}
				}
			}
		}
	}

	return enrichedIssues, nil
}

// getStringValue safely extracts a string value from a map
func getStringValue(m map[string]interface{}, key string) string {
	if val, ok := m[key].(string); ok {
		return val
	}
	return ""
}

// getIntValue safely extracts an integer value from a map
func getIntValue(m map[string]interface{}, key string) int {
	if val, ok := m[key].(float64); ok {
		return int(val)
	}
	return 0
}

// parseGitHubURL extracts owner and repo from a GitHub URL
func parseGitHubURL(url string) (owner, repo string, err error) {
	// Handle different GitHub URL formats
	// https://github.com/owner/repo
	// https://github.com/owner/repo/

	// Remove trailing slash if present
	if len(url) > 0 && url[len(url)-1] == '/' {
		url = url[:len(url)-1]
	}

	// Check if it's a GitHub URL
	const prefix = "https://github.com/"
	if !startsWith(url, prefix) {
		return "", "", fmt.Errorf("not a valid GitHub URL")
	}

	// Extract owner/repo part
	path := url[len(prefix):]

	// Split by slash
	parts := split(path, "/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid GitHub URL format")
	}

	return parts[0], parts[1], nil
}

// startsWith checks if a string starts with a prefix
func startsWith(s, prefix string) bool {
	if len(prefix) > len(s) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if s[i] != prefix[i] {
			return false
		}
	}
	return true
}

// split splits a string by a delimiter (simple implementation since we can't use strings.Split)
func split(s, delim string) []string {
	if delim == "" {
		result := make([]string, len(s))
		for i, c := range []byte(s) {
			result[i] = string(c)
		}
		return result
	}

	var result []string
	start := 0

	for i := 0; i <= len(s)-len(delim); i++ {
		match := true
		for j := 0; j < len(delim); j++ {
			if s[i+j] != delim[j] {
				match = false
				break
			}
		}

		if match {
			result = append(result, s[start:i])
			start = i + len(delim)
			i = start - 1 // Adjust for loop increment
		}
	}

	// Add the remaining part
	result = append(result, s[start:])
	return result
}

// getErrorMessage returns a human-readable error message for the given error code
func getErrorMessage(errorCode uintptr) string {
	switch errorCode {
	case 0x00000000:
		return "success"
	case 0xFFFFFFFF:
		return "failed to read URL from memory"
	case 0xFFFFFFFE:
		return "URL not allowed"
	case 0xFFFFFFFD:
		return "failed to create HTTP request"
	case 0xFFFFFFFC:
		return "failed to make HTTP request"
	case 0xFFFFFFFB:
		return "failed to read response body"
	case 0xFFFFFFF0:
		return "failed to read HTTP method from memory"
	case 0xFFFFFFF1:
		return "failed to read HTTP body from memory"
	case 0xFFFFFFF2:
		return "failed to read HTTP headers from memory"
	case 0xFFFFFFF3:
		return "failed to parse HTTP headers JSON"
	case 0xFFFFFFF4:
		return "no response available"
	case 0xFFFFFFF5:
		return "buffer too small for response data"
	case 0xFFFFFFF6:
		return "failed to write response data to memory"
	case 0xFFFFFFF7:
		return "failed to read header name from memory"
	default:
		return fmt.Sprintf("unknown error (code: 0x%x)", errorCode)
	}
}

// outputError outputs an error message in the expected format
func outputError(err error) {
	// Simple error output as JSON
	fmt.Printf("{\"error\": \"%s\"}\n", err.Error())
	os.Exit(1)
}

// getAuthenticatedUserLogin gets the login of the authenticated user
func getAuthenticatedUserLogin(token string) (string, error) {
	// Construct GitHub API URL
	apiURL := "https://api.github.com/user"

	// Prepare headers
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token),
		"Accept":        "application/vnd.github.v3+json",
		"User-Agent":    "Mule-AI-WASM-Module",
	}

	// Convert method to bytes
	method := "GET"
	methodBytes := []byte(method)
	methodPtr := uintptr(unsafe.Pointer(&methodBytes[0]))
	methodSize := uintptr(len(methodBytes))

	// Convert URL to bytes
	urlBytes := []byte(apiURL)
	urlPtr := uintptr(unsafe.Pointer(&urlBytes[0]))
	urlSize := uintptr(len(urlBytes))

	// No body for GET request
	var bodyPtr, bodySize uintptr = 0, 0

	// Convert headers to JSON
	headersBytes, err := json.Marshal(headers)
	if err != nil {
		return "", fmt.Errorf("failed to marshal headers: %w", err)
	}
	headersPtr := uintptr(unsafe.Pointer(&headersBytes[0]))
	headersSize := uintptr(len(headersBytes))

	// Make HTTP request using the enhanced host function
	errorCode := http_request_with_headers(
		methodPtr, methodSize,
		urlPtr, urlSize,
		bodyPtr, bodySize,
		headersPtr, headersSize)

	// Check for errors
	if errorCode != 0 {
		errorMsg := getErrorMessage(errorCode)
		return "", fmt.Errorf("HTTP request failed: %s", errorMsg)
	}

	// Get response status
	statusCode := int(get_last_response_status())
	if statusCode < 200 || statusCode >= 300 {
		return "", fmt.Errorf("GitHub API request failed with status: %d", statusCode)
	}

	// Get response body
	// Allocate a buffer for the response body (100KB max)
	buffer := make([]byte, 102400)
	bufferPtr := uintptr(unsafe.Pointer(&buffer[0]))
	bufferSize := uintptr(len(buffer))

	bodySizeRet := get_last_response_body(bufferPtr, bufferSize)
	if bodySizeRet == 0 {
		return "", fmt.Errorf("empty response from GitHub API")
	}

	if bodySizeRet > uint32(len(buffer)) {
		return "", fmt.Errorf("response too large")
	}

	// Parse response body as JSON
	responseBody := string(buffer[:bodySizeRet])
	var user map[string]interface{}
	if err := json.Unmarshal([]byte(responseBody), &user); err != nil {
		// Try to parse as error response
		var errorResp map[string]interface{}
		if err2 := json.Unmarshal([]byte(responseBody), &errorResp); err2 == nil {
			if message, ok := errorResp["message"].(string); ok {
				return "", fmt.Errorf("GitHub API error: %s", message)
			}
		}
		return "", fmt.Errorf("failed to parse GitHub API response: %w", err)
	}

	// Extract login
	if login, ok := user["login"].(string); ok {
		return login, nil
	}

	return "", fmt.Errorf("failed to extract login from user response")
}

// joinParams joins query parameters with a separator
func joinParams(params []string, sep string) string {
	if len(params) == 0 {
		return ""
	}

	result := params[0]
	for i := 1; i < len(params); i++ {
		result += sep + params[i]
	}
	return result
}

// shouldFetchComments determines if comments should be fetched based on filters
func shouldFetchComments(filters FilterConfig) bool {
	// Fetch comments if explicitly requested or if sorting by comments
	return filters.FetchComments || filters.Sort == "comments"
}

// fetchCommentsForIssues fetches comments for all issues
func fetchCommentsForIssues(issues []GitHubIssue, owner, repo, token string) ([]GitHubIssue, error) {
	// Fetch comments for each issue
	for i := range issues {
		comments, err := fetchIssueComments(issues[i].Number, owner, repo, token)
		if err != nil {
			// Log error but continue with other issues
			fmt.Fprintf(os.Stderr, "Failed to fetch comments for issue #%d: %v\n", issues[i].Number, err)
			continue
		}
		issues[i].Comments = comments
	}
	return issues, nil
}

// fetchIssueComments fetches comments for a specific issue
func fetchIssueComments(issueNumber int, owner, repo, token string) ([]GitHubComment, error) {
	// Construct GitHub API URL
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/comments", owner, repo, issueNumber)

	// Prepare headers
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token),
		"Accept":        "application/vnd.github.v3+json",
		"User-Agent":    "Mule-AI-WASM-Module",
	}

	// Convert method to bytes
	method := "GET"
	methodBytes := []byte(method)
	methodPtr := uintptr(unsafe.Pointer(&methodBytes[0]))
	methodSize := uintptr(len(methodBytes))

	// Convert URL to bytes
	urlBytes := []byte(apiURL)
	urlPtr := uintptr(unsafe.Pointer(&urlBytes[0]))
	urlSize := uintptr(len(urlBytes))

	// No body for GET request
	var bodyPtr, bodySize uintptr = 0, 0

	// Convert headers to JSON
	headersBytes, err := json.Marshal(headers)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal headers: %w", err)
	}
	headersPtr := uintptr(unsafe.Pointer(&headersBytes[0]))
	headersSize := uintptr(len(headersBytes))

	// Make HTTP request using the enhanced host function
	errorCode := http_request_with_headers(
		methodPtr, methodSize,
		urlPtr, urlSize,
		bodyPtr, bodySize,
		headersPtr, headersSize)

	// Check for errors
	if errorCode != 0 {
		errorMsg := getErrorMessage(errorCode)
		return nil, fmt.Errorf("HTTP request failed: %s", errorMsg)
	}

	// Get response status
	statusCode := int(get_last_response_status())
	if statusCode < 200 || statusCode >= 300 {
		return nil, fmt.Errorf("GitHub API request failed with status: %d", statusCode)
	}

	// Get response body
	// Allocate a buffer for the response body (500KB max)
	buffer := make([]byte, 512000)
	bufferPtr := uintptr(unsafe.Pointer(&buffer[0]))
	bufferSize := uintptr(len(buffer))

	bodySizeRet := get_last_response_body(bufferPtr, bufferSize)
	if bodySizeRet == 0 {
		return nil, fmt.Errorf("empty response from GitHub API")
	}

	if bodySizeRet > uint32(len(buffer)) {
		return nil, fmt.Errorf("response too large")
	}

	// Parse response body as JSON
	responseBody := string(buffer[:bodySizeRet])
	var comments []GitHubComment
	if err := json.Unmarshal([]byte(responseBody), &comments); err != nil {
		// Try to parse as error response
		var errorResp map[string]interface{}
		if err2 := json.Unmarshal([]byte(responseBody), &errorResp); err2 == nil {
			if message, ok := errorResp["message"].(string); ok {
				return nil, fmt.Errorf("GitHub API error: %s", message)
			}
		}
		return nil, fmt.Errorf("failed to parse GitHub API response: %w", err)
	}

	return comments, nil
}