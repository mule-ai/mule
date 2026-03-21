package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/internal/validation"
)

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
	Code    string `json:"code,omitempty"`
}

// LoggingMiddleware logs HTTP requests
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Create a response writer wrapper to capture status code
		wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapped, r)

		duration := time.Since(start)
		log.Printf("%s %s %d %v", r.Method, r.URL.Path, wrapped.statusCode, duration)
	})
}

// CORSMiddleware adds CORS headers
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// TimeoutMiddleware adds a timeout to requests with configurable duration
func TimeoutMiddleware(getTimeoutFunc func() time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip timeout for WebSocket connections
			if r.Header.Get("Upgrade") == "websocket" {
				next.ServeHTTP(w, r)
				return
			}

			// Skip timeout for chat completions - these can be long-running
			// and are handled by the workflow engine with its own timeout
			if r.URL.Path == "/v1/chat/completions" {
				next.ServeHTTP(w, r)
				return
			}

			timeout := getTimeoutFunc()
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			r = r.WithContext(ctx)

			// Create a channel to signal when the handler is done
			done := make(chan struct{})
			// Use buffered channel to prevent goroutine leak if we return early
			// and the handler panics after the select has completed
			panicChan := make(chan interface{}, 1)

			// Wrap the response writer to track if headers were written
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			go func() {
				defer func() {
					if p := recover(); p != nil {
						// Use non-blocking send to prevent goroutine leak
						// if nobody is listening (e.g., after timeout case)
						select {
						case panicChan <- p:
						default:
							log.Printf("Handler panic (channel full, already returned): %v", p)
						}
					}
					close(done)
				}()
				next.ServeHTTP(rw, r)
			}()

			select {
			case <-done:
				// Request completed normally
				return
			case p := <-panicChan:
				// Panic occurred in handler
				panic(p)
			case <-ctx.Done():
				// Request timed out
				// Wait for the handler goroutine to complete to prevent goroutine leak
				// Use a short timeout to avoid blocking forever if the handler is stuck
				go func() {
					select {
					case <-done:
						// Handler finished
					case <-time.After(5 * time.Second):
						// Handler still running after 5 seconds, log warning
						log.Printf("Handler goroutine still running after timeout (5s), possible resource leak")
					}
				}()

				if rw.headerWritten {
					// Headers already written - we can't change the status code
					// The client will receive an incomplete response, but we can't prevent it
					log.Printf("Request timeout after %v, but headers already written (status: %d)", timeout, rw.statusCode)
					return
				}

				// Headers not written yet, we can send a timeout response
				w.WriteHeader(http.StatusRequestTimeout)
				if err := json.NewEncoder(w).Encode(ErrorResponse{
					Error:   "request_timeout",
					Message: "Request took too long to process",
				}); err != nil {
					log.Printf("Warning: failed to encode timeout response: %v", err)
				}
				return
			}
		})
	}
}

// RecoveryMiddleware recovers from panics
func RecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("Panic recovered: %v", err)

				// Check if headers have already been written
				if rw, ok := w.(*responseWriter); ok && rw.headerWritten {
					// Headers already written, can't send error response
					return
				}

				w.WriteHeader(http.StatusInternalServerError)
				if encodeErr := json.NewEncoder(w).Encode(ErrorResponse{
					Error:   "internal_server_error",
					Message: "An unexpected error occurred",
				}); encodeErr != nil {
					log.Printf("Warning: failed to encode panic response: %v", encodeErr)
				}
			}
		}()

		next.ServeHTTP(w, r)
	})
}

// ValidationMiddleware validates requests
func ValidationMiddleware(validator *validation.Validator, validationFunc func(*validation.Validator, interface{}) validation.ValidationErrors) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "GET" || r.Method == "DELETE" {
				next.ServeHTTP(w, r)
				return
			}

			var request interface{}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				if encodeErr := json.NewEncoder(w).Encode(ErrorResponse{
					Error:   "invalid_json",
					Message: "Invalid JSON in request body",
				}); encodeErr != nil {
					log.Printf("Warning: failed to encode validation error response: %v", encodeErr)
				}
				return
			}

			if errors := validationFunc(validator, request); len(errors) > 0 {
				w.WriteHeader(http.StatusBadRequest)
				if encodeErr := json.NewEncoder(w).Encode(map[string]interface{}{
					"error":   "validation_failed",
					"message": "Request validation failed",
					"details": errors,
				}); encodeErr != nil {
					log.Printf("Warning: failed to encode validation error response: %v", encodeErr)
				}
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// responseWriter is a wrapper around http.ResponseWriter that captures the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode    int
	headerWritten bool
}

// Check if the underlying ResponseWriter implements http.Hijacker
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	// Check if the underlying ResponseWriter implements Hijacker
	hj, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}

	// Delegate to the underlying Hijacker
	conn, buf, err := hj.Hijack()
	if err != nil {
		return nil, nil, err
	}

	// Mark that headers have been written since we're hijacking the connection
	rw.headerWritten = true

	return conn, buf, nil
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.headerWritten {
		return
	}
	rw.headerWritten = true
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.headerWritten {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

// HandleError handles errors in a consistent way
func HandleError(w http.ResponseWriter, err error, statusCode int) {
	log.Printf("Error: %v", err)

	// Check if headers have already been written
	if rw, ok := w.(*responseWriter); ok && rw.headerWritten {
		// Headers already written, can't send error response
		return
	}

	w.WriteHeader(statusCode)
	response := ErrorResponse{
		Error: "request_error",
	}

	if statusCode >= 500 {
		response.Message = "An internal server error occurred"
	} else {
		response.Message = err.Error()
	}

	_ = json.NewEncoder(w).Encode(response)
}

// HandleValidationError handles validation errors
func HandleValidationError(w http.ResponseWriter, errors validation.ValidationErrors) {
	// Check if headers have already been written
	if rw, ok := w.(*responseWriter); ok && rw.headerWritten {
		// Headers already written, can't send error response
		return
	}

	w.WriteHeader(http.StatusBadRequest)
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   "validation_failed",
		"message": "Request validation failed",
		"details": errors,
	}); err != nil {
		log.Printf("Warning: failed to encode validation error response: %v", err)
	}
}

// IsNotFoundError checks if an error is a "not found" error.
// It checks both primitive.ErrNotFound and error messages containing "not found".
func IsNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	// Check for the standard not found error
	if err == primitive.ErrNotFound {
		return true
	}
	// Check for common "not found" patterns in error messages
	// This handles errors returned from managers that use fmt.Errorf("... not found: ...")
	return strings.Contains(err.Error(), "not found")
}

// HandleNotFoundOrError handles errors by returning 404 for not found errors
// and 500 for other errors. Returns true if the error was handled, false otherwise.
func HandleNotFoundOrError(w http.ResponseWriter, err error, resourceType string) bool {
	if err == nil {
		return false
	}

	if rw, ok := w.(*responseWriter); ok && rw.headerWritten {
		// Headers already written, can't send error response
		return true
	}

	if IsNotFoundError(err) {
		HandleError(w, err, http.StatusNotFound)
		return true
	}

	// Log the error with context for internal errors
	log.Printf("Failed to %s %s: %v", getResourceAction(resourceType), resourceType, err)
	HandleError(w, err, http.StatusInternalServerError)
	return true
}

// HandleNotFoundOrErrorf is like HandleNotFoundOrError but allows formatting the error message.
func HandleNotFoundOrErrorf(w http.ResponseWriter, err error, resourceType string, format string, args ...interface{}) bool {
	if err == nil {
		return false
	}

	if rw, ok := w.(*responseWriter); ok && rw.headerWritten {
		// Headers already written, can't send error response
		return true
	}

	if IsNotFoundError(err) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(ErrorResponse{
			Error:   "not_found",
			Message: fmt.Sprintf("%s not found", resourceType),
		})
		return true
	}

	// Log the error with context for internal errors
	log.Printf("Failed to %s %s: %v", getResourceAction(resourceType), resourceType, err)
	HandleError(w, err, http.StatusInternalServerError)
	return true
}

// getResourceAction returns the appropriate action verb for a resource type
func getResourceAction(resourceType string) string {
	switch strings.ToLower(resourceType) {
	case "provider", "providers":
		return "get/create/update/delete"
	case "agent", "agents":
		return "get/create/update/delete"
	case "workflow", "workflows":
		return "get/create/update/delete"
	case "skill", "skills":
		return "get/create/update/delete"
	case "tool", "tools":
		return "get/create/update/delete"
	default:
		return "get"
	}
}
