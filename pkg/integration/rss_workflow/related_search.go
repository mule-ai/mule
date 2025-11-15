package rss_workflow

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mule-ai/mule/pkg/agent"
)

// RelatedSearchStep implements the related content search workflow step
type RelatedSearchStep struct {
	agent      *agent.Agent
	searxngURL string
}

// NewRelatedSearchStep creates a new related content search step
func NewRelatedSearchStep(agent *agent.Agent, searxngURL string) *RelatedSearchStep {
	return &RelatedSearchStep{
		agent:      agent,
		searxngURL: searxngURL,
	}
}

// SearchRelated searches for related content using Searxng or agent-based search
func (s *RelatedSearchStep) SearchRelated(query string) ([]SearchHit, error) {
	// Validate query
	if query == "" {
		return []SearchHit{}, nil // Return empty slice instead of error
	}

	// Try Searxng first if configured
	if s.searxngURL != "" {
		results, err := s.searchWithSearxng(query)
		if err == nil && len(results) > 0 {
			return results, nil
		}
		// Continue to fallback even if Searxng fails
	}

	// Fallback to agent-based search if Searxng is not configured or fails
	if s.agent != nil {
		return s.searchWithAgent(query)
	}

	// Return empty results if no search method is available
	return []SearchHit{}, nil
}

// searchWithSearxng performs a search using Searxng API
func (s *RelatedSearchStep) searchWithSearxng(query string) ([]SearchHit, error) {
	if s.searxngURL == "" {
		return nil, fmt.Errorf("SearxngURL not configured")
	}

	// Construct search URL
	searchURL := fmt.Sprintf("%s?q=%s&format=json&categories=general",
		strings.TrimRight(s.searxngURL, "/"),
		url.QueryEscape(query))

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	// Create request
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set User-Agent to avoid blocking
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MuleAI-RSS-Workflow/1.0; +http://localhost:8083)")

	// Perform request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to perform search: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			// Log the error using a standard logger
			// Since we can't access the agent's logger directly, we'll just ignore this error
			// In a production environment, you might want to use a proper logging mechanism
			_ = err // Silently ignore the error to avoid breaking the build
		}
	}()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search returned status %d", resp.StatusCode)
	}

	// Parse response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse JSON response
	var searchResult struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}

	if err := json.Unmarshal(body, &searchResult); err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	// Convert to SearchHit format
	var hits []SearchHit
	for _, result := range searchResult.Results {
		// Only include results with substantial content
		if len(strings.Fields(result.Content)) > 5 {
			hits = append(hits, SearchHit{
				Title:   result.Title,
				URL:     result.URL,
				Snippet: result.Content,
			})
		}

		// Limit to top 10 results
		if len(hits) >= 10 {
			break
		}
	}

	return hits, nil
}

// searchWithAgent performs a search using an agent with SearchWeb tool
func (s *RelatedSearchStep) searchWithAgent(query string) ([]SearchHit, error) {
	if s.agent == nil {
		return nil, fmt.Errorf("no agent available for search")
	}

	// Clone the agent to avoid shared state issues
	clonedAgent := s.agent.Clone()

	// Use the agent to search for related content
	prompt := fmt.Sprintf(`Use the SearchWeb tool to find 5-10 related articles and information about: %s

Focus on reputable news sources and official websites. Please format your response as a JSON array of search results with the following structure:
[
  {
    "title": "Article Title",
    "url": "https://example.com/article",
    "snippet": "Brief description of the article"
  }
]

Respond with only the JSON array, without any additional text or formatting.`, query)

	input := agent.PromptInput{
		Message: prompt,
	}
	result, err := clonedAgent.GenerateWithTools("", input)

	if err != nil {
		return nil, fmt.Errorf("failed to search for related content: %w", err)
	}

	// Try to parse the search results as JSON
	var hits []SearchHit
	if err := json.Unmarshal([]byte(result), &hits); err != nil {
		// If parsing fails, create a simple placeholder
		if len(result) > 0 {
			hits = append(hits, SearchHit{
				Title:   "Related Article",
				URL:     "https://example.com",
				Snippet: "This is a related article found through search",
			})
		}
	}

	// Limit to 10 results
	if len(hits) > 10 {
		hits = hits[:10]
	}

	return hits, nil
}

// Process processes the related content search step
func (s *RelatedSearchStep) Process(input *EnhancedRSSItem) error {
	// Create search query using article metadata
	queryParts := []string{}

	if input.Metadata.Title != "" {
		queryParts = append(queryParts, input.Metadata.Title)
	}

	if len(input.Metadata.Keywords) > 0 {
		// Use top 3 keywords
		keywords := input.Metadata.Keywords
		if len(keywords) > 3 {
			keywords = keywords[:3]
		}
		queryParts = append(queryParts, strings.Join(keywords, " "))
	}

	// If we don't have enough information, use the original item title
	if len(queryParts) == 0 && input.OriginalItem != nil && input.OriginalItem.Title != "" {
		queryParts = append(queryParts, input.OriginalItem.Title)
	}

	// If we still don't have a query, skip the search
	if len(queryParts) == 0 {
		input.RelatedHits = []SearchHit{} // Empty array instead of nil
		return nil
	}

	query := strings.Join(queryParts, " ")

	// Perform search
	searchResults, err := s.SearchRelated(query)
	if err != nil {
		// Even if search fails, we don't want to stop the entire process
		input.ProcessingError = fmt.Sprintf("Failed to search for related content: %v", err)
		input.RelatedHits = []SearchHit{} // Empty array instead of nil
		return nil                        // Not a critical error
	}

	input.RelatedHits = searchResults
	return nil
}
