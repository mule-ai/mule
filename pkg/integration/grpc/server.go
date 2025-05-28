package grpc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/jbutlerdev/genai"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/mule-ai/mule/api/proto"
	"github.com/mule-ai/mule/pkg/agent"
)

// Server implements the MuleService gRPC server
type Server struct {
	pb.UnimplementedMuleServiceServer
	logger           logr.Logger
	runningWorkflows map[string]*RunningWorkflowExecution
	mu               sync.RWMutex
	agents           map[int]*agent.Agent
	workflows        map[string]*agent.Workflow
	providers        map[string]*genai.Provider
}

// RunningWorkflowExecution tracks a workflow execution
type RunningWorkflowExecution struct {
	ID           string
	WorkflowName string
	Status       string
	StartedAt    time.Time
	StepResults  []*pb.WorkflowStepResult
	CurrentStep  string
	Context      context.Context
	Cancel       context.CancelFunc
}

// NewServer creates a new gRPC server instance
func NewServer(logger logr.Logger, agents map[int]*agent.Agent, workflows map[string]*agent.Workflow, providers map[string]*genai.Provider) *Server {
	return &Server{
		logger:           logger.WithName("grpc-server"),
		runningWorkflows: make(map[string]*RunningWorkflowExecution),
		agents:           agents,
		workflows:        workflows,
		providers:        providers,
	}
}

func (s *Server) SetAgents(agents map[int]*agent.Agent) {
	s.agents = agents
}

func (s *Server) SetWorkflows(workflows map[string]*agent.Workflow) {
	s.workflows = workflows
}

func (s *Server) SetProviders(providers map[string]*genai.Provider) {
	s.providers = providers
}

// GetHeartbeat returns a heartbeat response
func (s *Server) GetHeartbeat(ctx context.Context, req *pb.HeartbeatRequest) (*pb.HeartbeatResponse, error) {
	s.logger.Info("Heartbeat request received")

	return &pb.HeartbeatResponse{
		Status:    "healthy",
		Timestamp: timestamppb.Now(),
		Version:   "1.0.0", // TODO: Get actual version
	}, nil
}

// ListWorkflows returns all available workflows
func (s *Server) ListWorkflows(ctx context.Context, req *pb.ListWorkflowsRequest) (*pb.ListWorkflowsResponse, error) {
	s.logger.Info("ListWorkflows request received")

	var workflows []*pb.Workflow
	for _, workflow := range s.workflows {
		pbWorkflow := s.convertWorkflowToPB(workflow)
		workflows = append(workflows, pbWorkflow)
	}

	return &pb.ListWorkflowsResponse{
		Workflows: workflows,
	}, nil
}

// GetWorkflow returns details about a specific workflow
func (s *Server) GetWorkflow(ctx context.Context, req *pb.GetWorkflowRequest) (*pb.GetWorkflowResponse, error) {
	s.logger.Info("GetWorkflow request received", "name", req.Name)

	workflow, exists := s.workflows[req.Name]
	if !exists {
		return nil, fmt.Errorf("workflow not found: %s", req.Name)
	}

	pbWorkflow := s.convertWorkflowToPB(workflow)

	return &pb.GetWorkflowResponse{
		Workflow: pbWorkflow,
	}, nil
}

// ListProviders returns all available providers
func (s *Server) ListProviders(ctx context.Context, req *pb.ListProvidersRequest) (*pb.ListProvidersResponse, error) {
	s.logger.Info("ListProviders request received")

	var providers []*pb.Provider
	for _, provider := range s.providers {
		pbProvider := s.convertProviderToPB(provider)
		providers = append(providers, pbProvider)
	}

	return &pb.ListProvidersResponse{
		Providers: providers,
	}, nil
}

// convertProviderToPB converts a genai.Provider to a pb.Provider
func (s *Server) convertProviderToPB(provider *genai.Provider) *pb.Provider {
	return &pb.Provider{
		Name: provider.Name,
	}
}

// ListAgents returns all available agents
func (s *Server) ListAgents(ctx context.Context, req *pb.ListAgentsRequest) (*pb.ListAgentsResponse, error) {
	s.logger.Info("ListAgents request received")

	var agents []*pb.Agent
	for _, agentInstance := range s.agents {
		pbAgent := s.convertAgentToPB(agentInstance)
		agents = append(agents, pbAgent)
	}

	return &pb.ListAgentsResponse{
		Agents: agents,
	}, nil
}

// GetAgent returns details about a specific agent
func (s *Server) GetAgent(ctx context.Context, req *pb.GetAgentRequest) (*pb.GetAgentResponse, error) {
	s.logger.Info("GetAgent request received", "id", req.Id)

	agentInstance, exists := s.agents[int(req.Id)]
	if !exists {
		return nil, fmt.Errorf("agent not found: %d", req.Id)
	}

	pbAgent := s.convertAgentToPB(agentInstance)

	return &pb.GetAgentResponse{
		Agent: pbAgent,
	}, nil
}

// ListRunningWorkflows returns currently executing workflows
func (s *Server) ListRunningWorkflows(ctx context.Context, req *pb.ListRunningWorkflowsRequest) (*pb.ListRunningWorkflowsResponse, error) {
	s.logger.Info("ListRunningWorkflows request received")

	s.mu.RLock()
	defer s.mu.RUnlock()

	var runningWorkflows []*pb.RunningWorkflow
	for _, execution := range s.runningWorkflows {
		pbRunningWorkflow := &pb.RunningWorkflow{
			ExecutionId:  execution.ID,
			WorkflowName: execution.WorkflowName,
			Status:       execution.Status,
			StartedAt:    timestamppb.New(execution.StartedAt),
			StepResults:  execution.StepResults,
			CurrentStep:  execution.CurrentStep,
		}
		runningWorkflows = append(runningWorkflows, pbRunningWorkflow)
	}

	return &pb.ListRunningWorkflowsResponse{
		RunningWorkflows: runningWorkflows,
	}, nil
}

// ExecuteWorkflow starts a new workflow execution
func (s *Server) ExecuteWorkflow(ctx context.Context, req *pb.ExecuteWorkflowRequest) (*pb.ExecuteWorkflowResponse, error) {
	s.logger.Info("ExecuteWorkflow request received", "workflow", req.WorkflowName, "prompt", req.Prompt)

	workflow, exists := s.workflows[req.WorkflowName]

	if !exists {
		return nil, fmt.Errorf("workflow not found: %s", req.WorkflowName)
	}

	// Generate execution ID
	executionID := uuid.New().String()

	// Create execution context
	execCtx, cancel := context.WithCancel(context.Background())

	// Create running workflow execution
	execution := &RunningWorkflowExecution{
		ID:           executionID,
		WorkflowName: req.WorkflowName,
		Status:       "running",
		StartedAt:    time.Now(),
		StepResults:  make([]*pb.WorkflowStepResult, 0),
		CurrentStep:  "",
		Context:      execCtx,
		Cancel:       cancel,
	}

	// Store the execution
	s.mu.Lock()
	s.runningWorkflows[executionID] = execution
	s.mu.Unlock()

	// Execute workflow in goroutine
	go s.executeWorkflowAsync(execution, workflow, req.Prompt, req.Path)

	return &pb.ExecuteWorkflowResponse{
		ExecutionId: executionID,
		Status:      "started",
		Message:     "Workflow execution started successfully",
	}, nil
}

// Helper method to convert agent.Workflow to pb.Workflow
func (s *Server) convertWorkflowToPB(workflow *agent.Workflow) *pb.Workflow {
	// Get workflow settings using reflection or public methods
	// Since Workflow.settings is private, we'll need to access public fields

	var steps []*pb.WorkflowStep
	for _, step := range workflow.Steps {
		pbStep := &pb.WorkflowStep{
			Id:          step.ID,
			AgentId:     int32(step.AgentID),
			AgentName:   step.AgentName,
			OutputField: step.OutputField,
			Integration: &pb.TriggerSettings{
				Integration: step.Integration.Integration,
				Name:        step.Integration.Event,
				Data:        make(map[string]string),
			},
		}

		// Convert integration data
		if step.Integration.Data != nil {
			if dataMap, ok := step.Integration.Data.(map[string]interface{}); ok {
				for k, v := range dataMap {
					if strVal, ok := v.(string); ok {
						pbStep.Integration.Data[k] = strVal
					}
				}
			}
		}

		steps = append(steps, pbStep)
	}

	settings := workflow.GetSettings()

	// Convert triggers
	var triggers []*pb.TriggerSettings
	for _, trigger := range settings.Triggers {
		pbTrigger := &pb.TriggerSettings{
			Integration: trigger.Integration,
			Name:        trigger.Event,
			Data:        make(map[string]string),
		}

		if trigger.Data != nil {
			if dataMap, ok := trigger.Data.(map[string]interface{}); ok {
				for k, v := range dataMap {
					if strVal, ok := v.(string); ok {
						pbTrigger.Data[k] = strVal
					}
				}
			}
		}

		triggers = append(triggers, pbTrigger)
	}

	// Convert outputs
	var outputs []*pb.TriggerSettings
	for _, output := range settings.Outputs {
		pbOutput := &pb.TriggerSettings{
			Integration: output.Integration,
			Name:        output.Event,
			Data:        make(map[string]string),
		}

		if output.Data != nil {
			if dataMap, ok := output.Data.(map[string]interface{}); ok {
				for k, v := range dataMap {
					if strVal, ok := v.(string); ok {
						pbOutput.Data[k] = strVal
					}
				}
			}
		}

		outputs = append(outputs, pbOutput)
	}

	return &pb.Workflow{
		Id:                  settings.ID,
		Name:                settings.Name,
		Description:         settings.Description,
		IsDefault:           settings.IsDefault,
		Steps:               steps,
		ValidationFunctions: workflow.ValidationFunctions,
		Triggers:            triggers,
		Outputs:             outputs,
	}
}

// Helper method to convert agent.Agent to pb.Agent
func (s *Server) convertAgentToPB(agentInstance *agent.Agent) *pb.Agent {
	// Since Agent fields might be private, we'll need to use reflection or add public getters
	// For now, we'll create a minimal agent representation

	udiffSettings := agentInstance.GetUDiffSettings()

	return &pb.Agent{
		Id:             int32(agentInstance.GetID()),
		Name:           agentInstance.Name,
		ProviderName:   agentInstance.GetProviderName(),
		Model:          agentInstance.GetModel(),
		PromptTemplate: agentInstance.GetPromptTemplate(),
		SystemPrompt:   agentInstance.GetSystemPrompt(),
		Tools:          agentInstance.GetTools(),
		UdiffSettings: &pb.UDiffSettings{
			Enabled: udiffSettings.Enabled,
		},
	}
}

// executeWorkflowAsync executes a workflow asynchronously
func (s *Server) executeWorkflowAsync(execution *RunningWorkflowExecution, workflow *agent.Workflow, prompt string, path string) {
	defer func() {
		// Clean up completed execution after some time
		time.AfterFunc(10*time.Minute, func() {
			s.mu.Lock()
			delete(s.runningWorkflows, execution.ID)
			s.mu.Unlock()
		})
	}()

	// Execute the workflow
	workflow.Execute(prompt)

	execution.Status = "completed"
	s.logger.Info("Workflow execution completed", "executionId", execution.ID)
}

// RegisterWithGRPCServer registers the MuleService with a gRPC server
func (s *Server) RegisterWithGRPCServer(grpcServer *grpc.Server) {
	pb.RegisterMuleServiceServer(grpcServer, s)
}
