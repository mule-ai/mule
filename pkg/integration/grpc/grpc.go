package grpc

import (
	"fmt"
	"net"

	"github.com/go-logr/logr"
	"github.com/jbutlerdev/genai"
	"github.com/mule-ai/mule/pkg/agent"
	"google.golang.org/grpc"
)

// Config represents the gRPC integration configuration
type Config struct {
	Enabled bool   `json:"enabled"`
	Port    int    `json:"port"`
	Host    string `json:"host"`
}

type GRPCInput struct {
	Config    *Config
	Agents    map[int]*agent.Agent
	Workflows map[string]*agent.Workflow
	Providers map[string]*genai.Provider
	Logger    logr.Logger
}

// Integration implements the Integration interface for gRPC
type GRPC struct {
	agents     map[int]*agent.Agent
	workflows  map[string]*agent.Workflow
	providers  map[string]*genai.Provider
	config     *Config
	logger     logr.Logger
	server     *grpc.Server
	muleServer *Server
	channel    chan any
}

// New creates a new gRPC integration
func New(input GRPCInput) *GRPC {
	config := input.Config
	if config == nil {
		config = &Config{
			Enabled: false,
			Port:    9090,
			Host:    "localhost",
		}
	}

	if input.Agents == nil {
		input.Agents = map[int]*agent.Agent{}
	}
	if input.Workflows == nil {
		input.Workflows = map[string]*agent.Workflow{}
	}
	if input.Providers == nil {
		input.Providers = map[string]*genai.Provider{}
	}

	muleServer := NewServer(input.Logger, input.Agents, input.Workflows, input.Providers)
	grpcServer := grpc.NewServer()
	muleServer.RegisterWithGRPCServer(grpcServer)

	grpc := &GRPC{
		agents:     input.Agents,
		config:     config,
		workflows:  input.Workflows,
		providers:  input.Providers,
		logger:     input.Logger,
		server:     grpcServer,
		muleServer: muleServer,
		channel:    make(chan any, 100),
	}

	err := grpc.startServer()
	if err != nil {
		input.Logger.Error(err, "Failed to start gRPC server")
		return nil
	}

	input.Logger.Info("GRPC integration initialized")
	return grpc
}

func (g *GRPC) SetSystemPointers(agents map[int]*agent.Agent, workflows map[string]*agent.Workflow, providers map[string]*genai.Provider) {
	g.muleServer.SetAgents(agents)
	g.muleServer.SetWorkflows(workflows)
	g.muleServer.SetProviders(providers)
}

// Call implements the Integration interface
func (g *GRPC) Call(name string, data any) (any, error) {
	g.logger.Info("Call method invoked", "name", name, "data", data)

	switch name {
	case "status":
		return g.getStatus()
	default:
		return nil, fmt.Errorf("unknown method: %s", name)
	}
}

// GetChannel implements the Integration interface
func (g *GRPC) GetChannel() chan any {
	return g.channel
}

// Name implements the Integration interface
func (g *GRPC) Name() string {
	return "grpc"
}

// RegisterTrigger implements the Integration interface
func (g *GRPC) RegisterTrigger(trigger string, data any, channel chan any) {
	g.logger.Info("RegisterTrigger called", "trigger", trigger, "data", data)
	// gRPC integration doesn't use triggers in the traditional sense
	// as it's a server-based integration, but we implement this for interface compliance
}

// GetChatHistory implements the Integration interface for chat memory
func (g *GRPC) GetChatHistory(channelID string, limit int) (string, error) {
	// gRPC integration doesn't maintain chat history
	return "", nil
}

// ClearChatHistory implements the Integration interface for chat memory
func (g *GRPC) ClearChatHistory(channelID string) error {
	// gRPC integration doesn't maintain chat history
	return nil
}

// startServer starts the gRPC server
func (g *GRPC) startServer() error {
	if !g.config.Enabled {
		return nil
	}

	address := fmt.Sprintf("%s:%d", g.config.Host, g.config.Port)

	listener, err := net.Listen("tcp", address)
	if err != nil {
		g.logger.Error(err, "Failed to listen on address", "address", address)
		return fmt.Errorf("failed to listen on %s: %w", address, err)
	}

	g.logger.Info("Starting gRPC server", "address", address)

	go func() {
		if err := g.server.Serve(listener); err != nil {
			g.logger.Error(err, "gRPC server failed")
			g.channel <- fmt.Sprintf("gRPC server error: %v", err)
		}
	}()

	return nil
}

// getStatus returns the current status of the gRPC server
func (g *GRPC) getStatus() (any, error) {
	status := map[string]interface{}{
		"enabled": g.config.Enabled,
		"host":    g.config.Host,
		"port":    g.config.Port,
	}
	return status, nil
}
