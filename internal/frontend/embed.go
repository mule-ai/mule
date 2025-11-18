package frontend

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed build
//go:embed build/*
var staticFiles embed.FS

// ServeStatic serves the embedded React frontend
func ServeStatic() http.Handler {
	// Get the subdirectory from the embedded filesystem
	buildFS, err := fs.Sub(staticFiles, "build")
	if err != nil {
		// If build directory doesn't exist, serve a simple fallback
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`
<!DOCTYPE html>
<html>
<head>
    <title>Mule v2 - Frontend Not Built</title>
</head>
<body>
    <h1>Mule v2 API Server</h1>
    <p>The React frontend has not been built yet.</p>
    <p>To build the frontend:</p>
    <pre>
cd frontend
npm install
npm run build
    </pre>
    <p>API endpoints are available at:</p>
    <ul>
        <li><a href="/health">Health Check</a></li>
        <li><a href="/v1/models">Models</a></li>
        <li><a href="/api/v1/providers">Providers</a></li>
        <li><a href="/api/v1/agents">Agents</a></li>
        <li><a href="/api/v1/workflows">Workflows</a></li>
        <li><a href="/api/v1/jobs">Jobs</a></li>
    </ul>
</body>
</html>
			`))
		})
	}

	// Create a file server that serves static files
	fileServer := http.FileServer(http.FS(buildFS))

	// Return a custom handler that serves index.html for non-static file routes
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to open the requested file
		path := r.URL.Path
		if path == "/" {
			path = "/index.html"
		} else {
			// For SPA, we need to check if the file exists
			// If it doesn't exist, serve index.html
			path = strings.TrimPrefix(path, "/")
		}

		// Try to open the file
		file, err := buildFS.Open(path)
		if err != nil {
			// If file doesn't exist, serve index.html for SPA routing
			file, err = buildFS.Open("index.html")
			if err != nil {
				http.Error(w, "Failed to open index.html", http.StatusInternalServerError)
				return
			}
			defer file.Close()

			// Serve index.html
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = io.Copy(w, file)
			return
		}
		defer file.Close()

		// File exists, serve it normally
		fileServer.ServeHTTP(w, r)
	})
}
