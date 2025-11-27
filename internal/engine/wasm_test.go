package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWASMExecutorURLFiltering(t *testing.T) {
	executor := NewWASMExecutor(nil)

	// Test default allowlist (should allow all URLs)
	assert.True(t, executor.isURLAllowed("http://example.com"))
	assert.True(t, executor.isURLAllowed("https://example.com"))
	assert.False(t, executor.isURLAllowed("ftp://example.com"))

	// Test custom allowlist
	executor.SetURLAllowList([]string{"https://api.example.com/", "https://secure.example.com/"})

	assert.True(t, executor.isURLAllowed("https://api.example.com/users"))
	assert.True(t, executor.isURLAllowed("https://secure.example.com/data"))
	assert.False(t, executor.isURLAllowed("http://example.com"))
	assert.False(t, executor.isURLAllowed("https://malicious.com"))
}

func TestWASMExecutorHTTPHostFunction(t *testing.T) {
	// This test would require a full integration test with a real WASM module
	// For now, we'll just test that the executor can be created
	executor := NewWASMExecutor(nil)
	assert.NotNil(t, executor)

	// Test that we can set URL allowlist
	executor.SetURLAllowList([]string{"https://example.com/"})

	// Test URL validation
	assert.True(t, executor.isURLAllowed("https://example.com/test"))
	assert.False(t, executor.isURLAllowed("https://malicious.com/test"))
}

func TestWASMExecutorMemoryFunctions(t *testing.T) {
	// TODO: Add tests for memory management functions
	// This would require mocking the wazero.Memory interface
}

func TestWASMExecutorNetworkFunctionality(t *testing.T) {
	executor := NewWASMExecutor(nil)

	// Test that we can set URL allowlist
	executor.SetURLAllowList([]string{"https://httpbin.org/"})

	// Test URL validation for network requests
	assert.True(t, executor.isURLAllowed("https://httpbin.org/get"))
	assert.False(t, executor.isURLAllowed("ftp://example.com"))

	// Test that the executor was created correctly
	assert.NotNil(t, executor)
}

func TestWASMExecutorHTTPRequestFunction(t *testing.T) {
	executor := NewWASMExecutor(nil)

	// Test that we can set URL allowlist
	executor.SetURLAllowList([]string{"https://httpbin.org/", "https://example.com/"})

	// Test URL validation for different HTTP methods
	assert.True(t, executor.isURLAllowed("https://httpbin.org/get"))
	assert.True(t, executor.isURLAllowed("https://httpbin.org/post"))
	assert.True(t, executor.isURLAllowed("https://example.com/api/users"))

	// Test disallowed URLs
	assert.False(t, executor.isURLAllowed("https://malicious.com"))
	assert.False(t, executor.isURLAllowed("ftp://example.com"))
}
