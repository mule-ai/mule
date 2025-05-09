# internal/scheduler Package
## Overview
Manages periodic execution of repository analysis and workflow scheduling. Handles timing and orchestration of:
- Repository scanning intervals
- Workflow execution triggers
- Agent task scheduling

## Key Functions
```go
// ScheduleWorkflow() - Creates a new workflow execution schedule
ScheduleWorkflow(ctx context.Context, interval time.Duration)

// GetNextExecution() - Calculates the next scheduled execution time
GetNextExecution(currentTime time.Time) time.Time
```
