# RSS Workflow Integration

The RSS Workflow Integration provides modular workflow steps for enhancing RSS items with additional content, metadata, and AI-powered summaries.

## Overview

This integration implements the workflow steps described in the RSS Integration Redesign PRD for Phase 2. It provides deterministic and AI-powered steps for enhancing RSS items through Mule's workflow system.

## Features

- **Content Extraction**: Fetches article content from URLs using RetrievePage tool
- **Metadata Extraction**: Extracts author, publish date, keywords from article content
- **Content Cleaning**: Removes ads, scripts, and other unwanted elements from content
- **Related Content Search**: Finds related articles using Searxng or agent-based search
- **AI Summarization**: Generates comprehensive summaries using LLMs
- **Caching**: Caches enhanced content with configurable TTL

## Configuration

The RSS workflow integration can be configured in your Mule configuration file:

```json
{
  "rss_workflow": {
    "enabled": true,
    "cacheTTL": 21600000000000, // 6 hours in nanoseconds
    "searxngURL": "http://localhost:8080",
    "agentID": 1
  }
}
```

## Workflow Steps

The RSS workflow integration provides the following methods that can be called as workflow steps:

1. **extractContent** - Extracts article content from a URL
2. **extractMetadata** - Extracts metadata from article content
3. **cleanContent** - Removes unwanted HTML elements from content
4. **searchRelated** - Searches for related content
5. **summarize** - Generates an AI summary of content
6. **enhanceItem** - Enhances an RSS item with all steps

## Usage in Workflows

Example workflow configuration:

```json
{
  "id": "rss-enhancement-workflow",
  "name": "RSS Enhancement Workflow",
  "steps": [
    {
      "id": "content-extraction",
      "integration": {
        "integration": "rss-workflow",
        "event": "extractContent",
        "data": "{{ .Message }}"
      },
      "outputField": "generatedText"
    },
    {
      "id": "metadata-extraction",
      "integration": {
        "integration": "rss-workflow",
        "event": "extractMetadata",
        "data": "{{ .Message }}"
      },
      "outputField": "generatedText"
    }
  ]
}
```

## Caching

The RSS workflow integration implements caching to avoid redundant processing:

- Enhanced content is cached with a configurable TTL (default: 6 hours)
- Cache is checked before performing any enhancement steps
- Cache can be persisted to disk for persistence across restarts

## Dependencies

- Requires an agent with RetrievePage tool for content extraction
- Optionally uses Searxng for related content search
- Requires LLM provider for AI summarization