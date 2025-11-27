package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	_ "github.com/lib/pq"

	"github.com/mule-ai/mule/internal/api"
	"github.com/mule-ai/mule/internal/database"
	"github.com/mule-ai/mule/internal/frontend"
	"github.com/mule-ai/mule/internal/initialization"
	"github.com/mule-ai/mule/internal/manager"
	"github.com/mule-ai/mule/pkg/job"
)

// parseDBConfig parses a PostgreSQL connection string into a database.Config
func parseDBConfig(connStr string) (database.Config, error) {
	var config database.Config

	u, err := url.Parse(connStr)
	if err != nil {
		return config, fmt.Errorf("failed to parse connection string: %w", err)
	}

	// Extract host and port
	host := u.Hostname()
	portStr := u.Port()
	if portStr == "" {
		portStr = "5432" // default PostgreSQL port
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return config, fmt.Errorf("invalid port: %w", err)
	}

	// Extract username and password
	username := u.User.Username()
	password, _ := u.User.Password()

	// Extract database name from path
	dbName := u.Path
	if len(dbName) > 0 && dbName[0] == '/' {
		dbName = dbName[1:]
	}

	// Extract SSL mode from query parameters
	sslMode := "disable" // default
	query := u.Query()
	if query.Get("sslmode") != "" {
		sslMode = query.Get("sslmode")
	}

	config = database.Config{
		Host:     host,
		Port:     port,
		User:     username,
		Password: password,
		DBName:   dbName,
		SSLMode:  sslMode,
	}

	return config, nil
}

func main() {
	var (
		dbConnStr  string
		listenAddr string
	)

	flag.StringVar(&dbConnStr, "db", "postgres://user:pass@localhost:5432/mulev2?sslmode=disable", "PostgreSQL connection string")
	flag.StringVar(&listenAddr, "listen", ":8080", "HTTP listen address")
	flag.Parse()

	// Parse the connection string to create database config
	config, err := parseDBConfig(dbConnStr)
	if err != nil {
		log.Fatalf("failed to parse database connection string: %v", err)
	}

	// Create database connection using the database package
	db, err := database.NewDB(config)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}

	// Initialize database schema
	if err := db.InitSchema(); err != nil {
		log.Fatalf("failed to initialize database schema: %v", err)
	}
	log.Println("Database schema initialized successfully")

	// Ensure default primitives exist
	// Generate a simple secret for encryption (in production, this should be from a secure source)
	secret := []byte("mule-default-secret-key-12345678")
	providerMgr := manager.NewProviderManager(db, secret)
	agentMgr := manager.NewAgentManager(db)
	workflowMgr := manager.NewWorkflowManager(db)

	initCtx := context.Background()
	defaults, err := initialization.EnsureAllDefaults(initCtx, providerMgr, agentMgr, workflowMgr)
	if err != nil {
		log.Printf("Warning: failed to ensure default primitives: %v", err)
	} else {
		log.Printf("Default primitives ensured: provider=%s, default_agent=%s, wasm_editor_agent=%s, workflow=%s",
			defaults.Provider.ID, defaults.DefaultAgent.ID, defaults.WasmEditorAgent.ID, defaults.Workflow.ID)
	}

	router := mux.NewRouter()

	// Apply middleware
	router.Use(api.LoggingMiddleware)
	router.Use(api.RecoveryMiddleware)
	router.Use(api.CORSMiddleware)

	// Create a function to get timeout from database
	getTimeoutFunc := func() time.Duration {
		// Default workflow timeout of 5 minutes
		defaultWorkflowTimeout := 5 * time.Minute
		// Default middleware timeout is workflow timeout + 1 minute buffer
		defaultTimeout := defaultWorkflowTimeout + 1*time.Minute

		// Try to get workflow timeout from database first
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		workflowSetting, err := db.GetSetting(ctx, "timeout_workflow_seconds")
		if err != nil {
			// Return default if we can't get the workflow timeout
			return defaultTimeout
		}

		// Parse the workflow timeout value
		workflowTimeoutSeconds, err := strconv.Atoi(workflowSetting.Value)
		if err != nil || workflowTimeoutSeconds <= 0 {
			return defaultTimeout
		}

		workflowTimeout := time.Duration(workflowTimeoutSeconds) * time.Second

		// Now check if there's an explicit request timeout setting
		requestSetting, err := db.GetSetting(ctx, "timeout_request_seconds")
		if err == nil {
			if requestTimeoutSeconds, err := strconv.Atoi(requestSetting.Value); err == nil && requestTimeoutSeconds > 0 {
				return time.Duration(requestTimeoutSeconds) * time.Second
			}
		}

		// No explicit request timeout, calculate it as workflow timeout + 1 minute buffer
		return workflowTimeout + 1*time.Minute
	}

	router.Use(api.TimeoutMiddleware(getTimeoutFunc))

	// Health check endpoint
	router.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	handler := NewAPIHandler(db)

	// Initialize WebSocket hub
	wsHub := api.NewWebSocketHub()
	go wsHub.Run()

	// Initialize job streamer
	jobStore := job.NewPGStore(db.DB) // Access the underlying *sql.DB
	jobStreamer := api.NewJobStreamer(wsHub, jobStore)
	jobStreamer.Start()
	defer jobStreamer.Stop()

	// Start the workflow engine
	ctx := context.Background()
	if err := handler.workflowEngine.Start(ctx); err != nil {
		log.Fatalf("Failed to start workflow engine: %v", err)
	}
	defer handler.workflowEngine.Stop()

	router.HandleFunc("/v1/models", handler.modelsHandler).Methods("GET")
	router.HandleFunc("/v1/chat/completions", handler.chatCompletionsHandler).Methods("POST")

	// Primitive management APIs
	router.HandleFunc("/api/v1/providers", handler.listProvidersHandler).Methods("GET")
	router.HandleFunc("/api/v1/providers", handler.createProviderHandler).Methods("POST")
	router.HandleFunc("/api/v1/providers/{id}", handler.getProviderHandler).Methods("GET")
	router.HandleFunc("/api/v1/providers/{id}", handler.updateProviderHandler).Methods("PUT")
	router.HandleFunc("/api/v1/providers/{id}", handler.deleteProviderHandler).Methods("DELETE")
	router.HandleFunc("/api/v1/providers/{id}/models", handler.getProviderModelsHandler).Methods("GET")

	router.HandleFunc("/api/v1/tools", handler.listToolsHandler).Methods("GET")
	router.HandleFunc("/api/v1/tools", handler.createToolHandler).Methods("POST")
	router.HandleFunc("/api/v1/tools/{id}", handler.getToolHandler).Methods("GET")
	router.HandleFunc("/api/v1/tools/{id}", handler.updateToolHandler).Methods("PUT")
	router.HandleFunc("/api/v1/tools/{id}", handler.deleteToolHandler).Methods("DELETE")

	// Memory configuration APIs
	router.HandleFunc("/api/v1/memory-config", handler.getMemoryConfigHandler).Methods("GET")
	router.HandleFunc("/api/v1/memory-config", handler.updateMemoryConfigHandler).Methods("PUT")

	// Settings APIs
	router.HandleFunc("/api/v1/settings", handler.listSettingsHandler).Methods("GET")
	router.HandleFunc("/api/v1/settings/{key}", handler.getSettingHandler).Methods("GET")
	router.HandleFunc("/api/v1/settings/{key}", handler.updateSettingHandler).Methods("PUT")

	router.HandleFunc("/api/v1/agents", handler.listAgentsHandler).Methods("GET")
	router.HandleFunc("/api/v1/agents", handler.createAgentHandler).Methods("POST")
	router.HandleFunc("/api/v1/agents/{id}", handler.getAgentHandler).Methods("GET")
	router.HandleFunc("/api/v1/agents/{id}", handler.updateAgentHandler).Methods("PUT")
	router.HandleFunc("/api/v1/agents/{id}", handler.deleteAgentHandler).Methods("DELETE")
	router.HandleFunc("/api/v1/agents/{id}/tools", handler.getAgentToolsHandler).Methods("GET")
	router.HandleFunc("/api/v1/agents/{id}/tools", handler.assignToolToAgentHandler).Methods("POST")
	router.HandleFunc("/api/v1/agents/{id}/tools/{toolId}", handler.removeToolFromAgentHandler).Methods("DELETE")

	router.HandleFunc("/api/v1/workflows", handler.listWorkflowsHandler).Methods("GET")
	router.HandleFunc("/api/v1/workflows", handler.createWorkflowHandler).Methods("POST")
	router.HandleFunc("/api/v1/workflows/{id}", handler.getWorkflowHandler).Methods("GET")
	router.HandleFunc("/api/v1/workflows/{id}", handler.updateWorkflowHandler).Methods("PUT")
	router.HandleFunc("/api/v1/workflows/{id}", handler.deleteWorkflowHandler).Methods("DELETE")

	router.HandleFunc("/api/v1/workflows/{id}/steps", handler.listWorkflowStepsHandler).Methods("GET")
	router.HandleFunc("/api/v1/workflows/{id}/steps", handler.createWorkflowStepHandler).Methods("POST")
	router.HandleFunc("/api/v1/workflows/{workflow_id}/steps/{step_id}", handler.updateWorkflowStepHandler).Methods("PUT")
	router.HandleFunc("/api/v1/workflows/{workflow_id}/steps/{step_id}", handler.deleteWorkflowStepHandler).Methods("DELETE")

	// Job management APIs
	router.HandleFunc("/api/v1/jobs", handler.listJobsHandler).Methods("GET")
	router.HandleFunc("/api/v1/jobs", handler.createJobHandler).Methods("POST")
	router.HandleFunc("/api/v1/jobs/{id}", handler.getJobHandler).Methods("GET")
	router.HandleFunc("/api/v1/jobs/{id}/steps", handler.listJobStepsHandler).Methods("GET")

	// WASM module APIs - Order matters! Specific routes before generic {id} routes
	router.HandleFunc("/api/v1/wasm-modules", handler.listWasmModulesHandler).Methods("GET")
	router.HandleFunc("/api/v1/wasm-modules", handler.createWasmModuleHandler).Methods("POST")

	// WASM compilation and testing APIs (must come before /{id} routes)
	router.HandleFunc("/api/v1/wasm-modules/compile", handler.compileWasmModuleHandler).Methods("POST")
	router.HandleFunc("/api/v1/wasm-modules/test", handler.testWasmModuleHandler).Methods("POST")
	router.HandleFunc("/api/v1/wasm-modules/example", handler.getWasmModuleExampleHandler).Methods("GET")

	// Module-specific routes
	router.HandleFunc("/api/v1/wasm-modules/{id}", handler.getWasmModuleHandler).Methods("GET")
	router.HandleFunc("/api/v1/wasm-modules/{id}", handler.updateWasmModuleHandler).Methods("PUT")
	router.HandleFunc("/api/v1/wasm-modules/{id}", handler.deleteWasmModuleHandler).Methods("DELETE")
	router.HandleFunc("/api/v1/wasm-modules/{id}/source", handler.getWasmModuleSourceHandler).Methods("GET")
	router.HandleFunc("/api/v1/wasm-modules/{id}/source", handler.updateWasmModuleSourceHandler).Methods("PUT")

	// WebSocket endpoint
	wsHandler := api.NewWebSocketHandler(wsHub)
	router.Handle("/ws", wsHandler)

	// Serve frontend (catch-all route)
	router.PathPrefix("/").Handler(frontend.ServeStatic())

	// Calculate server timeouts based on workflow timeout
	// Get workflow timeout from database
	serverWriteTimeout := 15 * time.Minute // Default generous timeout
	timeoutCtx, timeoutCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer timeoutCancel()

	if setting, err := db.GetSetting(timeoutCtx, "timeout_workflow_seconds"); err == nil {
		if timeoutSeconds, err := strconv.Atoi(setting.Value); err == nil && timeoutSeconds > 0 {
			// Set write timeout to workflow timeout + 2 minute buffer
			serverWriteTimeout = time.Duration(timeoutSeconds)*time.Second + 2*time.Minute
		}
	}

	srv := &http.Server{
		Addr:         listenAddr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,  // Time to read request headers
		WriteTimeout: serverWriteTimeout, // Time to write full response
		IdleTimeout:  120 * time.Second,  // Keep-alive idle timeout
	}

	go func() {
		log.Printf("API server listening on %s", listenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// Graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	log.Println("Shutting down API server...")
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Failed to shutdown server: %v", err)
	}

	log.Println("Server shutdown complete")
}
