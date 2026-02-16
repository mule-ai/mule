package tools

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	jbutlerdevgenai "github.com/jbutlerdev/genai"
	genaitools "github.com/jbutlerdev/genai/tools"

	"github.com/mule-ai/mule/internal/primitive"
)

// ToolConfigStore defines the interface for fetching tool configurations and providers
type ToolConfigStore interface {
	GetMemoryConfig(ctx context.Context, id string) (*primitive.MemoryConfig, error)
	GetProvider(ctx context.Context, id string) (*primitive.Provider, error)
	ListProviders(ctx context.Context) ([]*primitive.Provider, error)
}

// Registry manages built-in tools and provides them to agents
type Registry struct {
	tools map[string]Tool
	mu    sync.RWMutex
	store ToolConfigStore
}

// Tool defines the interface for built-in tools
type Tool interface {
	Name() string
	Description() string
	IsLongRunning() bool
	Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
	GetSchema() map[string]interface{}
}

// NewRegistry creates a new tool registry with built-in tools (legacy, without config)
func NewRegistry() *Registry {
	registry := &Registry{
		tools: make(map[string]Tool),
	}

	// Register built-in tools - use the old in-memory tool for legacy compatibility
	registry.Register(NewInMemoryTool())
	registry.Register(NewFilesystemTool("."))
	registry.Register(NewHTTPTool())
	registry.Register(NewDatabaseTool())
	registry.Register(NewBashTool())

	return registry
}

// NewRegistryWithConfig creates a new tool registry with configuration support
func NewRegistryWithConfig(store ToolConfigStore) (*Registry, error) {
	registry := &Registry{
		tools: make(map[string]Tool),
		store: store,
	}

	// Initialize memory tool with configuration
	if err := registry.initializeMemoryTool(); err != nil {
		return nil, fmt.Errorf("failed to initialize memory tool: %w", err)
	}

	// Register other built-in tools
	registry.Register(NewFilesystemTool("."))
	registry.Register(NewHTTPTool())
	registry.Register(NewDatabaseTool())
	registry.Register(NewBashTool())

	return registry, nil
}

// initializeMemoryTool initializes the genai memory tool with configuration from the store
func (r *Registry) initializeMemoryTool() error {
	ctx := context.Background()

	// Get memory configuration from store
	primitiveConfig, err := r.store.GetMemoryConfig(ctx, "default")
	if err != nil {
		if err == primitive.ErrNotFound {
			// No memory config exists yet, skip initialization
			// The memory tool can be initialized later when config is saved
			return nil
		}
		// Also handle "table does not exist" errors (migration not run yet)
		if strings.Contains(err.Error(), "does not exist") {
			// Table doesn't exist yet, skip initialization
			// The memory tool can be initialized later when config is saved
			return nil
		}
		return fmt.Errorf("failed to get memory config: %w", err)
	}

	// Convert to genai memory config
	memoryConfig := genaitools.MemoryConfig{
		DatabaseURL:       primitiveConfig.DatabaseURL,
		EmbeddingProvider: primitiveConfig.EmbeddingProvider,
		EmbeddingModel:    primitiveConfig.EmbeddingModel,
		EmbeddingDims:     primitiveConfig.EmbeddingDims,
		DefaultTTL:        time.Duration(primitiveConfig.DefaultTTLSeconds) * time.Second,
		DefaultTopK:       primitiveConfig.DefaultTopK,
	}

	// Create embedding provider based on configuration
	var embeddingProvider genaitools.EmbeddingProvider

	// First, try to get the provider by ID (new behavior - UI passes provider ID)
	provider, providerErr := r.store.GetProvider(ctx, primitiveConfig.EmbeddingProvider)
	if providerErr != nil {
		// Fallback: try to interpret embedding_provider as a provider type (old behavior)
		// This handles cases where the config has a provider type instead of ID
		providerType := primitiveConfig.EmbeddingProvider
		switch providerType {
		case "openai", "gemini", "ollama":
			// Try to find a provider with a matching name
			providers, listErr := r.store.ListProviders(ctx)
			if listErr == nil {
				for _, p := range providers {
					if p.Name == providerType || strings.EqualFold(p.Name, providerType) {
						provider = p
						providerErr = nil
						break
					}
				}
			}
		}
	}

	var apiKey, baseURL string

	if providerErr == nil && provider != nil {
		// Successfully found a provider by ID
		apiKey = provider.APIKeyEnc // This should be decrypted in a real implementation
		baseURL = provider.APIBaseURL
	}

	// Mule only supports OpenAI providers for now
	providerType := "openai"

	// Create a genai Provider which implements the EmbeddingProvider interface
	// Use the credentials from Mule's configured provider if available
	genaiProvider, err := jbutlerdevgenai.NewProvider(providerType, jbutlerdevgenai.ProviderOptions{
		APIKey:         apiKey,
		BaseURL:        baseURL,
		EmbeddingModel: primitiveConfig.EmbeddingModel,
	})
	if err != nil {
		return fmt.Errorf("failed to create embedding provider: %w", err)
	}
	embeddingProvider = genaiProvider

	// Initialize the memory tool
	memoryTool, err := genaitools.NewMemoryTool(memoryConfig, embeddingProvider)
	if err != nil {
		return fmt.Errorf("failed to create memory tool: %w", err)
	}

	// Register the memory tool
	r.Register(&genaiMemoryToolAdapter{tool: memoryTool})
	return nil
}

// ReinitializeMemoryTool re-initializes the memory tool when configuration changes
func (r *Registry) ReinitializeMemoryTool() error {
	// Check if we have a store available (NewRegistryWithConfig succeeded)
	if r.store == nil {
		// No store available, can't reinitialize the memory tool
		return fmt.Errorf("no configuration store available - memory tool not initialized with config support")
	}

	// Remove the old memory tool if it exists
	r.mu.Lock()
	delete(r.tools, "memory")
	r.mu.Unlock()

	// Re-initialize with new configuration
	return r.initializeMemoryTool()
}

// Register registers a tool in the registry
func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.tools[tool.Name()] = tool
}

// Get retrieves a tool by name
func (r *Registry) Get(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tool, exists := r.tools[name]
	if !exists {
		return nil, fmt.Errorf("tool not found: %s", name)
	}

	return tool, nil
}

// List returns all registered tools
func (r *Registry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tools := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		tools = append(tools, tool)
	}

	return tools
}

// GetToolNames returns a list of all registered tool names
func (r *Registry) GetToolNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}

	return names
}

// genaiMemoryToolAdapter adapts the genai MemoryTool to mule's Tool interface
type genaiMemoryToolAdapter struct {
	tool *genaitools.MemoryTool
}

func (a *genaiMemoryToolAdapter) Name() string {
	return "memory"
}

func (a *genaiMemoryToolAdapter) Description() string {
	return "Store and retrieve memories using semantic search with vector embeddings"
}

func (a *genaiMemoryToolAdapter) IsLongRunning() bool {
	return false
}

func (a *genaiMemoryToolAdapter) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	// The genai memory tool uses separate operations, so we need to handle them here
	operation, ok := params["operation"].(string)
	if !ok {
		return nil, fmt.Errorf("operation parameter is required")
	}

	switch operation {
	case "store":
		return a.executeStore(ctx, params)
	case "retrieve":
		return a.executeRetrieve(ctx, params)
	case "update":
		return a.executeUpdate(ctx, params)
	case "delete":
		return a.executeDelete(ctx, params)
	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)
	}
}

func (a *genaiMemoryToolAdapter) executeStore(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	content, ok := params["content"].(string)
	if !ok {
		return nil, fmt.Errorf("content parameter is required for store operation")
	}

	var metadata map[string]interface{}
	if meta, ok := params["metadata"]; ok {
		if metaMap, ok := meta.(map[string]interface{}); ok {
			metadata = metaMap
		}
	}

	id, err := a.tool.Store(ctx, content, metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to store memory: %w", err)
	}

	return map[string]interface{}{
		"id":      id,
		"success": true,
	}, nil
}

func (a *genaiMemoryToolAdapter) executeRetrieve(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	query, ok := params["query"].(string)
	if !ok {
		return nil, fmt.Errorf("query parameter is required for retrieve operation")
	}

	options := genaitools.RetrieveOptions{}

	if topK, ok := params["top_k"]; ok {
		if topKFloat, ok := topK.(float64); ok {
			options.TopK = int(topKFloat)
		} else if topKInt, ok := topK.(int); ok {
			options.TopK = topKInt
		}
	}

	if filters, ok := params["filters"]; ok {
		if filterMap, ok := filters.(map[string]interface{}); ok {
			options.Filters = filterMap
		}
	}

	results, err := a.tool.Retrieve(ctx, query, options)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve memories: %w", err)
	}

	// Convert results to a serializable format
	serializableResults := make([]map[string]interface{}, len(results))
	for i, result := range results {
		serializableResults[i] = map[string]interface{}{
			"id":         result.ID,
			"content":    result.Content,
			"metadata":   result.Metadata,
			"similarity": result.Similarity,
			"created_at": result.CreatedAt,
		}
		if result.ExpiresAt != nil {
			serializableResults[i]["expires_at"] = *result.ExpiresAt
		}
	}

	return map[string]interface{}{
		"results": serializableResults,
		"count":   len(results),
	}, nil
}

func (a *genaiMemoryToolAdapter) executeUpdate(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id parameter is required for update operation")
	}

	content, ok := params["content"].(string)
	if !ok {
		return nil, fmt.Errorf("content parameter is required for update operation")
	}

	var metadata map[string]interface{}
	if meta, ok := params["metadata"]; ok {
		if metaMap, ok := meta.(map[string]interface{}); ok {
			metadata = metaMap
		}
	}

	err := a.tool.Update(ctx, id, content, metadata)
	if err != nil {
		return nil, fmt.Errorf("failed to update memory: %w", err)
	}

	return map[string]interface{}{
		"success": true,
		"id":      id,
	}, nil
}

func (a *genaiMemoryToolAdapter) executeDelete(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	id, ok := params["id"].(string)
	if !ok {
		return nil, fmt.Errorf("id parameter is required for delete operation")
	}

	err := a.tool.Delete(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to delete memory: %w", err)
	}

	return map[string]interface{}{
		"success": true,
		"id":      id,
	}, nil
}

func (a *genaiMemoryToolAdapter) GetSchema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"operation": map[string]interface{}{
				"type":        "string",
				"description": "The operation to perform: store, retrieve, update, or delete",
				"enum":        []string{"store", "retrieve", "update", "delete"},
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The content to store or update (required for store/update operations)",
			},
			"id": map[string]interface{}{
				"type":        "string",
				"description": "The ID of the memory to update or delete (required for update/delete operations)",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "The query to search for similar memories (required for retrieve operation)",
			},
			"top_k": map[string]interface{}{
				"type":        "integer",
				"description": "Number of results to return (optional for retrieve operation)",
			},
			"filters": map[string]interface{}{
				"type":        "object",
				"description": "Metadata filters to apply (optional for retrieve operation)",
			},
			"metadata": map[string]interface{}{
				"type":        "object",
				"description": "Metadata to associate with the memory (optional for store/update operations)",
			},
		},
		"required": []string{"operation"},
	}
}

// BuiltInTools returns a list of built-in tool names
func BuiltInTools() []string {
	return []string{
		"memory",
		"filesystem",
		"http",
		"database",
		"bash",
	}
}
