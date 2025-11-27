package initialization

import (
	"context"
	"fmt"

	"github.com/mule-ai/mule/internal/manager"
	dbmodels "github.com/mule-ai/mule/pkg/database"
)

// DefaultPrimitives contains all the default primitive configurations
type DefaultPrimitives struct {
	Provider        *dbmodels.Provider
	DefaultAgent    *dbmodels.Agent
	WasmEditorAgent *dbmodels.Agent
	Workflow        *dbmodels.Workflow
}

// EnsureAllDefaults ensures all default primitives exist
func EnsureAllDefaults(ctx context.Context,
	providerMgr *manager.ProviderManager,
	agentMgr *manager.AgentManager,
	workflowMgr *manager.WorkflowManager) (*DefaultPrimitives, error) {

	result := &DefaultPrimitives{}

	// Ensure default provider
	provider, err := ensureDefaultProvider(ctx, providerMgr)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure default provider: %w", err)
	}
	result.Provider = provider

	// Ensure default agent
	defaultAgent, err := ensureDefaultAgent(ctx, agentMgr, provider.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure default agent: %w", err)
	}
	result.DefaultAgent = defaultAgent

	// Ensure WASM editor agent
	wasmEditorAgent, err := ensureWasmEditorAgent(ctx, agentMgr, provider.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure WASM editor agent: %w", err)
	}
	result.WasmEditorAgent = wasmEditorAgent

	// Ensure default workflow
	workflow, err := ensureDefaultWorkflow(ctx, workflowMgr, defaultAgent.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to ensure default workflow: %w", err)
	}
	result.Workflow = workflow

	return result, nil
}

func ensureDefaultProvider(ctx context.Context, providerMgr *manager.ProviderManager) (*dbmodels.Provider, error) {
	// Try to find existing "Default" provider
	providers, err := providerMgr.ListProviders(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list providers: %w", err)
	}

	for _, p := range providers {
		if p.Name == "Default" {
			return p, nil
		}
	}

	// Create default provider if it doesn't exist
	created, err := providerMgr.CreateProvider(ctx, "Default", "https://openrouter.ai/api/v1", "")
	if err != nil {
		return nil, fmt.Errorf("failed to create default provider: %w", err)
	}

	return created, nil
}

func ensureDefaultAgent(ctx context.Context, agentMgr *manager.AgentManager, providerID string) (*dbmodels.Agent, error) {
	// Try to find existing "Default" agent
	agents, err := agentMgr.ListAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}

	for _, a := range agents {
		if a.Name == "Default" {
			return a, nil
		}
	}

	// Create default agent if it doesn't exist
	created, err := agentMgr.CreateAgent(ctx, "Default", "Default agent for general purpose tasks", providerID, "anthropic/claude-sonnet-4.5", "You are a helpful assistant")
	if err != nil {
		return nil, fmt.Errorf("failed to create default agent: %w", err)
	}

	return created, nil
}

func ensureWasmEditorAgent(ctx context.Context, agentMgr *manager.AgentManager, providerID string) (*dbmodels.Agent, error) {
	// Try to find existing "WASM Editor" agent
	agents, err := agentMgr.ListAgents(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list agents: %w", err)
	}

	for _, a := range agents {
		if a.Name == "WASM Editor" {
			return a, nil
		}
	}

	// Create WASM Editor agent if it doesn't exist
	wasmSystemPrompt := `You are an expert WebAssembly (WASM) module developer specializing in creating modules for the Mule AI workflow platform.

Your role is to help users create, debug, and optimize WASM modules that integrate seamlessly with Mule workflows.

**WASM Module Requirements:**

1. **Language**: Go (primary language for WASM modules in Mule)
2. **Input/Output**: Modules must communicate via JSON through stdin/stdout
3. **Package**: Must use 'package main'
4. **Execution**: Must have a main() function as entry point

**Input Structure:**
Modules receive input via stdin in this format:
{
  "prompt": "string - content from previous workflow step",
  "data": {} // optional additional data from previous steps
}

**Output Structure:**
Modules must output JSON to stdout in this format:
{
  "result": "string - processing result",
  "data": {} // optional processed data for next steps,
  "success": true/false
}

**Key Development Guidelines:**

1. Always handle JSON parsing errors gracefully
2. Use appropriate Go data structures with json tags
3. Write clear, well-documented code
4. Process input flexibly - previous steps may output different formats
5. Ensure output is valid JSON that next workflow steps can consume
6. Use os.Stderr for error logging (not for program output)
7. Keep modules focused on a single responsibility

**Common Module Patterns:**

- Text processing and transformation
- Data validation and cleaning
- API response formatting
- Content analysis and extraction
- Data structure conversions

When helping users:
1. Ask clarifying questions about module purpose if needed
2. Provide complete, runnable Go code
3. Explain the input/output handling
4. Suggest improvements for robustness
5. Help debug compilation errors
6. Optimize for workflow integration

Remember: The goal is to create reliable, efficient WASM modules that enhance Mule workflows.`

	created, err := agentMgr.CreateAgent(ctx, "WASM Editor", "Specialized agent for helping create and debug WASM modules", providerID, "anthropic/claude-sonnet-4.5", wasmSystemPrompt)
	if err != nil {
		return nil, fmt.Errorf("failed to create WASM editor agent: %w", err)
	}

	return created, nil
}

func ensureDefaultWorkflow(ctx context.Context,
	workflowMgr *manager.WorkflowManager,
	defaultAgentID string) (*dbmodels.Workflow, error) {

	// Try to find existing "Default" workflow
	workflows, err := workflowMgr.ListWorkflows(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list workflows: %w", err)
	}

	for _, w := range workflows {
		if w.Name == "Default" {
			return w, nil
		}
	}

	// Create default workflow if it doesn't exist
	createdWorkflow, err := workflowMgr.CreateWorkflow(ctx, "Default", "Default workflow with a single agent step", false)
	if err != nil {
		return nil, fmt.Errorf("failed to create default workflow: %w", err)
	}

	return createdWorkflow, nil
}
