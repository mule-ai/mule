package rss_workflow

import (
	"regexp"
	"strings"
)

// ContentCleaningStep implements the content cleaning workflow step
type ContentCleaningStep struct{}

// NewContentCleaningStep creates a new content cleaning step
func NewContentCleaningStep() *ContentCleaningStep {
	return &ContentCleaningStep{}
}

// CleanContent removes unwanted HTML elements from content
func (s *ContentCleaningStep) CleanContent(content string) (string, error) {
	if content == "" {
		return "", nil
	}

	// Remove script and style tags
	scriptRegex := regexp.MustCompile(`(?i)<script[^>]*>.*?</script>`)
	content = scriptRegex.ReplaceAllString(content, "")

	styleRegex := regexp.MustCompile(`(?i)<style[^>]*>.*?</style>`)
	content = styleRegex.ReplaceAllString(content, "")

	// Remove navigation and header elements
	navRegex := regexp.MustCompile(`(?i)<nav[^>]*>.*?</nav>`)
	content = navRegex.ReplaceAllString(content, "")

	headerRegex := regexp.MustCompile(`(?i)<header[^>]*>.*?</header>`)
	content = headerRegex.ReplaceAllString(content, "")

	footerRegex := regexp.MustCompile(`(?i)<footer[^>]*>.*?</footer>`)
	content = footerRegex.ReplaceAllString(content, "")

	// Remove advertisements and sidebars
	adRegex := regexp.MustCompile(`(?i)<aside[^>]*>.*?</aside>`)
	content = adRegex.ReplaceAllString(content, "")

	// Remove divs with common ad/sidebar classes
	adClassRegex := regexp.MustCompile(`(?i)<div[^>]*class\s*=\s*["'][^"']*(ad|advertisement|sidebar|menu|navigation)[^"']*[\"'][^>]*>.*?</div>`)
	content = adClassRegex.ReplaceAllString(content, "")

	// Remove common ad containers
	adIdRegex := regexp.MustCompile(`(?i)<div[^>]*id\s*=\s*["'][^"']*(ad|advertisement|sidebar)[^"']*[\"'][^>]*>.*?</div>`)
	content = adIdRegex.ReplaceAllString(content, "")

	// Remove tracking pixels and hidden elements
	trackingRegex := regexp.MustCompile(`(?i)<img[^>]*((width\s*=\s*["']1["'])|(height\s*=\s*["']1["'])|(display\s*:\s*none)).*?>`)
	content = trackingRegex.ReplaceAllString(content, "")

	hiddenRegex := regexp.MustCompile(`(?i)<[^>]*style\s*=\s*["'][^"']*display\s*:\s*none[^"']*[\"'][^>]*>.*?</[^>]+>`)
	content = hiddenRegex.ReplaceAllString(content, "")

	// Remove comments
	commentRegex := regexp.MustCompile(`(?s)<!--.*?-->`)
	content = commentRegex.ReplaceAllString(content, "")

	// Remove multiple whitespace and normalize
	whitespaceRegex := regexp.MustCompile(`\s+`)
	content = whitespaceRegex.ReplaceAllString(content, " ")

	// Trim leading/trailing whitespace
	content = strings.TrimSpace(content)

	return content, nil
}

// Process processes the content cleaning step
func (s *ContentCleaningStep) Process(input *EnhancedRSSItem) error {
	// If we don't have content to clean, that's not necessarily an error
	if input.ArticleContent == "" {
		// Try to use the original description as a fallback
		if input.OriginalItem != nil && input.OriginalItem.Description != "" {
			input.ArticleContent = input.OriginalItem.Description
		} else {
			return nil // Nothing to process
		}
	}

	cleanedContent, err := s.CleanContent(input.ArticleContent)
	if err != nil {
		input.ProcessingError = "Failed to clean content"
		// Even if cleaning fails, we'll keep the original content
		return nil // Not a critical error
	}

	input.ArticleContent = cleanedContent
	return nil
}
