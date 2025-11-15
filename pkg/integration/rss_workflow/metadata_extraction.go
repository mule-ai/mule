package rss_workflow

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// MetadataExtractionStep implements the metadata extraction workflow step
type MetadataExtractionStep struct{}

// NewMetadataExtractionStep creates a new metadata extraction step
func NewMetadataExtractionStep() *MetadataExtractionStep {
	return &MetadataExtractionStep{}
}

// ExtractMetadata extracts metadata from article content
func (s *MetadataExtractionStep) ExtractMetadata(content string) (ArticleMeta, error) {
	meta := ArticleMeta{}

	if content == "" {
		return meta, nil
	}

	// Count words
	words := strings.Fields(content)
	meta.WordCount = len(words)

	// Extract keywords (simple approach - could be enhanced with NLP)
	meta.Keywords = s.extractTopKeywords(content, 10)

	// Try to extract title from <title> tag or <h1> tag
	titleRegex := regexp.MustCompile(`(?i)<title[^>]*>([^<]+)`)
	titleMatches := titleRegex.FindStringSubmatch(content)
	if len(titleMatches) > 1 {
		// Clean up the title by removing extra whitespace and common suffixes
		title := strings.TrimSpace(titleMatches[1])
		// Remove common site name suffixes
		siteSuffixRegex := regexp.MustCompile(`(?i)\s*[|-]\s*[^|]+\s*$`)
		title = siteSuffixRegex.ReplaceAllString(title, "")
		meta.Title = strings.TrimSpace(title)
	} else {
		// Try h1 tag as fallback
		h1Regex := regexp.MustCompile(`(?i)<h1[^>]*>([^<]+)`)
		h1Matches := h1Regex.FindStringSubmatch(content)
		if len(h1Matches) > 1 {
			meta.Title = strings.TrimSpace(h1Matches[1])
		} else {
			// Try og:title meta tag
			ogTitleRegex := regexp.MustCompile(`(?i)<meta[^>]*property\s*=\s*["']og:title["'][^>]*content\s*=\s*["']([^"']+)["']`)
			ogTitleMatches := ogTitleRegex.FindStringSubmatch(content)
			if len(ogTitleMatches) > 1 {
				meta.Title = strings.TrimSpace(ogTitleMatches[1])
			} else {
				meta.Title = "Untitled Article"
			}
		}
	}

	// Try to extract author from common author meta tags or bylines
	authorRegex := regexp.MustCompile(`(?i)<meta[^>]*name\s*=\s*["']author["'][^>]*content\s*=\s*["']([^"']+)["']`)
	authorMatches := authorRegex.FindStringSubmatch(content)
	if len(authorMatches) > 1 {
		meta.Author = strings.TrimSpace(authorMatches[1])
	} else {
		// Try og:author meta tag
		ogAuthorRegex := regexp.MustCompile(`(?i)<meta[^>]*property\s*=\s*["']article:author["'][^>]*content\s*=\s*["']([^"']+)["']`)
		ogAuthorMatches := ogAuthorRegex.FindStringSubmatch(content)
		if len(ogAuthorMatches) > 1 {
			meta.Author = strings.TrimSpace(ogAuthorMatches[1])
		} else {
			// Try byline patterns
			bylineRegex := regexp.MustCompile(`(?i)(?:by|author)[:\s]+([^<>\n\r]{3,50})`)
			bylineMatches := bylineRegex.FindStringSubmatch(content)
			if len(bylineMatches) > 1 {
				meta.Author = strings.TrimSpace(bylineMatches[1])
			} else {
				// Try itemprop author
				itempropRegex := regexp.MustCompile(`(?i)<[^>]*itemprop\s*=\s*["']author["'][^>]*>([^<]+)`)
				itempropMatches := itempropRegex.FindStringSubmatch(content)
				if len(itempropMatches) > 1 {
					meta.Author = strings.TrimSpace(itempropMatches[1])
				} else {
					meta.Author = "Unknown Author"
				}
			}
		}
	}

	// Try to extract publish date from common date meta tags or patterns
	dateRegex := regexp.MustCompile(`(?i)<meta[^>]*name\s*=\s*["'](date|publishdate|publicationdate)["'][^>]*content\s*=\s*["']([^"']+)["']`)
	dateMatches := dateRegex.FindStringSubmatch(content)
	if len(dateMatches) > 2 {
		if parsedTime, err := time.Parse(time.RFC3339, dateMatches[2]); err == nil {
			meta.PublishDate = parsedTime
		} else if parsedTime, err := time.Parse("2006-01-02", dateMatches[2]); err == nil {
			meta.PublishDate = parsedTime
		} else {
			meta.PublishDate = time.Now()
		}
	} else {
		// Try og:article:published_time meta tag
		ogDateRegex := regexp.MustCompile(`(?i)<meta[^>]*property\s*=\s*["']article:published_time["'][^>]*content\s*=\s*["']([^"']+)["']`)
		ogDateMatches := ogDateRegex.FindStringSubmatch(content)
		if len(ogDateMatches) > 1 {
			if parsedTime, err := time.Parse(time.RFC3339, ogDateMatches[1]); err == nil {
				meta.PublishDate = parsedTime
			} else {
				meta.PublishDate = time.Now()
			}
		} else {
			// Try itemprop datePublished
			itempropDateRegex := regexp.MustCompile(`(?i)<[^>]*itemprop\s*=\s*["']datePublished["'][^>]*datetime\s*=\s*["']([^"']+)["']`)
			itempropDateMatches := itempropDateRegex.FindStringSubmatch(content)
			if len(itempropDateMatches) > 1 {
				if parsedTime, err := time.Parse(time.RFC3339, itempropDateMatches[1]); err == nil {
					meta.PublishDate = parsedTime
				} else if parsedTime, err := time.Parse("2006-01-02", itempropDateMatches[1]); err == nil {
					meta.PublishDate = parsedTime
				} else {
					meta.PublishDate = time.Now()
				}
			} else {
				// Try common date patterns in content
				datePatternRegex := regexp.MustCompile(`(?i)(\d{4}[-/]\d{1,2}[-/]\d{1,2}|\d{1,2}[-/]\d{1,2}[-/]\d{4})`)
				datePatternMatches := datePatternRegex.FindStringSubmatch(content)
				if len(datePatternMatches) > 1 {
					if parsedTime, err := time.Parse("2006-01-02", datePatternMatches[1]); err == nil {
						meta.PublishDate = parsedTime
					} else if parsedTime, err := time.Parse("01/02/2006", datePatternMatches[1]); err == nil {
						meta.PublishDate = parsedTime
					} else {
						meta.PublishDate = time.Now()
					}
				} else {
					meta.PublishDate = time.Now()
				}
			}
		}
	}

	return meta, nil
}

// Process processes the metadata extraction step
func (s *MetadataExtractionStep) Process(input *EnhancedRSSItem) error {
	// If we don't have article content, try to use the description from the original item
	if input.ArticleContent == "" {
		if input.OriginalItem != nil {
			input.ArticleContent = input.OriginalItem.Description
		}
	}

	// If we still don't have content, that's not necessarily an error
	if input.ArticleContent == "" {
		// Set some basic metadata from the original item
		if input.OriginalItem != nil {
			input.Metadata = ArticleMeta{
				Title:       input.OriginalItem.Title,
				Author:      input.OriginalItem.Author,
				PublishDate: input.OriginalItem.PublishDate,
				WordCount:   0,
				Keywords:    []string{},
			}
		}
		return nil // Nothing to process
	}

	metadata, err := s.ExtractMetadata(input.ArticleContent)
	if err != nil {
		input.ProcessingError = fmt.Sprintf("Failed to extract metadata: %v", err)
		// Even if metadata extraction fails, we'll continue with basic metadata
		if input.OriginalItem != nil {
			input.Metadata = ArticleMeta{
				Title:       input.OriginalItem.Title,
				Author:      input.OriginalItem.Author,
				PublishDate: input.OriginalItem.PublishDate,
				WordCount:   0,
				Keywords:    []string{},
			}
		}
		return nil // Not a critical error
	}

	input.Metadata = metadata
	return nil
}

// extractTopKeywords extracts the most frequent words from content as keywords
func (s *MetadataExtractionStep) extractTopKeywords(content string, count int) []string {
	if content == "" || count <= 0 {
		return []string{}
	}

	// Convert to lowercase and remove HTML tags
	content = strings.ToLower(content)
	tagRegex := regexp.MustCompile(`<[^>]*>`)
	content = tagRegex.ReplaceAllString(content, " ")

	// Remove punctuation and split into words
	punctRegex := regexp.MustCompile(`[^\w\s]`)
	content = punctRegex.ReplaceAllString(content, " ")
	words := strings.Fields(content)

	// Filter out common stop words
	stopWords := map[string]bool{
		"the": true, "and": true, "or": true, "but": true, "in": true, "on": true, "at": true,
		"to": true, "for": true, "of": true, "with": true, "by": true, "a": true, "an": true,
		"is": true, "are": true, "was": true, "were": true, "be": true, "been": true, "have": true,
		"has": true, "had": true, "do": true, "does": true, "did": true, "will": true, "would": true,
		"could": true, "should": true, "may": true, "might": true, "must": true, "can": true,
		"this": true, "that": true, "these": true, "those": true, "i": true, "you": true, "he": true,
		"she": true, "it": true, "we": true, "they": true, "me": true, "him": true, "her": true,
		"us": true, "them": true, "my": true, "your": true, "his": true, "its": true, "our": true,
		"their": true, "myself": true, "yourself": true, "himself": true, "herself": true,
		"itself": true, "ourselves": true, "yourselves": true, "themselves": true,
	}

	// Count word frequencies
	wordFreq := make(map[string]int)
	for _, word := range words {
		if len(word) > 3 && !stopWords[word] { // Only consider words longer than 3 characters and not stop words
			wordFreq[word]++
		}
	}

	// Convert to slice for sorting
	type wordCount struct {
		word  string
		count int
	}

	wordCounts := make([]wordCount, 0, len(wordFreq))
	for word, count := range wordFreq {
		wordCounts = append(wordCounts, wordCount{word, count})
	}

	// Sort by frequency (descending)
	sort.Slice(wordCounts, func(i, j int) bool {
		return wordCounts[i].count > wordCounts[j].count
	})

	// Take top N keywords
	var keywords []string
	for i := 0; i < len(wordCounts) && i < count; i++ {
		keywords = append(keywords, wordCounts[i].word)
	}

	return keywords
}
