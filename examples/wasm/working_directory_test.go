package main

import (
	"context"
	"fmt"
	"log"

	"github.com/mule-ai/mule/internal/database"
	"github.com/mule-ai/mule/internal/manager"
)

// This example demonstrates a workflow that changes the working directory
// and shows how subsequent steps operate in the new directory

func main() {
	// Connect to database (adjust connection string as needed)
	db, err := database.NewDB(database.Config{
		Host:     "localhost",
		Port:     5432,
		User:     "mule",
		Password: "mule",
		DBName:   "mulev2",
		SSLMode:  "disable",
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Failed to close database: %v", err)
		}
	}()

	// Create managers
	secret := []byte("test-secret-key-for-encryption-32-bytes-long!!") // 32 bytes for AES-256
	providerMgr := manager.NewProviderManager(db, secret)
	agentMgr := manager.NewAgentManager(db)
	toolMgr := manager.NewToolManager(db)
	workflowMgr := manager.NewWorkflowManager(db)

	ctx := context.Background()

	// Create a provider
	provider, err := providerMgr.CreateProvider(ctx, "Test Provider", "", "test-key")
	if err != nil {
		log.Fatalf("Failed to create provider: %v", err)
	}
	fmt.Printf("Created provider: %s\n", provider.ID)

	// Create a filesystem tool
	config := map[string]interface{}{
		"root": ".",
	}

	filesystemTool, err := toolMgr.CreateTool(ctx, "filesystem", "Filesystem tool for file operations", "filesystem", config)
	if err != nil {
		log.Fatalf("Failed to create filesystem tool: %v", err)
	}
	fmt.Printf("Created filesystem tool: %s\n", filesystemTool.ID)

	// Create an agent with filesystem tool
	agent, err := agentMgr.CreateAgent(ctx, "Filesystem Agent", "Agent with filesystem access", provider.ID, "gemini-1.5-flash", "You are an agent that can manipulate files. Use the filesystem tool to create directories and files as instructed.")
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}
	fmt.Printf("Created agent: %s\n", agent.ID)

	// Associate the filesystem tool with the agent
	err = agentMgr.AddToolToAgent(ctx, agent.ID, filesystemTool.ID)
	if err != nil {
		log.Fatalf("Failed to add tool to agent: %v", err)
	}
	fmt.Printf("Associated filesystem tool with agent\n")

	// Note: For the WASM module, you would need to compile the working-dir-demo module
	// and add it through the API or CLI. For this example, we'll just show how to
	// create a workflow that would use such a module.

	// Create a workflow that demonstrates working directory changes
	workflow, err := workflowMgr.CreateWorkflow(ctx, "Working Directory Test", "Workflow demonstrating working directory changes", false)
	if err != nil {
		log.Fatalf("Failed to create workflow: %v", err)
	}
	fmt.Printf("Created workflow: %s\n", workflow.ID)

	// Step 1: Agent that creates a directory and sets it as working directory
	// In a real implementation, this would be a WASM module that calls set_working_directory
	step1Config := map[string]interface{}{
		"description": "Create a directory and set it as working directory",
	}

	step1, err := workflowMgr.CreateWorkflowStep(ctx, workflow.ID, 1, "agent", &agent.ID, nil, step1Config)
	if err != nil {
		log.Fatalf("Failed to create workflow step 1: %v", err)
	}
	fmt.Printf("Created workflow step 1 (Agent): %s\n", step1.ID)

	// Step 2: Agent that operates in the new working directory
	step2Config := map[string]interface{}{
		"description": "Operate in the new working directory",
	}

	step2, err := workflowMgr.CreateWorkflowStep(ctx, workflow.ID, 2, "agent", &agent.ID, nil, step2Config)
	if err != nil {
		log.Fatalf("Failed to create workflow step 2: %v", err)
	}
	fmt.Printf("Created workflow step 2 (Agent): %s\n", step2.ID)

	fmt.Println("\nWorking directory test workflow setup complete!")
	fmt.Printf("Workflow ID: %s\n", workflow.ID)
	fmt.Println("\nTo use this workflow:")
	fmt.Println("1. Compile the WASM module in examples/wasm/working-dir-demo/")
	fmt.Println("2. Add the WASM module through the API or CLI")
	fmt.Println("3. Update the workflow to use the WASM module for step 1")
	fmt.Println("4. Run the workflow to see working directory changes in action")
}