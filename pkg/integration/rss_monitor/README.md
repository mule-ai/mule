# RSS Monitor Integration Example Configuration

This is an example configuration showing how to set up the new RSS monitor integration.

```yaml
rss_monitor:
  news:
    enabled: true
    feedURL: "https://hnrss.org/newest"
    pollInterval: 5
    maxItems: 10
    userAgent: "Mozilla/5.0 (compatible; MuleAI-RSS-Monitor/1.0; +http://localhost:8083)"
    timeout: 30
  tech:
    enabled: true
    feedURL: "https://feeds.arstechnica.com/arstechnica/index"
    pollInterval: 10
    maxItems: 5
    userAgent: "Mozilla/5.0 (compatible; MuleAI-RSS-Monitor/1.0; +http://localhost:8083)"
    timeout: 30
```

This configuration sets up two RSS monitor instances:
1. "news" - monitors Hacker News for new stories every 5 minutes
2. "tech" - monitors Ars Technica for new articles every 10 minutes

Each instance will fire "newItem" events when new items are detected, which can be used to trigger workflows.