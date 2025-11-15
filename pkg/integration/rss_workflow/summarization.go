package rss_workflow

import (
	"fmt"
	"strings"

	"github.com/mule-ai/mule/pkg/agent"
)

// SummarizationStep implements the summarization workflow step
type SummarizationStep struct {
	agent *agent.Agent
}

// NewSummarizationStep creates a new summarization step
func NewSummarizationStep(agent *agent.Agent) *SummarizationStep {
	return &SummarizationStep{
		agent: agent,
	}
}

// Summarize generates an AI summary of content using an agent with RetrievePage tool
func (s *SummarizationStep) Summarize(input *EnhancedRSSItem) (string, error) {
	if s.agent == nil {
		return "", fmt.Errorf("no agent available for summarization")
	}

	// Check if we have content to summarize
	if input.ArticleContent == "" {
		return "", fmt.Errorf("no article content available for summarization")
	}

	// Clone the agent to avoid shared state issues
	clonedAgent := s.agent.Clone()

	// Create a prompt with all the pre-processed information
	relatedContentStr := ""
	if len(input.RelatedHits) > 0 {
		relatedContentStr = "Related Articles:\n"
		for i, hit := range input.RelatedHits {
			if i >= 5 { // Limit to top 5 related articles
				break
			}
			relatedContentStr += fmt.Sprintf("%d. %s (%s)\n", i+1, hit.Title, hit.URL)
		}
	}

	summaryPrompt := fmt.Sprintf(`Please provide a concise, well-written summary of the following article. 
Focus on the key points and main ideas. Keep the summary to 3-5 sentences.

Article Information:
Title: %s
Author: %s
Published Date: %s
Word Count: %d

Article Content:
%s

%s

Please provide a clear, informative summary that captures the essential points of this article. 
Respond with only the summary text, without any additional formatting or explanations.`,
		input.Metadata.Title,
		input.Metadata.Author,
		input.Metadata.PublishDate.Format("2006-01-02"),
		input.Metadata.WordCount,
		input.ArticleContent,
		relatedContentStr)

	inputStruct := agent.PromptInput{
		Message: summaryPrompt,
	}
	summary, err := clonedAgent.GenerateWithTools("", inputStruct)

	if err != nil {
		return "", fmt.Errorf("failed to generate summary: %w", err)
	}

	// If we didn't get a good summary, provide a fallback
	if summary == "" || len(summary) < 10 {
		// Use the first part of the article content as a fallback
		content := strings.TrimSpace(input.ArticleContent)
		if len(content) > 200 {
			// Find a good sentence break
			end := 200
			for i := 199; i > 100; i-- {
				if content[i] == '.' || content[i] == '!' || content[i] == '?' {
					end = i + 1
					break
				}
			}
			summary = content[:end] + "..."
		} else {
			summary = content
		}

		if summary == "" {
			summary = "Summary unavailable - content could not be processed."
		}
	}

	return summary, nil
}

// Process processes the summarization step
func (s *SummarizationStep) Process(input *EnhancedRSSItem) error {
	// If we don't have article content, try to use the original description as a fallback
	if input.ArticleContent == "" && input.OriginalItem != nil {
		input.ArticleContent = input.OriginalItem.Description
	}

	summary, err := s.Summarize(input)
	if err != nil {
		// Even if summarization fails, we still want to provide some content
		if input.OriginalItem != nil && input.OriginalItem.Description != "" {
			input.FinalSummary = input.OriginalItem.Description
			input.ProcessingError = fmt.Sprintf("Failed to generate AI summary: %v. Using original description.", err)
			return nil // Not a critical error
		}
		input.ProcessingError = fmt.Sprintf("Failed to generate summary: %v", err)
		return err
	}

	input.FinalSummary = summary
	return nil
}
