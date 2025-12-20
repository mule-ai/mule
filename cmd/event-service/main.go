package main

import (
	"context"
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"event-service/internal/config"
	"event-service/internal/processor"
	"event-service/internal/supabase"
	"event-service/internal/terraform"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Create Supabase client
	supabaseClient, err := supabase.NewClient(supabase.Config{
		URL:    cfg.SupabaseURL,
		APIKey: cfg.SupabaseKey,
	})
	if err != nil {
		log.Fatalf("Failed to create Supabase client: %v", err)
	}
	defer supabaseClient.Close()

	// Create webhook processor
	webhookProcessor := processor.NewWebhookProcessor(cfg.MuleAPIURL, cfg.MuleAPIToken)

	// Create context that listens for interrupt signal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start event listener in a separate goroutine
	eventChannel := make(chan supabase.Event, 100)
	go func() {
		for {
			err := supabaseClient.ListenForEvents(ctx, eventChannel)
			if err != nil {
				log.Printf("Error listening for events: %v", err)
				time.Sleep(5 * time.Second) // Wait before retrying
			}
		}
	}()

	// Start event processor in a separate goroutine
	go func() {
		for {
			select {
			case event := <-eventChannel:
				// Process the event
				err := webhookProcessor.ProcessEvent(ctx, event)
				if err != nil {
					log.Printf("Error processing event %s: %v", event.ID, err)
					continue
				}

				// Mark event as processed
				err = supabaseClient.MarkEventProcessed(ctx, event.ID)
				if err != nil {
					log.Printf("Error marking event %s as processed: %v", event.ID, err)
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Create HTTP server
	r := gin.Default()

	// Serve static files
	r.Static("/static", "./web/static")

	// Register routes
	registerRoutes(r, cfg, supabaseClient, terraform.GetEmbeddedFiles())

	// Format server address
	addr := ":" + fmt.Sprintf("%d", cfg.ServerPort)
	srv := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	// Start server in a goroutine
	go func() {
		log.Printf("Starting server on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	log.Println("Event Service started successfully")

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
	}

	log.Println("Server exiting")
}

func registerRoutes(r *gin.Engine, cfg *config.Config, supabaseClient *supabase.Client, tfFiles embed.FS) {
	// Serve HTML files
	r.LoadHTMLGlob("web/html/*.html")

	// Home page
	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", gin.H{})
	})

	// Dashboard
	r.GET("/events", func(c *gin.Context) {
		c.HTML(http.StatusOK, "events.html", gin.H{})
	})

	// Configuration page
	r.GET("/config", func(c *gin.Context) {
		c.HTML(http.StatusOK, "config.html", gin.H{})
	})

	// API routes
	api := r.Group("/api")
	{
		// Configuration endpoints
		api.POST("/config/supabase", func(c *gin.Context) {
			// TODO: Implement Supabase configuration saving
			c.JSON(http.StatusOK, gin.H{"message": "Supabase configuration saved"})
		})

		api.POST("/config/mule", func(c *gin.Context) {
			// TODO: Implement Mule API configuration saving
			c.JSON(http.StatusOK, gin.H{"message": "Mule API configuration saved"})
		})

		// Terraform endpoints
		api.POST("/terraform/apply", func(c *gin.Context) {
			// TODO: Implement Terraform apply
			c.JSON(http.StatusOK, gin.H{"message": "Terraform configuration applied"})
		})

		api.POST("/terraform/destroy", func(c *gin.Context) {
			// TODO: Implement Terraform destroy
			c.JSON(http.StatusOK, gin.H{"message": "Terraform resources destroyed"})
		})
	}
}