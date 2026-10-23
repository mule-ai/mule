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
	"github.com/mule-ai/mule/internal/primitive"
	"github.com/mule-ai/mule/internal/wake"
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

	flag.StringVar(&dbConnStr, "db", "postgres://user:pass@localhost:5432/mule?sslmode=disable", "PostgreSQL connection string")
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
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("Error closing database: %v", closeErr)
		}
	}()

	if err := db.Ping(); err != nil {
		log.Fatalf("failed to ping database: %v", err)
	}

	// Initialize database schema
	if err := db.InitSchema(); err != nil {
		log.Fatalf("failed to initialize database schema: %v", err)
	}
	log.Println("Database schema initialized successfully")

	// Tenancy bootstrap (docs/TENANCY.md): on first boot create the
	// operator's admin user + API key from MULE_ADMIN_* (generated and
	// logged once when unset). Idempotent: a boot with existing users
	// is a no-op.
	bootInfo, err := api.BootstrapAdmin(context.Background(), db.DB)
	if err != nil {
		log.Fatalf("failed to bootstrap tenancy admin: %v", err)
	}

	// Encryption secret for provider API keys. Set MULE_ENCRYPTION_SECRET to a
	// 16/24/32-byte value; without it we fall back to a built-in default so
	// local development works, but the key provides no real secrecy then.
	secret := os.Getenv("MULE_ENCRYPTION_SECRET")
	if secret == "" {
		secret = "mule-default-secret-key-12345678"
		log.Printf("WARNING: MULE_ENCRYPTION_SECRET not set - provider API keys are encrypted with a public default. Set it in any real deployment.")
	}
	if k := len(secret); k != 16 && k != 24 && k != 32 {
		log.Fatalf("MULE_ENCRYPTION_SECRET must be 16, 24 or 32 bytes (AES key size), got %d", k)
	}
	providerMgr := manager.NewProviderManager(db, []byte(secret))
	agentMgr := manager.NewAgentManager(db)
	workflowMgr := manager.NewWorkflowManager(db)

	// Carries the bootstrap admin as principal so the default
	// provider/agent/workflow rows created below are owned by the
	// admin (visible to everyone, editable only by the admin).
	initCtx := primitive.WithPrincipal(context.Background(),
		primitive.Principal{UserID: bootInfo.AdminID, Admin: true})
	defaults, err := initialization.EnsureAllDefaults(initCtx, providerMgr, agentMgr, workflowMgr)
	if err != nil {
		log.Printf("Warning: failed to ensure default primitives: %v", err)
	} else {
		log.Printf("Default primitives ensured: provider=%s, default_agent=%s, wasm_editor_agent=%s, workflow=%s",
			defaults.Provider.ID, defaults.DefaultAgent.ID, defaults.WasmEditorAgent.ID, defaults.Workflow.ID)
	}

	router := mux.NewRouter()

	// Apply basic middleware first
	router.Use(api.LoggingMiddleware)
	router.Use(api.RecoveryMiddleware)
	router.Use(api.CORSMiddleware)
	// Tenancy auth for /api/v1/* and /v1/*. /health, /ws and the
	// frontend catch-all stay unauthenticated (docs/TENANCY.md).
	router.Use(api.AuthMiddleware(db.DB))

	// Create the API handler early: the voice-loop routes below
	// (H6.3) need its voice hub, and they must register BEFORE the
	// timeout middleware (WebSocket upgrade), same constraint as /ws.
	handler := NewAPIHandler(db)

	// Register WebSocket endpoint BEFORE timeout middleware
	// This is critical because the timeout middleware wraps the ResponseWriter
	// in a way that doesn't implement http.Hijacker, which is required for WebSocket upgrades
	wsHub := api.NewWebSocketHub()
	go wsHub.Run()

	// Initialize job streamer
	jobStore := job.NewPGStore(db.DB) // Access the underlying *sql.DB
	jobStreamer := api.NewJobStreamer(wsHub, jobStore)
	jobStreamer.Start()
	defer jobStreamer.Stop()

	// WebSocket endpoint - registered BEFORE timeout middleware
	wsHandler := api.NewWebSocketHandler(wsHub)
	router.Handle("/ws", wsHandler)

	// Voice-loop call transport (Herd H6.3): per-call audio sockets
	// addressed by mule job id. Registered before the timeout
	// middleware like /ws (WebSocket upgrade needs the raw
	// ResponseWriter); like /ws these are unauthenticated — the 128-
	// bit job id is the capability (spec.md "Voice loop (H6.3)").
	router.Handle("/voice/{job_id}/in", handler.NewVoiceInHandler())
	router.Handle("/voice/{job_id}/out", handler.NewVoiceOutHandler())

	// NOW apply the timeout middleware - it won't affect the WebSocket endpoint
	// because it was registered before this middleware
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

	// Start the workflow engine
	var ctx context.Context
	ctx = context.Background()
	if err := handler.workflowEngine.Start(ctx); err != nil {
		log.Fatalf("Failed to start workflow engine: %v", err)
	}
	defer handler.workflowEngine.Stop()

	// Start the wake scheduler (Herd H3.3): registers the engine's
	// job-event sink and runs the cron scan + event lanes.
	handler.wakeScheduler.Start()
	defer handler.wakeScheduler.Stop()

	// Memory trigger forwarder lane (Herd H4.5): when FORGE_TRIGGER_*
	// is set, poll the forge memory_trigger_queue every 30 s and fire
	// the corresponding mule wake in-process (disabled when
	// FORGE_TRIGGER_AGENTS is empty — no loop, no HTTP).
	memoryTriggerFwd := wake.NewMemoryTriggerForwarder(
		handler.wakeScheduler.Store(),
		handler.wakeScheduler,
		wake.NewMemoryTriggerForwarderConfigFromEnv(),
	)
	memoryTriggerFwd.Start()
	defer memoryTriggerFwd.Stop()

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

	// Skill management APIs
	router.HandleFunc("/api/v1/skills", handler.listSkillsHandler).Methods("GET")
	router.HandleFunc("/api/v1/skills", handler.createSkillHandler).Methods("POST")
	router.HandleFunc("/api/v1/skills/{id}", handler.getSkillHandler).Methods("GET")
	router.HandleFunc("/api/v1/skills/{id}", handler.updateSkillHandler).Methods("PUT")
	router.HandleFunc("/api/v1/skills/{id}", handler.deleteSkillHandler).Methods("DELETE")

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

	// Agent skills APIs
	router.HandleFunc("/api/v1/agents/{id}/skills", handler.getAgentSkillsHandler).Methods("GET")
	router.HandleFunc("/api/v1/agents/{id}/skills", handler.assignSkillsToAgentHandler).Methods("PUT")
	router.HandleFunc("/api/v1/agents/{id}/skills/{skillId}", handler.removeSkillFromAgentHandler).Methods("DELETE")

	router.HandleFunc("/api/v1/workflows", handler.listWorkflowsHandler).Methods("GET")
	router.HandleFunc("/api/v1/workflows", handler.createWorkflowHandler).Methods("POST")
	router.HandleFunc("/api/v1/workflows/{id}", handler.getWorkflowHandler).Methods("GET")
	router.HandleFunc("/api/v1/workflows/{id}", handler.updateWorkflowHandler).Methods("PUT")
	router.HandleFunc("/api/v1/workflows/{id}", handler.deleteWorkflowHandler).Methods("DELETE")

	router.HandleFunc("/api/v1/workflows/{id}/steps", handler.listWorkflowStepsHandler).Methods("GET")
	router.HandleFunc("/api/v1/workflows/{id}/steps", handler.createWorkflowStepHandler).Methods("POST")
	router.HandleFunc("/api/v1/workflows/{id}/steps/reorder", handler.reorderWorkflowStepsHandler).Methods("POST")
	router.HandleFunc("/api/v1/workflows/{workflow_id}/steps/{step_id}", handler.updateWorkflowStepHandler).Methods("PUT")
	router.HandleFunc("/api/v1/workflows/{workflow_id}/steps/{step_id}", handler.deleteWorkflowStepHandler).Methods("DELETE")

	// Job management APIs
	router.HandleFunc("/api/v1/jobs", handler.listJobsHandler).Methods("GET")
	router.HandleFunc("/api/v1/jobs", handler.createJobHandler).Methods("POST")
	router.HandleFunc("/api/v1/jobs/{id}", handler.getJobHandler).Methods("GET")
	router.HandleFunc("/api/v1/jobs/{id}", handler.cancelJobHandler).Methods("DELETE")
	router.HandleFunc("/api/v1/jobs/{id}/steps", handler.listJobStepsHandler).Methods("GET")

	// Wake engine (Herd H3.3): fleet wake policies + external fire ingress.
	router.HandleFunc("/api/v1/wakes", handler.listWakesHandler).Methods("GET")
	router.HandleFunc("/api/v1/wakes", handler.createWakesHandler).Methods("POST")
	router.HandleFunc("/api/v1/wakes/fire", handler.fireWakesHandler).Methods("POST")
	// H5.3 kill-switch + activity feed — static segments, registered
	// before the {id} routes (mux matches in registration order).
	router.HandleFunc("/api/v1/wakes/pause", handler.pauseWakesHandler).Methods("POST")
	router.HandleFunc("/api/v1/wakes/activity", handler.activityWakesHandler).Methods("GET")
	router.HandleFunc("/api/v1/wakes/{id}", handler.getWakesHandler).Methods("GET")
	router.HandleFunc("/api/v1/wakes/{id}", handler.patchWakesHandler).Methods("PATCH")
	router.HandleFunc("/api/v1/wakes/{id}", handler.deleteWakesHandler).Methods("DELETE")
	router.HandleFunc("/api/v1/wakes/{id}/log", handler.wakeLogHandler).Methods("GET")

	// Policy engine (Herd H3.5): owner-scoped CRUD + the synchronous
	// evaluate hook surface + the agent-proposal flow. The static
	// evaluate/proposals routes register BEFORE the {id} routes.
	router.HandleFunc("/api/v1/policies", handler.listPoliciesHandler).Methods("GET")
	router.HandleFunc("/api/v1/policies", handler.createPoliciesHandler).Methods("POST")
	router.HandleFunc("/api/v1/policies/evaluate", handler.evaluatePolicyHandler).Methods("POST")
	router.HandleFunc("/api/v1/policies/memo", handler.memoPolicyHandler).Methods("POST")
	router.HandleFunc("/api/v1/policies/proposals", handler.createProposalHandler).Methods("POST")
	router.HandleFunc("/api/v1/policies/proposals/{id}/approve", handler.approveProposalHandler).Methods("POST")
	router.HandleFunc("/api/v1/policies/{id}", handler.getPolicyHandler).Methods("GET")
	router.HandleFunc("/api/v1/policies/{id}", handler.patchPolicyHandler).Methods("PATCH")
	router.HandleFunc("/api/v1/policies/{id}", handler.deletePolicyHandler).Methods("DELETE")

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
		ReadTimeout:  30 * time.Second,   // Time to read request headers
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
