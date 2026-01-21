package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"mule/internal/config"
	"mule/internal/handlers"
	"mule/internal/middleware/telemetry"
	"mule/internal/services/health"
	"mule/internal/services/observability"

	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Initialize observability
	shutdown := observability.Initialize(cfg.Observability)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdown(ctx); err != nil {
			log.Printf("Error shutting down observability: %v", err)
		}
	}()

	// Initialize services
	healthService := health.NewService()

	// Setup router
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(telemetry.Metrics())
	router.Use(telemetry.Telemetry()) // Add telemetry middleware

	// Register handlers
	handlers.RegisterHealthRoutes(router, healthService)

	// Start server
	server := &http.Server{
		Addr:    ":" + cfg.Server.Port,
		Handler: router,
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start server: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	// Shutdown server gracefully
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	wg.Wait()
}