package handlers

import (
	"net/http"
	"runtime"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/metric/instrument"
)

type HealthCheckResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
	System    SystemInfo `json:"system"`
}

type SystemInfo struct {
	Goroutines int    `json:"goroutines"`
	CPU        int    `json:"cpu_count"`
}

type HealthHandler struct {
	service health.Service
}

func NewHealthHandler(service health.Service) *HealthHandler {
	return &HealthHandler{service: service}
}

func (h *HealthHandler) Check(c *gin.Context) {
	resp := HealthCheckResponse{
		Status:    "healthy",
		Timestamp: time.Now().Format(time.RFC3339),
		System: SystemInfo{
			Goroutines: runtime.NumGoroutine(),
			CPU:        runtime.NumCPU(),
		},
	}

	c.JSON(http.StatusOK, resp)
}

func (h *HealthHandler) DetailedCheck(c *gin.Context) {
	// Collect detailed metrics
	metrics := collectRuntimeMetrics()
	
	detailedResp := struct {
		Status    string      `json:"status"`
		Timestamp string      `json:"timestamp"`
		System    SystemInfo  `json:"system"`
		Metrics   interface{} `json:"metrics"`
	}{
		Status:    "healthy",
		Timestamp: time.Now().Format(time.RFC3339),
		System: SystemInfo{
			Goroutines: runtime.NumGoroutine(),
			CPU:        runtime.NumCPU(),
		},
		Metrics: metrics,
	}

	c.JSON(http.StatusOK, detailedResp)
}

func collectRuntimeMetrics() map[string]interface{} {
	metrics := make(map[string]interface{})
	
	// Get the global meter provider
	meter := global.Meter("mule-runtime")
	
	// Memory stats
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	
	metrics["alloc"] = ms.Alloc
	metrics["sys"] = ms.Sys
	metrics["gc"] = ms.NumGC
	
	return metrics
}