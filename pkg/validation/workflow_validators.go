package validation

import (
	"fmt"
	"strings"
)

// PlanValidation checks if a plan validation result indicates success
func PlanValidation(output string) (string, error) {
	// The output here is from the validation agent (last step in the workflow)
	// We need to check if the validation agent approved the plan

	// Check if the validation output contains INVALID first (more specific)
	if strings.Contains(output, "INVALID:") {
		// Extract the reason after INVALID:
		parts := strings.Split(output, "INVALID:")
		if len(parts) > 1 {
			reason := strings.TrimSpace(parts[1])
			return output, fmt.Errorf("plan validation failed: %s", reason)
		}
		return output, fmt.Errorf("plan validation failed")
	}

	// Check if the validation output contains VALID
	if strings.Contains(output, "VALID:") {
		// Return the validated content (which is the validation message itself)
		return output, nil
	}

	// If neither VALID nor INVALID is found, return error
	return output, fmt.Errorf("plan validation did not produce a clear result")
}

// ResearchValidation checks if a research validation result indicates success
func ResearchValidation(output string) (string, error) {
	// Check if the validation output contains INVALID first (more specific)
	if strings.Contains(output, "INVALID:") {
		// Extract the reason after INVALID:
		parts := strings.Split(output, "INVALID:")
		if len(parts) > 1 {
			reason := strings.TrimSpace(parts[1])
			return output, fmt.Errorf("research validation failed: %s", reason)
		}
		return output, fmt.Errorf("research validation failed")
	}

	// Check if the validation output contains VALID
	if strings.Contains(output, "VALID:") {
		return output, nil
	}

	// If neither VALID nor INVALID is found, return error
	return output, fmt.Errorf("research validation did not produce a clear result")
}
