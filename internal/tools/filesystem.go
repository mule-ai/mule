package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

// FilesystemTool provides filesystem operations for agents
type FilesystemTool struct {
	name string
	desc string
	root string // root directory to restrict access
}

// NewFilesystemTool creates a new filesystem tool
func NewFilesystemTool(rootDir string) *FilesystemTool {
	if rootDir == "" {
		rootDir = "."
	}
	return &FilesystemTool{
		name: "filesystem",
		desc: "Read, write, and manage files in the filesystem",
		root: rootDir,
	}
}

// Name returns the tool name
func (f *FilesystemTool) Name() string {
	return f.name
}

// Description returns the tool description
func (f *FilesystemTool) Description() string {
	return f.desc
}

// IsLongRunning indicates if this is a long-running operation
func (f *FilesystemTool) IsLongRunning() bool {
	return false
}

// Execute executes the filesystem tool with the given parameters
func (f *FilesystemTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	action, ok := params["action"].(string)
	if !ok {
		return nil, fmt.Errorf("action parameter is required")
	}

	switch action {
	case "read":
		path, ok := params["path"].(string)
		if !ok {
			return nil, fmt.Errorf("path parameter is required for read action")
		}
		return f.Read(path)
	case "write":
		path, ok := params["path"].(string)
		if !ok {
			return nil, fmt.Errorf("path parameter is required for write action")
		}
		content, ok := params["content"].(string)
		if !ok {
			return nil, fmt.Errorf("content parameter is required for write action")
		}
		return f.Write(path, content)
	case "delete":
		path, ok := params["path"].(string)
		if !ok {
			return nil, fmt.Errorf("path parameter is required for delete action")
		}
		return f.Delete(path)
	case "list":
		path, ok := params["path"].(string)
		if !ok {
			path = "."
		}
		return f.List(path)
	case "exists":
		path, ok := params["path"].(string)
		if !ok {
			return nil, fmt.Errorf("path parameter is required for exists action")
		}
		return f.Exists(path)
	default:
		return nil, fmt.Errorf("unknown action: %s", action)
	}
}

// Read reads a file from the filesystem
func (f *FilesystemTool) Read(path string) (interface{}, error) {
	// Ensure path is within root directory
	fullPath := filepath.Join(f.root, path)
	if !f.isPathAllowed(fullPath) {
		return nil, fmt.Errorf("access denied: path outside allowed root directory")
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return map[string]interface{}{
		"content": string(content),
		"path":    path,
		"size":    len(content),
	}, nil
}

// Write writes content to a file
func (f *FilesystemTool) Write(path string, content string) (interface{}, error) {
	// Ensure path is within root directory
	fullPath := filepath.Join(f.root, path)
	if !f.isPathAllowed(fullPath) {
		return nil, fmt.Errorf("access denied: path outside allowed root directory")
	}

	// Create directory if it doesn't exist
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
		return nil, fmt.Errorf("failed to write file: %w", err)
	}

	return map[string]interface{}{
		"success": true,
		"path":    path,
		"size":    len(content),
	}, nil
}

// Delete deletes a file
func (f *FilesystemTool) Delete(path string) (interface{}, error) {
	// Ensure path is within root directory
	fullPath := filepath.Join(f.root, path)
	if !f.isPathAllowed(fullPath) {
		return nil, fmt.Errorf("access denied: path outside allowed root directory")
	}

	if err := os.Remove(fullPath); err != nil {
		return nil, fmt.Errorf("failed to delete file: %w", err)
	}

	return map[string]interface{}{
		"success": true,
		"path":    path,
	}, nil
}

// List lists files in a directory
func (f *FilesystemTool) List(path string) (interface{}, error) {
	// Ensure path is within root directory
	fullPath := filepath.Join(f.root, path)
	if !f.isPathAllowed(fullPath) {
		return nil, fmt.Errorf("access denied: path outside allowed root directory")
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to list directory: %w", err)
	}

	files := make([]map[string]interface{}, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		fileInfo := map[string]interface{}{
			"name":     entry.Name(),
			"is_dir":   entry.IsDir(),
			"size":     info.Size(),
			"mod_time": info.ModTime(),
		}
		files = append(files, fileInfo)
	}

	return map[string]interface{}{
		"path":  path,
		"files": files,
		"count": len(files),
	}, nil
}

// Exists checks if a file exists
func (f *FilesystemTool) Exists(path string) (interface{}, error) {
	fullPath := filepath.Join(f.root, path)
	if !f.isPathAllowed(fullPath) {
		return nil, fmt.Errorf("access denied: path outside allowed root directory")
	}

	_, err := os.Stat(fullPath)
	exists := err == nil

	return map[string]interface{}{
		"exists": exists,
		"path":   path,
	}, nil
}

// isPathAllowed checks if a path is within the allowed root directory
func (f *FilesystemTool) isPathAllowed(path string) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	absRoot, err := filepath.Abs(f.root)
	if err != nil {
		return false
	}

	// Check if the path starts with the root directory
	return len(absPath) >= len(absRoot) && absPath[:len(absRoot)] == absRoot
}

// GetSchema returns the JSON schema for this tool
func (f *FilesystemTool) GetSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"action": map[string]interface{}{
				"type":        "string",
				"description": "The action to perform: read, write, delete, list, or exists",
				"enum":        []string{"read", "write", "delete", "list", "exists"},
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The file or directory path",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The content to write (required for write action)",
			},
		},
		"required": []string{"action"},
	}
}

// ToTool converts this to an ADK tool
func (f *FilesystemTool) ToTool() tool.Tool {
	return &filesystemToolAdapter{tool: f}
}

// filesystemToolAdapter adapts FilesystemTool to the ADK tool interface
type filesystemToolAdapter struct {
	tool *FilesystemTool
}

func (a *filesystemToolAdapter) Name() string {
	return a.tool.Name()
}

func (a *filesystemToolAdapter) Description() string {
	return a.tool.Description()
}

func (a *filesystemToolAdapter) IsLongRunning() bool {
	return a.tool.IsLongRunning()
}

func (a *filesystemToolAdapter) GetTool() interface{} {
	return a.tool
}

// Declaration returns the function declaration for this tool
func (a *filesystemToolAdapter) Declaration() *genai.FunctionDeclaration {
	schema := a.tool.GetSchema()
	paramsJSON, _ := json.Marshal(schema)

	return &genai.FunctionDeclaration{
		Name:                 a.tool.Name(),
		Description:          a.tool.Description(),
		ParametersJsonSchema: string(paramsJSON),
	}
}

// Run executes the tool with the provided context and arguments
func (a *filesystemToolAdapter) Run(ctx tool.Context, args any) (map[string]any, error) {
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
