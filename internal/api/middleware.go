package api

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

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
			timeout := getTimeoutFunc()
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			r = r.WithContext(ctx)

			// Create a channel to signal when the handler is done
			done := make(chan struct{})
			panicChan := make(chan interface{}, 1)

			// Wrap the response writer to track if headers were written
			rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			go func() {
				defer func() {
					if p := recover(); p != nil {
						panicChan <- p
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
				if rw.headerWritten {
					// Headers already written - we can't change the status code
					// The client will receive an incomplete response, but we can't prevent it
					log.Printf("Request timeout after %v, but headers already written (status: %d)", timeout, rw.statusCode)
					return
				}

				// Headers not written yet, we can send a timeout response
				w.WriteHeader(http.StatusRequestTimeout)
				_ = json.NewEncoder(w).Encode(ErrorResponse{
					Error:   "request_timeout",
					Message: "Request took too long to process",
				})
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
				_ = json.NewEncoder(w).Encode(ErrorResponse{
					Error:   "internal_server_error",
					Message: "An unexpected error occurred",
				})
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
				_ = json.NewEncoder(w).Encode(ErrorResponse{
					Error:   "invalid_json",
					Message: "Invalid JSON in request body",
				})
				return
			}

			if errors := validationFunc(validator, request); len(errors) > 0 {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"error":   "validation_failed",
					"message": "Request validation failed",
					"details": errors,
				})
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
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"error":   "validation_failed",
		"message": "Request validation failed",
		"details": errors,
	})
}
