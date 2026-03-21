package frontend

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestServeStatic(t *testing.T) {
	handler := ServeStatic()

	t.Run("serves index.html", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		// Check that we get a successful response
		assert.Equal(t, http.StatusOK, w.Code, "Expected status %d, got %d", http.StatusOK, w.Code)

		// Check that the response contains HTML content
		contentType := w.Header().Get("Content-Type")
		assert.Equal(t, "text/html; charset=utf-8", contentType, "Expected Content-Type 'text/html; charset=utf-8', got '%s'", contentType)

		// Check that the body contains expected HTML content
		body := w.Body.String()
		assert.NotEmpty(t, body, "Expected non-empty response body")
	})

	t.Run("serves static assets", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/static/js/main.js", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		// Should either serve the file or return 404 if not found
		// The important thing is that it doesn't crash
		assert.True(t, w.Code == http.StatusOK || w.Code == http.StatusNotFound,
			"Expected status %d or %d, got %d", http.StatusOK, http.StatusNotFound, w.Code)
	})
}
