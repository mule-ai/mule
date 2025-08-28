# Memory CLI Tool

A standalone command-line interface for managing ChromeM memory store operations.

## Overview

The Memory CLI tool provides direct access to ChromeM-based memory databases used by the Mule AI system. It supports all essential memory operations including listing, adding, querying, and managing memories.

## Installation

Build the CLI tool from the Mule project root:

```bash
go build -o bin/memory-cli ./cmd/memory-cli
```

## Usage

```bash
memory-cli [command] [flags]
```

### Global Flags

- `--db string`: Path to ChromeM database (default: `/tmp/mule_memory.db`)
- `--max int`: Maximum number of messages to store (default: 1000)

## Commands

### List Memories

List all memories from the store with optional filtering:

```bash
# List all memories
memory-cli list

# List with custom database path
memory-cli list --db /path/to/memory.db

# List with limit
memory-cli list --limit 50

# List filtered by integration and channel
memory-cli list --integration cli --channel default

# List filtered by integration only
memory-cli list --integration matrix
```

**Flags:**
- `--limit, -l int`: Maximum number of memories to list (default: 10)
- `--integration string`: Filter by integration ID
- `--channel string`: Filter by channel ID

### Add Memory

Add a new memory entry to the store:

```bash
# Add a basic memory
memory-cli add --content "This is a test message"

# Add with custom metadata
memory-cli add \
  --content "I prefer dark themes" \
  --username "Designer" \
  --integration "cli" \
  --channel "design" \
  --user-id "user123"

# Add a bot message
memory-cli add \
  --content "Python is also a great language" \
  --username "Assistant" \
  --bot \
  --integration "chat"
```

**Flags:**
- `--content, -c string`: Memory content (required)
- `--integration string`: Integration ID (default: "cli")
- `--channel string`: Channel ID (default: "default")
- `--user-id string`: User ID (default: "user")
- `--username string`: Username (default: "User")
- `--bot`: Mark as bot message

### Query Memories

Search for memories using semantic similarity:

```bash
# Basic semantic search
memory-cli query --query "programming languages"

# Search with limit
memory-cli query --query "dark themes" --limit 5

# Search within specific integration/channel
memory-cli query \
  --query "error handling" \
  --integration "matrix" \
  --channel "development"
```

**Flags:**
- `--query, -q string`: Search query (required)
- `--limit, -l int`: Maximum number of results (default: 10)
- `--integration string`: Filter by integration ID
- `--channel string`: Filter by channel ID

### Delete Memory

Attempt to delete a specific memory by ID:

```bash
memory-cli delete [message-id]
```

**Note:** ChromeM doesn't support direct deletion of individual documents. This command will explain the limitation and suggest alternatives.

### Clear Memories

Clear all memories for a specific integration and channel:

```bash
# Clear memories (requires confirmation)
memory-cli clear \
  --integration "cli" \
  --channel "default" \
  --confirm
```

**Flags:**
- `--integration string`: Integration ID (required)
- `--channel string`: Channel ID (required)
- `--confirm`: Confirm the clear operation

**Note:** This operation is permanent and cannot be undone.

### Database Statistics

Show information about the ChromeM database:

```bash
memory-cli stats

# With custom database path
memory-cli stats --db /path/to/memory.db
```

## Examples

### Basic Workflow

```bash
# Add some memories
memory-cli add --content "I love Go programming" --username "Developer"
memory-cli add --content "Dark mode is essential" --username "Designer"
memory-cli add --content "Python is great too" --username "Developer" --bot

# List all memories
memory-cli list

# Search for programming-related memories
memory-cli query --query "programming"

# Get database statistics
memory-cli stats
```

### Working with Different Databases

```bash
# Use production memory database
memory-cli list --db /var/lib/mule/memory.db

# Use development database
memory-cli add --content "Test message" --db /tmp/dev_memory.db

# Query specific database
memory-cli query --query "test" --db /tmp/dev_memory.db
```

### Managing Specific Channels

```bash
# List memories from Matrix integration
memory-cli list --integration matrix --channel "!room:matrix.org"

# Clear all Discord memories
memory-cli clear --integration discord --channel general --confirm

# Add memory to specific channel
memory-cli add \
  --content "Meeting at 3pm" \
  --integration "matrix" \
  --channel "!team:company.com" \
  --username "Manager"
```

## Output Format

### List/Query Results

```
Found 2 memories:

1. [2025-08-27 08:41:19] Developer (Bot) - default
   ID: 20250827084119.434453135
   Integration: cli
   Content: Python is also a great language

2. [2025-08-27 08:41:15] Designer (User) - default
   ID: 20250827084115.602671553
   Integration: cli
   Content: I prefer dark themes
```

### Statistics

```
ChromeM Memory Database Statistics
==================================
Database Path: /tmp/mule_memory.db
Max Messages: 1000
Database Size: 45.23 KB
Last Modified: 2025-08-27 08:41:19
Most Recent Memory: 2025-08-27 08:41:19

Note: ChromeM doesn't provide direct document count functionality.
Use 'list --limit 1000' to get an approximate count.
```

## Troubleshooting

### Common Issues

**"No memories found"**
- Check if the database path is correct
- Verify that memories exist by trying different queries
- Ensure the database file has proper permissions

**"Failed to open store"**
- Check if the database directory exists and is writable
- Verify the database path is accessible
- Ensure no other processes are locking the database

**Query returns unexpected results**
- ChromeM uses semantic similarity, not exact text matching
- Try different query terms or phrases
- Use broader search terms for better coverage

### Performance Notes

- Large databases may take longer to query
- The `list` command uses multiple search queries internally
- Consider using `--limit` to improve performance with large datasets

## Integration with Mule

This CLI tool operates on the same ChromeM databases used by the Mule AI system. Changes made through the CLI will be reflected in the Mule workflows and vice versa.

### Default Database Locations

- Development: `/tmp/mule_memory.db`
- Matrix Integration: `/tmp/mule_memory.db`
- Workflow Memory: `/tmp/mule_workflow_memory.db`

## Technical Details

- **Storage Backend**: ChromeM vector database
- **Embeddings**: Local hash-based embeddings (no API required)
- **Search**: Semantic similarity using vector embeddings
- **Persistence**: SQLite-backed storage
- **Concurrency**: Thread-safe operations with read/write locks