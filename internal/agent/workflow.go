package agent

import (
	"context"
	"github.com/mule-ai/mule/pkg/job"
)

// WorkflowEngine interface defines the methods needed for workflow execution
// This interface is used to avoid import cycles between agent and engine packages
type WorkflowEngine interface {
	SubmitJob(ctx context.Context, workflowID string, inputData map[string]interface{}) (*job.Job, error)
}
