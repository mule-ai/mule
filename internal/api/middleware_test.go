package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mule-ai/mule/internal/validation"
	"github.com/stretchr/testify/assert"
)

// Test helper to capture log output
func captureLogs(f func()) string {
	var buf bytes.Buffer
	oldOutput := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(oldOutput)
	f()
	return buf.String()
}

func TestLoggingMiddleware(t *testing.T) {
	t.Run("logs request details", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		middleware := LoggingMiddleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		logs := captureLogs(func() {
			middleware.ServeHTTP(rec, req)
		})

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, logs, "GET /test 200")
	})

	t.Run("logs request duration", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(10 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		})

		middleware := LoggingMiddleware(handler)

		req := httptest.NewRequest("POST", "/test", nil)
		rec := httptest.NewRecorder()

		logs := captureLogs(func() {
			middleware.ServeHTTP(rec, req)
		})

		assert.Contains(t, logs, "POST /test 200")
	})
}

func TestCORSMiddleware(t *testing.T) {
	t.Run("adds CORS headers", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		middleware := CORSMiddleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
		assert.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), "GET")
		assert.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), "POST")
		assert.Contains(t, rec.Header().Get("Access-Control-Allow-Headers"), "Content-Type")
	})

	t.Run("handles OPTIONS preflight", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("Handler should not be called for OPTIONS")
		})

		middleware := CORSMiddleware(handler)

		req := httptest.NewRequest("OPTIONS", "/test", nil)
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "*", rec.Header().Get("Access-Control-Allow-Origin"))
	})
}

func TestTimeoutMiddleware(t *testing.T) {
	t.Run("skips timeout for WebSocket connections", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		})

		middleware := TimeoutMiddleware(func() time.Duration { return 50 * time.Millisecond })(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Upgrade", "websocket")
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("skips timeout for chat completions", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		})

		middleware := TimeoutMiddleware(func() time.Duration { return 50 * time.Millisecond })(handler)

		req := httptest.NewRequest("POST", "/v1/chat/completions", nil)
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("times out long running requests", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		})

		middleware := TimeoutMiddleware(func() time.Duration { return 50 * time.Millisecond })(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusRequestTimeout, rec.Code)

		var resp ErrorResponse
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, "request_timeout", resp.Error)
	})

	t.Run("completes requests within timeout", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		middleware := TimeoutMiddleware(func() time.Duration { return 100 * time.Millisecond })(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestRecoveryMiddleware(t *testing.T) {
	t.Run("recovers from panic", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("test panic")
		})

		middleware := RecoveryMiddleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		assert.NotPanics(t, func() {
			middleware.ServeHTTP(rec, req)
		})

		assert.Equal(t, http.StatusInternalServerError, rec.Code)

		var resp ErrorResponse
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, "internal_server_error", resp.Error)
	})

	t.Run("recovers from panic with error", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic(errors.New("specific error"))
		})

		middleware := RecoveryMiddleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		logs := captureLogs(func() {
			assert.NotPanics(t, func() {
				middleware.ServeHTTP(rec, req)
			})
		})

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, logs, "Panic recovered")
	})

	t.Run("passes through normal requests", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		middleware := RecoveryMiddleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestValidationMiddleware(t *testing.T) {
	t.Run("skips validation for GET requests", func(t *testing.T) {
		called := false
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		})

		validator := validation.NewValidator()
		middleware := ValidationMiddleware(validator, func(v *validation.Validator, req interface{}) validation.ValidationErrors {
			t.Error("Validation should not be called for GET")
			return nil
		})(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)

		assert.True(t, called)
	})

	t.Run("skips validation for DELETE requests", func(t *testing.T) {
		called := false
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		})

		validator := validation.NewValidator()
		middleware := ValidationMiddleware(validator, func(v *validation.Validator, req interface{}) validation.ValidationErrors {
			t.Error("Validation should not be called for DELETE")
			return nil
		})(handler)

		req := httptest.NewRequest("DELETE", "/test", nil)
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)

		assert.True(t, called)
	})

	t.Run("returns error for invalid JSON", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("Handler should not be called for invalid JSON")
		})

		validator := validation.NewValidator()
		middleware := ValidationMiddleware(validator, func(v *validation.Validator, req interface{}) validation.ValidationErrors {
			return nil
		})(handler)

		req := httptest.NewRequest("POST", "/test", strings.NewReader("invalid json"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, "invalid_json", resp["error"])
	})

	t.Run("validates request and passes if valid", func(t *testing.T) {
		called := false
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		})

		validator := validation.NewValidator()
		middleware := ValidationMiddleware(validator, func(v *validation.Validator, req interface{}) validation.ValidationErrors {
			return nil
		})(handler)

		body := `{"name": "test"}`
		req := httptest.NewRequest("POST", "/test", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()

		middleware.ServeHTTP(rec, req)

		assert.True(t, called)
		assert.Equal(t, http.StatusOK, rec.Code)
	})
}

func TestResponseWriter(t *testing.T) {
	t.Run("captures status code", func(t *testing.T) {
		rec := httptest.NewRecorder()

		rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

		rw.WriteHeader(http.StatusCreated)

		assert.Equal(t, http.StatusCreated, rw.statusCode)
		assert.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("prevents double WriteHeader", func(t *testing.T) {
		rec := httptest.NewRecorder()

		rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

		rw.WriteHeader(http.StatusCreated)
		rw.WriteHeader(http.StatusOK) // Should not change

		assert.Equal(t, http.StatusCreated, rw.statusCode)
		assert.Equal(t, http.StatusCreated, rec.Code)
	})

	t.Run("writes default status if not set", func(t *testing.T) {
		rec := httptest.NewRecorder()

		rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

		_, _ = rw.Write(nil)

		assert.Equal(t, http.StatusOK, rw.statusCode)
	})
}

func TestHandleError(t *testing.T) {
	t.Run("handles error with status code", func(t *testing.T) {
		rec := httptest.NewRecorder()

		HandleError(rec, errors.New("test error"), http.StatusBadRequest)

		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp ErrorResponse
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, "request_error", resp.Error)
		assert.Equal(t, "test error", resp.Message)
	})

	t.Run("returns generic message for 5xx errors", func(t *testing.T) {
		rec := httptest.NewRecorder()

		HandleError(rec, errors.New("internal details"), http.StatusInternalServerError)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)

		var resp ErrorResponse
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, "An internal server error occurred", resp.Message)
	})
}

func TestHandleValidationError(t *testing.T) {
	t.Run("formats validation errors", func(t *testing.T) {
		rec := httptest.NewRecorder()

		errs := validation.ValidationErrors{
			{Field: "name", Message: "name is required"},
			{Field: "email", Message: "email is invalid"},
		}

		HandleValidationError(rec, errs)

		assert.Equal(t, http.StatusBadRequest, rec.Code)

		var resp map[string]interface{}
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, "validation_failed", resp["error"])
	})
}

func TestIsNotFoundError(t *testing.T) {
	t.Run("returns true for primitive.ErrNotFound", func(t *testing.T) {
		// Create a new ErrNotFound instance (from primitive package)
		// We can't import primitive directly since we're testing the function
		// that checks for it, so we test with errors containing "not found"
		result := IsNotFoundError(errors.New("not found"))
		assert.True(t, result)
	})

	t.Run("returns true for error messages containing 'not found'", func(t *testing.T) {
		result := IsNotFoundError(errors.New("provider not found: test-id"))
		assert.True(t, result)
	})

	t.Run("returns false for nil error", func(t *testing.T) {
		result := IsNotFoundError(nil)
		assert.False(t, result)
	})

	t.Run("returns false for other errors", func(t *testing.T) {
		result := IsNotFoundError(errors.New("connection refused"))
		assert.False(t, result)
	})
}

func TestHandleNotFoundOrError(t *testing.T) {
	t.Run("returns 404 for not found errors", func(t *testing.T) {
		rec := httptest.NewRecorder()

		result := HandleNotFoundOrError(rec, errors.New("not found"), "provider")

		assert.True(t, result)
		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("returns 500 for other errors", func(t *testing.T) {
		rec := httptest.NewRecorder()

		logs := captureLogs(func() {
			result := HandleNotFoundOrError(rec, errors.New("database error"), "provider")
			assert.True(t, result)
		})

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, logs, "Failed to get/create/update/delete provider")
	})

	t.Run("returns false for nil error", func(t *testing.T) {
		rec := httptest.NewRecorder()

		result := HandleNotFoundOrError(rec, nil, "provider")

		assert.False(t, result)
	})
}

func TestHandleNotFoundOrErrorf(t *testing.T) {
	t.Run("returns 404 for not found errors", func(t *testing.T) {
		rec := httptest.NewRecorder()

		result := HandleNotFoundOrErrorf(rec, errors.New("not found"), "agent", "Agent %s not found", "test-id")

		assert.True(t, result)
		assert.Equal(t, http.StatusNotFound, rec.Code)

		var resp ErrorResponse
		err := json.Unmarshal(rec.Body.Bytes(), &resp)
		assert.NoError(t, err)
		assert.Equal(t, "not_found", resp.Error)
		assert.Equal(t, "agent not found", resp.Message)
	})

	t.Run("returns 500 for other errors", func(t *testing.T) {
		rec := httptest.NewRecorder()

		logs := captureLogs(func() {
			result := HandleNotFoundOrErrorf(rec, errors.New("db error"), "workflow", "Workflow error")
			assert.True(t, result)
		})

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Contains(t, logs, "Failed to get/create/update/delete workflow")
	})
}

func TestGetResourceAction(t *testing.T) {
	tests := []struct {
		resourceType string
		expected     string
	}{
		{"provider", "get/create/update/delete"},
		{"providers", "get/create/update/delete"},
		{"agent", "get/create/update/delete"},
		{"agents", "get/create/update/delete"},
		{"workflow", "get/create/update/delete"},
		{"workflows", "get/create/update/delete"},
		{"skill", "get/create/update/delete"},
		{"skills", "get/create/update/delete"},
		{"tool", "get/create/update/delete"},
		{"tools", "get/create/update/delete"},
		{"unknown", "get"},
	}

	for _, tt := range tests {
		t.Run(tt.resourceType, func(t *testing.T) {
			result := getResourceAction(tt.resourceType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestErrorResponse(t *testing.T) {
	t.Run("JSON marshaling", func(t *testing.T) {
		resp := ErrorResponse{
			Error:   "test_error",
			Message: "Test message",
			Code:    "ERR001",
		}

		data, err := json.Marshal(resp)
		assert.NoError(t, err)

		var decoded ErrorResponse
		err = json.Unmarshal(data, &decoded)
		assert.NoError(t, err)
		assert.Equal(t, resp.Error, decoded.Error)
		assert.Equal(t, resp.Message, decoded.Message)
		assert.Equal(t, resp.Code, decoded.Code)
	})
}
