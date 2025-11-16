package frontend

import (
	"embed"
	"io/fs"
	"net/http"
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

	return http.FileServer(http.FS(buildFS))
}
