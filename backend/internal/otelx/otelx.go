// Package otelx wires OpenTelemetry traces behind a feature flag.
//
// Initialisation is gated on OTEL_EXPORTER_OTLP_ENDPOINT. When the env var
// is empty all helpers become no-ops so call sites never need nil checks.
// The transport is selected from OTEL_EXPORTER_OTLP_PROTOCOL
// ("grpc" — default — or "http/protobuf"); sampling is parent-based with a
// head sampler controlled by OTEL_TRACES_SAMPLER_ARG (default 0.1).
//
// Instrumentation scope is deliberately small: the HTTP server, outbound
// HTTP clients used by integrations, and a thin SpanFromContext wrapper for
// store calls. Fine-grained database span coverage is intentionally out of
// scope per PMS_11 T3.5 ("top-level only").
package otelx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
)

// enabled reflects whether Init() successfully configured a tracer provider.
// Reads don't need synchronisation because Init() runs once at startup
// before any goroutines spawn.
var enabled bool

// tracerName is the instrumentation scope used for manually-created spans.
const tracerName = "pms/backend"

// Init configures the global tracer provider when OTEL_EXPORTER_OTLP_ENDPOINT
// is set. env and release propagate to the resource attributes. The returned
// shutdown function flushes and closes exporters; callers should defer it
// before process exit.
func Init(ctx context.Context, env, release string) (shutdown func(context.Context) error, err error) {
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint == "" {
		return func(context.Context) error { return nil }, nil
	}

	exporter, err := buildExporter(ctx)
	if err != nil {
		return nil, fmt.Errorf("otel exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("pms-backend"),
			semconv.ServiceVersion(release),
			semconv.DeploymentEnvironment(env),
		),
		resource.WithProcessRuntimeName(),
		resource.WithProcessRuntimeVersion(),
	)
	if err != nil {
		return nil, fmt.Errorf("otel resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(5*time.Second)),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(sampleRatio()))),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	enabled = true

	return func(c context.Context) error {
		c, cancel := context.WithTimeout(c, 5*time.Second)
		defer cancel()
		return tp.Shutdown(c)
	}, nil
}

// buildExporter picks the OTLP transport based on OTEL_EXPORTER_OTLP_PROTOCOL.
// Insecure endpoints (e.g. localhost:4317) are implied by stripping the
// scheme; operators needing TLS configure it through OTEL standard env vars.
func buildExporter(ctx context.Context) (sdktrace.SpanExporter, error) {
	proto := strings.ToLower(strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")))
	switch proto {
	case "", "grpc":
		return otlptrace.New(ctx, otlptracegrpc.NewClient())
	case "http/protobuf", "http":
		return otlptrace.New(ctx, otlptracehttp.NewClient())
	default:
		return nil, errors.New("OTEL_EXPORTER_OTLP_PROTOCOL must be grpc or http/protobuf")
	}
}

// sampleRatio reads OTEL_TRACES_SAMPLER_ARG and falls back to 0.1 so the
// default production setup is cheap to run.
func sampleRatio() float64 {
	raw := strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER_ARG"))
	if raw == "" {
		return 0.1
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v < 0 || v > 1 {
		return 0.1
	}
	return v
}

// Enabled reports whether tracing is live. Handy for conditionally adding
// attributes without importing otel in the caller.
func Enabled() bool { return enabled }

// Middleware wraps an http.Handler with server-side span creation. When
// tracing is disabled it returns the handler unchanged to avoid allocating
// a wrapper per request.
func Middleware(serviceName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if !enabled {
			return next
		}
		return otelhttp.NewHandler(next, serviceName)
	}
}

// HTTPTransport wraps base (or http.DefaultTransport when nil) so outbound
// calls propagate the current span's context to the remote service.
func HTTPTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	if !enabled {
		return base
	}
	return otelhttp.NewTransport(base)
}

// NewHTTPClient builds an instrumented http.Client with the given timeout.
// Integrations (Nuki, ICS fetch, …) should use this factory.
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: HTTPTransport(nil),
	}
}

// StartSpan opens a manual span. Callers must call the returned end
// function (typically via defer). When tracing is disabled, the returned
// span is a no-op and end() does nothing measurable.
func StartSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, func(err error)) {
	if !enabled {
		return ctx, func(error) {}
	}
	ctx, span := otel.Tracer(tracerName).Start(ctx, name, trace.WithAttributes(attrs...))
	return ctx, func(err error) {
		if err != nil {
			span.RecordError(err)
		}
		span.End()
	}
}
