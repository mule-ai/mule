package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/mule-ai/mule/internal/database"
	"github.com/mule-ai/mule/internal/manager"
	"github.com/mule-ai/mule/internal/primitive"
)

// This example demonstrates how to programmatically create a workflow with steps
// It's meant to be run as a standalone Go program, not as a WASM module

func main() {
	// Connect to database (adjust connection string as needed)
	db, err := database.NewDB("postgres://mule:mule@localhost:5432/mulev2?sslmode=disable")
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Create managers
	providerMgr := manager.NewProviderManager(db)
	agentMgr := manager.NewAgentManager(db)
	workflowMgr := manager.NewWorkflowManager(db)

	ctx := context.Background()

	// Create a provider
	provider, err := providerMgr.CreateProvider(ctx, "Example Provider", "", "test-key")
	if err != nil {
		log.Fatalf("Failed to create provider: %v", err)
	}
	fmt.Printf("Created provider: %s\n", provider.ID)

	// Create an agent
	agent, err := agentMgr.CreateAgent(ctx, "Example Agent", "Example agent for workflow", provider.ID, "gemini-1.5-flash", "You are a helpful assistant.")
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}
	fmt.Printf("Created agent: %s\n", agent.ID)

	// Create a workflow
	workflow, err := workflowMgr.CreateWorkflow(ctx, "Programmatic Workflow", "Workflow created programmatically", false)
	if err != nil {
		log.Fatalf("Failed to create workflow: %v", err)
	}
	fmt.Printf("Created workflow: %s\n", workflow.ID)

	// Add a step to the workflow
	step, err := workflowMgr.CreateWorkflowStep(ctx, workflow.ID, 1, "agent", &agent.ID, nil, nil)
	if err != nil {
		log.Fatalf("Failed to create workflow step: %v", err)
	}
	fmt.Printf("Created workflow step: %s\n", step.ID)

	fmt.Println("Workflow setup complete!")
	fmt.Printf("You can now use this workflow with ID: %s\n", workflow.ID)
}