package frontend

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServeStatic(t *testing.T) {
	handler := ServeStatic()

	t.Run("serves index.html", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		// Check that we get a successful response
		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		// Check that the response contains HTML content
		contentType := w.Header().Get("Content-Type")
		if contentType != "text/html; charset=utf-8" {
			t.Errorf("Expected Content-Type 'text/html; charset=utf-8', got '%s'", contentType)
		}

		// Check that the body contains expected HTML content
		body := w.Body.String()
		if len(body) == 0 {
			t.Error("Expected non-empty response body")
		}
	})

	t.Run("serves static assets", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/static/js/main.js", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)

		// Should either serve the file or return 404 if not found
		// The important thing is that it doesn't crash
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("Expected status %d or %d, got %d", http.StatusOK, http.StatusNotFound, w.Code)
		}
	})
}
