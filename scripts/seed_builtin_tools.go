package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	_ "github.com/lib/pq"
)

func main() {
	// Connect to database - use host.docker.internal when running in Docker, otherwise localhost
	connStr := "postgres://mule:mule@host.docker.internal:5432/mulev2?sslmode=disable"
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		// Fallback to localhost if host.docker.internal doesn't work
		connStr = "postgres://mule:mule@localhost:5432/mulev2?sslmode=disable"
		db, err = sql.Open("postgres", connStr)
		if err != nil {
			log.Fatal(err)
		}
	}
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	// Define built-in tools
	tools := []struct {
		name        string
		description string
		metadata    map[string]interface{}
	}{
		{
			name:        "memory",
			description: "In-memory key-value storage for agents",
			metadata: map[string]interface{}{
				"tool_type": "memory",
				"builtin":   true,
			},
		},
		{
			name:        "filesystem",
			description: "Secure filesystem operations (read, write, delete, list, exists)",
			metadata: map[string]interface{}{
				"tool_type": "filesystem",
				"builtin":   true,
			},
		},
		{
			name:        "http",
			description: "Make HTTP requests to external APIs",
			metadata: map[string]interface{}{
				"tool_type": "http",
				"builtin":   true,
			},
		},
		{
			name:        "database",
			description: "Execute SQL queries (SELECT only for security)",
			metadata: map[string]interface{}{
				"tool_type": "database",
				"builtin":   true,
			},
		},
	}

	// Insert tools
	for _, tool := range tools {
		metadataJSON, err := json.Marshal(tool.metadata)
		if err != nil {
			log.Printf("Failed to marshal metadata for %s: %v", tool.name, err)
			continue
		}

		query := `
			INSERT INTO tools (id, name, description, metadata, created_at, updated_at)
			VALUES ($1, $2, $3, $4, NOW(), NOW())
			ON CONFLICT (name) DO UPDATE SET
				description = EXCLUDED.description,
				metadata = EXCLUDED.metadata,
				updated_at = NOW()
		`
		
		_, err = db.ExecContext(ctx, query, tool.name, tool.name, tool.description, metadataJSON)
		if err != nil {
			log.Printf("Failed to insert tool %s: %v", tool.name, err)
		} else {
			fmt.Printf("✓ Tool '%s' seeded\n", tool.name)
		}
	}

	fmt.Println("\nBuilt-in tools seeded successfully!")
}
