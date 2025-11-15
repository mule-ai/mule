package rss_workflow

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/mule-ai/mule/pkg/agent"
	"golang.org/x/net/html"
)

// ContentExtractionStep implements the content extraction workflow step
type ContentExtractionStep struct {
	agent  *agent.Agent
	logger logr.Logger
}

// NewContentExtractionStep creates a new content extraction step
func NewContentExtractionStep(agent *agent.Agent, logger logr.Logger) *ContentExtractionStep {
	return &ContentExtractionStep{
		agent:  agent,
		logger: logger,
	}
}

// ExtractContent extracts article content from a URL using RetrievePage tool or direct HTTP request
func (s *ContentExtractionStep) ExtractContent(url string) (string, error) {
	// First, try to use the agent with RetrievePage tool if available
	if s.agent != nil {
		// Clone the agent to avoid shared state issues
		clonedAgent := s.agent.Clone()

		// Use the agent to fetch the article content
		prompt := fmt.Sprintf(`Use the RetrievePage tool to fetch the content of this article: %s
		
		Please extract only the main article content, excluding navigation, ads, and other non-content elements.
		Return only the article text content without any HTML formatting.`, url)

		input := agent.PromptInput{
			Message: prompt,
		}
		result, err := clonedAgent.GenerateWithTools("", input)

		if err == nil && result != "" {
			// Successfully retrieved content with agent
			return result, nil
		}

		// If agent method fails, fall back to direct HTTP request
	}

	// Fallback to direct HTTP request
	return s.fetchContentDirectly(url)
}

// fetchContentDirectly fetches content using a direct HTTP request
func (s *ContentExtractionStep) fetchContentDirectly(url string) (string, error) {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set User-Agent to avoid blocking
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MuleAI-RSS-Workflow/1.0; +http://localhost:8083)")

	// Perform request
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch content: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			// Log error but don't fail the function
			s.logger.Error(err, "Failed to close response body")
		}
	}()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch returned status %d", resp.StatusCode)
	}

	// Check content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/html") {
		return "", fmt.Errorf("content is not HTML: %s", contentType)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Parse HTML and extract text content
	content, err := s.extractTextFromHTML(string(body))
	if err != nil {
		return "", fmt.Errorf("failed to extract text from HTML: %w", err)
	}

	return content, nil
}

// extractTextFromHTML extracts text content from HTML
func (s *ContentExtractionStep) extractTextFromHTML(htmlContent string) (string, error) {
	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return "", fmt.Errorf("failed to parse HTML: %w", err)
	}

	var text strings.Builder
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.TextNode {
			// Skip script and style content
			parent := n.Parent
			if parent != nil && (parent.Data == "script" || parent.Data == "style") {
				return
			}
			text.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)

	// Clean up the text
	content := strings.TrimSpace(text.String())

	// Remove excessive whitespace
	content = strings.Join(strings.Fields(content), " ")

	return content, nil
}

// Process processes the content extraction step
func (s *ContentExtractionStep) Process(input *EnhancedRSSItem) error {
	if input.OriginalItem == nil || input.OriginalItem.Link == "" {
		return fmt.Errorf("no link available for content extraction")
	}

	content, err := s.ExtractContent(input.OriginalItem.Link)
	if err != nil {
		input.ProcessingError = fmt.Sprintf("Failed to extract content: %v", err)
		return err
	}

	input.ArticleContent = content
	return nil
}
