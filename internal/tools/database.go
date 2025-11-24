package tools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

// DatabaseTool provides database query capabilities for agents
type DatabaseTool struct {
	name string
	desc string
}

// NewDatabaseTool creates a new database tool
func NewDatabaseTool() *DatabaseTool {
	return &DatabaseTool{
		name: "database",
		desc: "Execute SQL queries on connected databases",
	}
}

// Name returns the tool name
func (d *DatabaseTool) Name() string {
	return d.name
}

// Description returns the tool description
func (d *DatabaseTool) Description() string {
	return d.desc
}

// IsLongRunning indicates if this is a long-running operation
func (d *DatabaseTool) IsLongRunning() bool {
	return false
}

// Execute executes the database tool with the given parameters
func (d *DatabaseTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	query, ok := params["query"].(string)
	if !ok {
		return nil, fmt.Errorf("query parameter is required")
	}

	// Get connection string from params or use default
	connectionString, ok := params["connection_string"].(string)
	if !ok {
		return nil, fmt.Errorf("connection_string parameter is required")
	}

	// Only allow SELECT queries for safety
	trimmedQuery := strings.TrimSpace(strings.ToUpper(query))
	if !strings.HasPrefix(trimmedQuery, "SELECT") {
		return nil, fmt.Errorf("only SELECT queries are allowed for security reasons")
	}

	// Connect to database
	db, err := sql.Open("postgres", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	// Execute query
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get column names: %w", err)
	}

	// Prepare result slice
	var results []map[string]interface{}

	// Scan rows
	for rows.Next() {
		// Create a slice of interface{} to hold the values
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		// Scan the row
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Create a map for this row
		rowMap := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]

			// Handle different types
			switch v := val.(type) {
			case []byte:
				// Try to unmarshal as JSON, otherwise convert to string
				var jsonData interface{}
				if err := json.Unmarshal(v, &jsonData); err == nil {
					rowMap[col] = jsonData
				} else {
					rowMap[col] = string(v)
				}
			default:
				rowMap[col] = v
			}
		}

		results = append(results, rowMap)
	}

	// Check for errors during iteration
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return map[string]interface{}{
		"query":   query,
		"columns": columns,
		"rows":    results,
		"count":   len(results),
	}, nil
}

// GetSchema returns the JSON schema for this tool
func (d *DatabaseTool) GetSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"connection_string": map[string]interface{}{
				"type":        "string",
				"description": "PostgreSQL connection string (e.g., postgres://user:pass@host:5432/dbname?sslmode=disable)",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "SQL SELECT query to execute",
			},
		},
		"required": []string{"connection_string", "query"},
	}
}

// ToTool converts this to an ADK tool
func (d *DatabaseTool) ToTool() tool.Tool {
	return &databaseToolAdapter{tool: d}
}

// databaseToolAdapter adapts DatabaseTool to the ADK tool interface
type databaseToolAdapter struct {
	tool *DatabaseTool
}

func (a *databaseToolAdapter) Name() string {
	return a.tool.Name()
}

func (a *databaseToolAdapter) Description() string {
	return a.tool.Description()
}

func (a *databaseToolAdapter) IsLongRunning() bool {
	return a.tool.IsLongRunning()
}

func (a *databaseToolAdapter) GetTool() interface{} {
	return a.tool
}

// Declaration returns the function declaration for this tool
func (a *databaseToolAdapter) Declaration() *genai.FunctionDeclaration {
	schema := a.tool.GetSchema()
	paramsJSON, _ := json.Marshal(schema)

	return &genai.FunctionDeclaration{
		Name:                 a.tool.Name(),
		Description:          a.tool.Description(),
		ParametersJsonSchema: string(paramsJSON),
	}
}

// Run executes the tool with the provided context and arguments
func (a *databaseToolAdapter) Run(ctx tool.Context, args any) (map[string]any, error) {
	// Convert args to map[string]interface{}
	argsMap, ok := args.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected map[string]any, got %T", args)
	}

	result, err := a.tool.Execute(context.Background(), argsMap)
	if err != nil {
		return nil, err
	}

	// Convert result to map[string]any
	resultMap, ok := result.(map[string]any)
	if !ok {
		return map[string]any{"result": result}, nil
	}

	return resultMap, nil
}
