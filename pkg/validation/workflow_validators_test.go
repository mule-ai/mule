package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanValidation(t *testing.T) {
	t.Run("Valid plan passes validation", func(t *testing.T) {
		output := "VALID: The plan is comprehensive and well-structured."
		result, err := PlanValidation(output)
		require.NoError(t, err)
		assert.Equal(t, output, result)
	})

	t.Run("Invalid plan fails validation with reason", func(t *testing.T) {
		output := "INVALID: The plan is too vague and lacks specific steps."
		result, err := PlanValidation(output)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plan validation failed: The plan is too vague and lacks specific steps.")
		assert.Equal(t, output, result)
	})

	t.Run("Invalid plan fails validation without reason", func(t *testing.T) {
		output := "INVALID:"
		result, err := PlanValidation(output)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plan validation failed")
		assert.Equal(t, output, result)
	})

	t.Run("Unclear result fails validation", func(t *testing.T) {
		output := "The plan looks okay I guess"
		result, err := PlanValidation(output)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "plan validation did not produce a clear result")
		assert.Equal(t, output, result)
	})
}

func TestResearchValidation(t *testing.T) {
	t.Run("Valid research passes validation", func(t *testing.T) {
		output := "VALID: The report thoroughly answers the original question."
		result, err := ResearchValidation(output)
		require.NoError(t, err)
		assert.Equal(t, output, result)
	})

	t.Run("Invalid research fails validation with reason", func(t *testing.T) {
		output := "INVALID: The report lacks sufficient detail and citations."
		result, err := ResearchValidation(output)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "research validation failed: The report lacks sufficient detail and citations.")
		assert.Equal(t, output, result)
	})

	t.Run("Invalid research fails validation without reason", func(t *testing.T) {
		output := "INVALID:"
		result, err := ResearchValidation(output)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "research validation failed")
		assert.Equal(t, output, result)
	})

	t.Run("Unclear result fails validation", func(t *testing.T) {
		output := "This research seems fine"
		result, err := ResearchValidation(output)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "research validation did not produce a clear result")
		assert.Equal(t, output, result)
	})
}

func TestValidationFunctionRegistration(t *testing.T) {
	t.Run("Plan validation function is registered", func(t *testing.T) {
		fn, exists := Get("planValidation")
		require.True(t, exists)
		assert.NotNil(t, fn)

		// Test the retrieved function
		result, err := fn("VALID: Test plan")
		require.NoError(t, err)
		assert.Equal(t, "VALID: Test plan", result)
	})

	t.Run("Research validation function is registered", func(t *testing.T) {
		fn, exists := Get("researchValidation")
		require.True(t, exists)
		assert.NotNil(t, fn)

		// Test the retrieved function
		result, err := fn("VALID: Test research")
		require.NoError(t, err)
		assert.Equal(t, "VALID: Test research", result)
	})
}
