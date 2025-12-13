package manager

import (
	"context"
	"os"
	"testing"

	"github.com/mule-ai/mule/internal/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeleteWorkflowStep_Renumbering(t *testing.T) {
	// Skip if no test database is available or in CI
	if testing.Short() || os.Getenv("CI") != "" {
		t.Skip("Skipping database test that requires PostgreSQL")
	}

	// Use a test database
	config := database.Config{
		Host:     "localhost",
		Port:     5432,
		User:     "postgres",
		Password: "postgres",
		DBName:   "mulev2_test",
		SSLMode:  "disable",
	}

	db, err := database.NewDB(config)
	if err != nil {
		t.Skipf("Could not connect to test database: %v", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("Error closing database: %v", closeErr)
		}
	}()

	// Initialize schema
	err = db.InitSchema()
	require.NoError(t, err, "Schema initialization should not fail")

	// Create workflow manager
	wm := NewWorkflowManager(db)

	ctx := context.Background()

	// Create a workflow
	workflow, err := wm.CreateWorkflow(ctx, "test-workflow", "Test workflow for step deletion", false)
	require.NoError(t, err)
	require.NotNil(t, workflow)

	// Create multiple workflow steps
	step1, err := wm.CreateWorkflowStep(ctx, workflow.ID, 1, "agent", nil, nil, map[string]interface{}{})
	require.NoError(t, err)
	require.NotNil(t, step1)

	step2, err := wm.CreateWorkflowStep(ctx, workflow.ID, 2, "agent", nil, nil, map[string]interface{}{})
	require.NoError(t, err)
	require.NotNil(t, step2)

	step3, err := wm.CreateWorkflowStep(ctx, workflow.ID, 3, "agent", nil, nil, map[string]interface{}{})
	require.NoError(t, err)
	require.NotNil(t, step3)

	// Verify all steps exist with correct order
	steps, err := wm.GetWorkflowSteps(ctx, workflow.ID)
	require.NoError(t, err)
	require.Len(t, steps, 3)
	assert.Equal(t, 1, steps[0].StepOrder)
	assert.Equal(t, step1.ID, steps[0].ID)
	assert.Equal(t, 2, steps[1].StepOrder)
	assert.Equal(t, step2.ID, steps[1].ID)
	assert.Equal(t, 3, steps[2].StepOrder)
	assert.Equal(t, step3.ID, steps[2].ID)

	// Delete the middle step (step2)
	err = wm.DeleteWorkflowStep(ctx, step2.ID)
	require.NoError(t, err)

	// Verify remaining steps are renumbered correctly
	steps, err = wm.GetWorkflowSteps(ctx, workflow.ID)
	require.NoError(t, err)
	require.Len(t, steps, 2)
	assert.Equal(t, 1, steps[0].StepOrder)
	assert.Equal(t, step1.ID, steps[0].ID)
	assert.Equal(t, 2, steps[1].StepOrder)
	assert.Equal(t, step3.ID, steps[1].ID)

	// Delete the first step
	err = wm.DeleteWorkflowStep(ctx, step1.ID)
	require.NoError(t, err)

	// Verify the last step is renumbered correctly
	steps, err = wm.GetWorkflowSteps(ctx, workflow.ID)
	require.NoError(t, err)
	require.Len(t, steps, 1)
	assert.Equal(t, 1, steps[0].StepOrder)
	assert.Equal(t, step3.ID, steps[0].ID)
}