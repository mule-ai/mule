package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

// HTTPTool provides HTTP request capabilities for agents
type HTTPTool struct {
	name       string
	desc       string
	httpClient *http.Client
}

// NewHTTPTool creates a new HTTP tool
func NewHTTPTool() *HTTPTool {
	return &HTTPTool{
		name: "http",
		desc: "Make HTTP requests to external APIs and websites",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name returns the tool name
func (h *HTTPTool) Name() string {
	return h.name
}

// Description returns the tool description
func (h *HTTPTool) Description() string {
	return h.desc
}

// IsLongRunning indicates if this is a long-running operation
func (h *HTTPTool) IsLongRunning() bool {
	return false
}

// Execute executes the HTTP tool with the given parameters
func (h *HTTPTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	method, ok := params["method"].(string)
	if !ok {
		method = "GET"
	}

	urlStr, ok := params["url"].(string)
	if !ok {
		return nil, fmt.Errorf("url parameter is required")
	}

	// Validate URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Only allow http and https schemes
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("only http and https URLs are allowed")
	}

	var body io.Reader
	if params["body"] != nil {
		bodyBytes, err := json.Marshal(params["body"])
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		body = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(ctx, method, urlStr, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	if params["headers"] != nil {
		headers, ok := params["headers"].(map[string]interface{})
		if ok {
			for key, value := range headers {
				if strValue, ok := value.(string); ok {
					req.Header.Set(key, strValue)
				}
			}
		}
	}

	// Set default content type for POST/PUT/PATCH with body
	if body != nil && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Try to parse response as JSON
	var respData interface{}
	if err := json.Unmarshal(respBody, &respData); err != nil {
		// If not JSON, return as string
		respData = string(respBody)
	}

	return map[string]interface{}{
		"status":     resp.StatusCode,
		"statusText": resp.Status,
		"headers":    resp.Header,
		"body":       respData,
	}, nil
}

// GetSchema returns the JSON schema for this tool
func (h *HTTPTool) GetSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"method": map[string]interface{}{
				"type":        "string",
				"description": "HTTP method (GET, POST, PUT, DELETE, PATCH)",
				"enum":        []string{"GET", "POST", "PUT", "DELETE", "PATCH"},
				"default":     "GET",
			},
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL to make the request to",
			},
			"headers": map[string]interface{}{
				"type":        "object",
				"description": "Optional headers to include in the request",
			},
			"body": map[string]interface{}{
				"type":        "string",
				"description": "Optional request body as JSON string (for POST, PUT, PATCH)",
			},
		},
		"required": []string{"url"},
	}
}

// ToTool converts this to an ADK tool
func (h *HTTPTool) ToTool() tool.Tool {
	return &httpToolAdapter{tool: h}
}

// httpToolAdapter adapts HTTPTool to the ADK tool interface
type httpToolAdapter struct {
	tool *HTTPTool
}

func (a *httpToolAdapter) Name() string {
	return a.tool.Name()
}

func (a *httpToolAdapter) Description() string {
	return a.tool.Description()
}

func (a *httpToolAdapter) IsLongRunning() bool {
	return a.tool.IsLongRunning()
}

func (a *httpToolAdapter) GetTool() interface{} {
	return a.tool
}

// Declaration returns the function declaration for this tool
func (a *httpToolAdapter) Declaration() *genai.FunctionDeclaration {
	schema := a.tool.GetSchema()
	paramsJSON, _ := json.Marshal(schema)

	return &genai.FunctionDeclaration{
		Name:        a.tool.Name(),
		Description: a.tool.Description(),
		ParametersJsonSchema: string(paramsJSON),
	}
}

// Run executes the tool with the provided context and arguments
func (a *httpToolAdapter) Run(ctx tool.Context, args any) (map[string]any, error) {
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
