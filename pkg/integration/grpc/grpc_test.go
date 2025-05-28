package grpc

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mule-ai/mule/pkg/log"
)

func TestNew(t *testing.T) {
	logger := log.NewStdoutLogger()

	// Test with nil config
	integration := New(GRPCInput{
		Logger: logger,
	})
	assert.NotNil(t, integration)
	assert.Equal(t, "grpc", integration.Name())
	assert.NotNil(t, integration.GetChannel())
	assert.False(t, integration.config.Enabled)
	assert.Equal(t, 9090, integration.config.Port)
	assert.Equal(t, "localhost", integration.config.Host)

	// Test with custom config
	customConfig := &Config{
		Enabled: false,
		Port:    8080,
		Host:    "0.0.0.0",
	}

	integration2 := New(GRPCInput{
		Config: customConfig,
		Logger: logger,
	})
	assert.NotNil(t, integration2)
	assert.False(t, integration2.config.Enabled)
	assert.Equal(t, 8080, integration2.config.Port)
	assert.Equal(t, "0.0.0.0", integration2.config.Host)
}

func TestIntegrationInterface(t *testing.T) {
	logger := log.NewStdoutLogger()
	config := &Config{
		Enabled: false,
		Port:    9090,
		Host:    "localhost",
	}

	integration := New(GRPCInput{
		Config: config,
		Logger: logger,
	})

	// Test Name method
	assert.Equal(t, "grpc", integration.Name())

	// Test GetChannel method
	channel := integration.GetChannel()
	assert.NotNil(t, channel)

	// Test RegisterTrigger method (should not panic)
	integration.RegisterTrigger("test-trigger", "test-data", make(chan any))

	// Test GetChatHistory method
	history, err := integration.GetChatHistory("test-channel", 10)
	assert.NoError(t, err)
	assert.Empty(t, history)

	// Test ClearChatHistory method
	err = integration.ClearChatHistory("test-channel")
	assert.NoError(t, err)
}

func TestCallMethods(t *testing.T) {
	logger := log.NewStdoutLogger()
	config := &Config{
		Enabled: false,
		Port:    9090,
		Host:    "localhost",
	}

	integration := New(GRPCInput{
		Config: config,
		Logger: logger,
	})

	// Test status call
	result, err := integration.Call("status", nil)
	require.NoError(t, err)

	status, ok := result.(map[string]interface{})
	require.True(t, ok)
	assert.False(t, status["enabled"].(bool))
	assert.Equal(t, "localhost", status["host"].(string))
	assert.Equal(t, 9090, status["port"].(int))

	// Test unknown method
	_, err = integration.Call("unknown", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown method")
}
