package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/jbutlerdev/genai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/mule-ai/mule/api/proto"
	"github.com/mule-ai/mule/pkg/agent"
	"github.com/mule-ai/mule/pkg/log"
)

func setupTestServer() *Server {
	logger := log.NewStdoutLogger()
	return NewServer(logger, map[int]*agent.Agent{}, map[string]*agent.Workflow{}, map[string]*genai.Provider{})
}

func TestGetHeartbeat(t *testing.T) {
	server := setupTestServer()

	req := &pb.HeartbeatRequest{}
	resp, err := server.GetHeartbeat(context.Background(), req)

	require.NoError(t, err)
	assert.Equal(t, "healthy", resp.Status)
	assert.Equal(t, "1.0.0", resp.Version)
	assert.NotNil(t, resp.Timestamp)

	// Check that timestamp is recent (within last 5 seconds)
	timeDiff := time.Since(resp.Timestamp.AsTime())
	assert.True(t, timeDiff < 5*time.Second)
}

func TestListWorkflows(t *testing.T) {
	server := setupTestServer()

	req := &pb.ListWorkflowsRequest{}
	resp, err := server.ListWorkflows(context.Background(), req)

	require.NoError(t, err)
	assert.Empty(t, resp.Workflows) // Empty because setupTestServer provides no workflows
}

func TestGetWorkflow(t *testing.T) {
	server := setupTestServer()

	req := &pb.GetWorkflowRequest{Name: "test-workflow"}
	_, err := server.GetWorkflow(context.Background(), req)

	require.Error(t, err) // Should error because workflow doesn't exist
	assert.Contains(t, err.Error(), "workflow not found")
}

func TestGetWorkflowNotFound(t *testing.T) {
	server := setupTestServer()

	req := &pb.GetWorkflowRequest{Name: "non-existent-workflow"}
	_, err := server.GetWorkflow(context.Background(), req)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "workflow not found")
}

func TestListAgents(t *testing.T) {
	server := setupTestServer()

	req := &pb.ListAgentsRequest{}
	resp, err := server.ListAgents(context.Background(), req)

	require.NoError(t, err)
	assert.Empty(t, resp.Agents) // Empty because setupTestServer provides no agents
}

func TestGetAgent(t *testing.T) {
	server := setupTestServer()

	req := &pb.GetAgentRequest{Id: 10}
	_, err := server.GetAgent(context.Background(), req)

	require.Error(t, err) // Should error because agent doesn't exist
	assert.Contains(t, err.Error(), "agent not found")
}

func TestGetAgentNotFound(t *testing.T) {
	server := setupTestServer()

	req := &pb.GetAgentRequest{Id: 999}
	_, err := server.GetAgent(context.Background(), req)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent not found")
}

func TestListRunningWorkflows(t *testing.T) {
	server := setupTestServer()

	// Initially should be empty
	req := &pb.ListRunningWorkflowsRequest{}
	resp, err := server.ListRunningWorkflows(context.Background(), req)

	require.NoError(t, err)
	assert.Empty(t, resp.RunningWorkflows)
}

func TestExecuteWorkflow(t *testing.T) {
	server := setupTestServer()

	req := &pb.ExecuteWorkflowRequest{
		WorkflowName: "test-workflow",
		Prompt:       "Test prompt",
		Path:         "/test/path",
	}
	_, err := server.ExecuteWorkflow(context.Background(), req)

	require.Error(t, err) // Should error because workflow doesn't exist
	assert.Contains(t, err.Error(), "workflow not found")
}

func TestExecuteWorkflowNotFound(t *testing.T) {
	server := setupTestServer()

	req := &pb.ExecuteWorkflowRequest{
		WorkflowName: "non-existent-workflow",
		Prompt:       "Test prompt",
		Path:         "/test/path",
	}
	_, err := server.ExecuteWorkflow(context.Background(), req)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "workflow not found")
}

func TestConvertWorkflowToPB(t *testing.T) {
	server := setupTestServer()

	// Verify that no workflows exist in test setup
	assert.Empty(t, server.workflows)

	// Test conversion with empty workflow would require creating a mock workflow
	// For now, just verify the server has no workflows
}

func TestConvertAgentToPB(t *testing.T) {
	server := setupTestServer()

	// Create a test agent directly instead of relying on the state
	testAgent := agent.NewAgent(agent.AgentOptions{
		ID:             1,
		Name:           "test-agent",
		ProviderName:   "test-provider",
		Model:          "test-model",
		PromptTemplate: "Test prompt template",
		SystemPrompt:   "Test system prompt",
		Tools:          []string{},
		UDiffSettings: agent.UDiffSettings{
			Enabled: true,
		},
	})

	pbAgent := server.convertAgentToPB(testAgent)

	assert.Equal(t, int32(1), pbAgent.Id)
	assert.Equal(t, "test-agent", pbAgent.Name)
	assert.Equal(t, "test-provider", pbAgent.ProviderName)
	assert.Equal(t, "test-model", pbAgent.Model)
	assert.Equal(t, "Test prompt template", pbAgent.PromptTemplate)
	assert.Equal(t, "Test system prompt", pbAgent.SystemPrompt)
	assert.Empty(t, pbAgent.Tools)
	assert.True(t, pbAgent.UdiffSettings.Enabled)
}
