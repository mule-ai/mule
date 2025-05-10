# pkg/agent Package
## Overview
Manages AI agent configurations, workflow execution, and code generation capabilities. Provides interfaces for:
- Agent lifecycle management
- Workflow coordination between multiple agents
- Unified diff (udiff) application to code repositories

## Key Components
```go
type Agent struct {
    ID          int
    Provider    string
    Model       string
    Tools       []string
    SystemPrompt string
}

type Workflow struct {
    Steps []*WorkflowStep
}

func CreateAgent(cfg AgentOptions) (*Agent, error)
func ExecuteWorkflow(wf *Workflow, input string) (string, error)
```
