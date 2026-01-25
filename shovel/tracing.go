package shovel

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"go.opentelemetry.io/otel/trace"
)

// TracingConfig holds configuration for OpenTelemetry tracing.
type TracingConfig struct {
	// Enabled controls whether tracing is active.
	Enabled bool

	// Endpoint is the OTEL collector endpoint (e.g., "otel-collector.monitoring:4317").
	// If empty, uses OTEL_EXPORTER_OTLP_ENDPOINT env var.
	Endpoint string

	// Protocol is "grpc" or "http". Defaults to "grpc".
	Protocol string

	// ServiceName identifies this service in traces. Defaults to "shovel".
	ServiceName string

	// ServiceVersion is the version tag. Defaults to build version.
	ServiceVersion string

	// SampleRate is the fraction of traces to sample (0.0 to 1.0).
	// Defaults to 1.0 (sample all). Set lower for high-throughput production.
	SampleRate float64

	// Insecure disables TLS for the exporter connection.
	Insecure bool
}

// Tracer is the global tracer instance for Shovel.
// Use this to create spans: ctx, span := Tracer.Start(ctx, "operation-name")
// Initialized to a no-op tracer by default; call InitTracing to enable real tracing.
var Tracer trace.Tracer = otel.Tracer("shovel")

// tracerProvider holds the SDK tracer provider for shutdown.
var tracerProvider *sdktrace.TracerProvider

// InitTracing initializes OpenTelemetry tracing with the given configuration.
// Returns a shutdown function that should be called on application exit.
//
// Example:
//
//	shutdown, err := InitTracing(ctx, TracingConfig{
//	    Enabled:     true,
//	    Endpoint:    "otel-collector.monitoring:4317",
//	    ServiceName: "shovel",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer shutdown(ctx)
func InitTracing(ctx context.Context, cfg TracingConfig) (func(context.Context) error, error) {
	if !cfg.Enabled {
		// Return no-op tracer and shutdown
		Tracer = otel.Tracer("shovel")
		return func(context.Context) error { return nil }, nil
	}

	// Apply defaults
	if cfg.ServiceName == "" {
		cfg.ServiceName = "shovel"
	}
	if cfg.ServiceVersion == "" {
		cfg.ServiceVersion = "unknown"
	}
	if cfg.Protocol == "" {
		cfg.Protocol = "grpc"
	}
	// Normalize http/protobuf to http (standard OTEL env value)
	if cfg.Protocol == "http/protobuf" {
		cfg.Protocol = "http"
	}
	// Only default to 1.0 if SampleRate was never set (negative).
	// This allows explicit 0.0 sample rate while tracing is enabled.
	if cfg.SampleRate < 0 {
		cfg.SampleRate = 1.0
	}

	// Build resource with service information
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			attribute.String("service.namespace", "shovel"),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("creating resource: %w", err)
	}

	// Create exporter based on protocol
	var exporter sdktrace.SpanExporter
	switch cfg.Protocol {
	case "grpc":
		opts := []otlptracegrpc.Option{}
		if cfg.Endpoint != "" {
			opts = append(opts, otlptracegrpc.WithEndpoint(cfg.Endpoint))
		}
		if cfg.Insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		exporter, err = otlptracegrpc.New(ctx, opts...)
	case "http":
		opts := []otlptracehttp.Option{}
		if cfg.Endpoint != "" {
			opts = append(opts, otlptracehttp.WithEndpoint(cfg.Endpoint))
		}
		if cfg.Insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		exporter, err = otlptracehttp.New(ctx, opts...)
	default:
		return nil, fmt.Errorf("unsupported protocol: %s (use 'grpc' or 'http')", cfg.Protocol)
	}
	if err != nil {
		return nil, fmt.Errorf("creating exporter: %w", err)
	}

	// Create sampler
	var sampler sdktrace.Sampler
	if cfg.SampleRate >= 1.0 {
		sampler = sdktrace.AlwaysSample()
	} else {
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}

	// Create tracer provider
	tracerProvider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set as global provider
	otel.SetTracerProvider(tracerProvider)

	// Set up propagation (W3C Trace Context)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Create named tracer
	Tracer = tracerProvider.Tracer(cfg.ServiceName)

	slog.Info("tracing-initialized",
		"endpoint", cfg.Endpoint,
		"protocol", cfg.Protocol,
		"sample_rate", cfg.SampleRate,
	)

	// Return shutdown function
	return func(ctx context.Context) error {
		slog.Info("tracing-shutdown")
		return tracerProvider.Shutdown(ctx)
	}, nil
}

// TracingConfigFromEnv creates a TracingConfig from environment variables.
// Supported variables:
//   - OTEL_ENABLED: "true" to enable tracing
//   - OTEL_EXPORTER_OTLP_ENDPOINT: collector endpoint
//   - OTEL_EXPORTER_OTLP_PROTOCOL: "grpc", "http", or "http/protobuf"
//   - OTEL_SERVICE_NAME: service name (default: "shovel")
//   - OTEL_TRACE_SAMPLE_RATE: sample rate 0.0-1.0 (default: 1.0)
//   - OTEL_EXPORTER_OTLP_INSECURE: "true" to disable TLS
func TracingConfigFromEnv() TracingConfig {
	cfg := TracingConfig{
		Enabled:     os.Getenv("OTEL_ENABLED") == "true",
		Endpoint:    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		Protocol:    os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL"),
		ServiceName: os.Getenv("OTEL_SERVICE_NAME"),
		Insecure:    os.Getenv("OTEL_EXPORTER_OTLP_INSECURE") == "true",
		SampleRate:  -1.0, // Sentinel: unset, will default to 1.0 in InitTracing
	}

	// Parse sample rate - allows explicit 0.0 to disable sampling while tracing enabled
	if rate := os.Getenv("OTEL_TRACE_SAMPLE_RATE"); rate != "" {
		var r float64
		if _, err := fmt.Sscanf(rate, "%f", &r); err == nil && r >= 0 && r <= 1 {
			cfg.SampleRate = r
		}
	}

	return cfg
}

// SpanFromContext extracts the current span from context.
// Returns a no-op span if none exists.
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// WithSpanAttributes adds attributes to the current span in context.
func WithSpanAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attrs...)
}
