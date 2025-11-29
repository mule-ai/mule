package engine

import (
	"context"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// TestBufferSizeQuery tests that the buffer size query functionality works correctly
// This verifies that when bufferSize is 0, the functions return the required buffer size
// instead of an error code
func TestBufferSizeQuery(t *testing.T) {
	// Create a mock WASM executor
	executor := &WASMExecutor{
		lastOperationResult: map[string][]byte{
			"test": []byte("test result data"),
		},
		lastResponseBody: map[string][]byte{
			"test": []byte("test response body"),
		},
		lastResponse: map[string]*ResponseMock{
			"test": &ResponseMock{},
		},
	}

	// Create a mock module
	ctx := context.Background()
	runtime := wazero.NewRuntime(ctx)
	defer runtime.Close(ctx)

	// Test get_last_operation_result with bufferSize=0
	t.Run("get_last_operation_result buffer size query", func(t *testing.T) {
		// Mock module that simulates the WASM module calling our host function
		mockModule := &MockModule{
			key: "test",
		}

		// Call the function with bufferSize=0 to query for required size
		result := executor.get_last_operation_result_func(ctx, mockModule, 0, 0)

		// Should return the length of the result data (15 bytes) instead of an error code
		expectedSize := uint32(len("test result data"))
		if result != expectedSize {
			t.Errorf("Expected buffer size %d, got %d", expectedSize, result)
		}

		// Verify it's not an error code (error codes are >= 0xFFFFFFF0)
		if result >= 0xFFFFFFF0 {
			t.Errorf("Returned value is an error code: %d", result)
		}
	})

	// Test get_last_response_body with bufferSize=0
	t.Run("get_last_response_body buffer size query", func(t *testing.T) {
		// Mock module that simulates the WASM module calling our host function
		mockModule := &MockModule{
			key: "test",
		}

		// Call the function with bufferSize=0 to query for required size
		result := executor.get_last_response_body_func(ctx, mockModule, 0, 0)

		// Should return the length of the response body (17 bytes) instead of an error code
		expectedSize := uint32(len("test response body"))
		if result != expectedSize {
			t.Errorf("Expected buffer size %d, got %d", expectedSize, result)
		}

		// Verify it's not an error code (error codes are >= 0xFFFFFFF0)
		if result >= 0xFFFFFFF0 {
			t.Errorf("Returned value is an error code: %d", result)
		}
	})
}

// MockModule implements api.Module for testing
type MockModule struct {
	key string
}

func (m *MockModule) Name() string {
	return "test"
}

func (m *MockModule) Memory() api.Memory {
	return &MockMemory{}
}

func (m *MockModule) ExportedFunction(name string) api.Function {
	return nil
}

func (m *MockModule) Close(context.Context) error {
	return nil
}

func (m *MockModule) Closer() api.Closer {
	return m
}

// MockMemory implements api.Memory for testing
type MockMemory struct{}

func (m *MockMemory) Size() uint32 {
	return 1024
}

func (m *MockMemory) Grow(delta uint32) bool {
	return true
}

func (m *MockMemory) Read(offset, size uint32) ([]byte, bool) {
	return make([]byte, size), true
}

func (m *MockMemory) Write(offset uint32, data []byte) bool {
	return true
}

// ResponseMock implements a mock HTTP response for testing
type ResponseMock struct {
	StatusCode int
	HeaderMock map[string][]string
}

func (r *ResponseMock) StatusCodeMethod() int {
	if r.StatusCode == 0 {
		return 200
	}
	return r.StatusCode
}

func (r *ResponseMock) Header() map[string][]string {
	if r.HeaderMock == nil {
		return make(map[string][]string)
	}
	return r.HeaderMock
}