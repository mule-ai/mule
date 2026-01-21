package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

var (
	tracer = otel.Tracer("mule-api")
	meter  = otel.Meter("mule-api")

	httpRequestCount metric.Int64Counter
	httpResponseTime metric.Float64Histogram
)

func init() {
	var err error
	httpRequestCount, err = meter.Int64Counter(
		"http_request_count",
		metric.WithDescription("Number of HTTP requests"),
		metric.WithUnit("1"),
	)
	if err != nil {
		panic(err)
	}

	httpResponseTime, err = meter.Float64Histogram(
		"http_response_time_ms",
		metric.WithDescription("HTTP response times in milliseconds"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		panic(err)
	}
}

func Telemetry() gin.HandlerFunc {
	// Use the otelgin middleware for tracing
	return otelgin.Middleware("mule-api")
}

func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		
		// Process request
		c.Next()
		
		// Record metrics
		duration := time.Since(start).Seconds() * 1000 // Convert to milliseconds
		
		labels := []attribute.KeyValue{
			attribute.String("method", c.Request.Method),
			attribute.String("path", c.Request.URL.Path),
			attribute.Int("status", c.Writer.Status()),
		}
		
		httpRequestCount.Add(c.Request.Context(), 1, metric.WithAttributes(labels...))
		httpResponseTime.Record(c.Request.Context(), duration, metric.WithAttributes(labels...))
	}
}