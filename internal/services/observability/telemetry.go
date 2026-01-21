package observability

import (
	"context"
	"fmt"
	"log"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
)

type ObservabilityConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	ServiceName    string `mapstructure:"service_name"`
	OTLPEndpoint   string `mapstructure:"otlp_endpoint"`
	MetricsEnabled bool   `mapstructure:"metrics_enabled"`
	TracesEnabled  bool   `mapstructure:"traces_enabled"`
}

func Initialize(config ObservabilityConfig) func(context.Context) error {
	if !config.Enabled {
		return func(context.Context) error { return nil }
	}

	var tracerProvider *sdktrace.TracerProvider
	var meterProvider metric.MeterProvider
	var conn *grpc.ClientConn
	var shutdownFuncs []func(context.Context) error

	// Setup tracing
	if config.TracesEnabled {
		var err error
		conn, err = grpc.Dial(config.OTLPEndpoint, grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			log.Fatalf("Failed to connect to OTLP endpoint: %v", err)
		}

		traceExporter, err := otlptracegrpc.New(context.Background(), otlptracegrpc.WithGRPCConn(conn))
		if err != nil {
			log.Fatalf("Failed to create trace exporter: %v", err)
		}

		tracerProvider = sdktrace.NewTracerProvider(
			sdktrace.WithBatcher(traceExporter),
			sdktrace.WithResource(resource.NewWithAttributes(
				semconv.SchemaURL,
				semconv.ServiceNameKey.String(config.ServiceName),
			)),
		)
		otel.SetTracerProvider(tracerProvider)
		shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	}

	// Setup metrics
	if config.MetricsEnabled {
		metricExporter, err := otlpmetricgrpc.New(context.Background(), otlpmetricgrpc.WithGRPCConn(conn))
		if err != nil {
			log.Fatalf("Failed to create metric exporter: %v", err)
		}

		meterProvider = metric.NewMeterProvider(
			metric.WithReader(metric.NewPeriodicReader(metricExporter, metric.WithInterval(10*time.Second))),
		)
		global.SetMeterProvider(meterProvider)
		shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	}

	// Set global propagator
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))

	// Return composite shutdown function
	return func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			if e := fn(ctx); e != nil {
				err = fmt.Errorf("shutdown error: %v", e)
			}
		}
		if conn != nil {
			conn.Close()
		}
		return err
	}
}