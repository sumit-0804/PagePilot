// Package observability sets up OpenTelemetry tracing, exported over OTLP/HTTP (by default to Langfuse).
package observability

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const defaultServiceName = "pagepilot-backend"

// Config holds the tracing settings. Everything is optional: with no exporter configured, tracing is off.
type Config struct {
	Environment string
	Version     string

	LangfusePublicKey string
	LangfuseSecretKey string
	LangfuseBaseURL   string // e.g. https://cloud.langfuse.com or https://us.cloud.langfuse.com

	// SampleRatio is the share of new traces kept, from 0 to 1.
	SampleRatio float64
}

// Enabled reports whether some trace exporter is configured.
func (c Config) Enabled() bool {
	return otlpEnvSet() || (c.LangfusePublicKey != "" && c.LangfuseSecretKey != "")
}

// Setup installs the global tracer provider and returns a function that flushes and stops it.
// When nothing is configured it installs nothing, so spans are no-ops.
func Setup(ctx context.Context, cfg Config) (shutdown func(context.Context) error, err error) {
	noop := func(context.Context) error { return nil }
	if !cfg.Enabled() {
		return noop, nil
	}

	exporter, err := newExporter(ctx, cfg)
	if err != nil {
		return noop, fmt.Errorf("creating trace exporter: %w", err)
	}
	res, err := newResource(ctx, cfg)
	if err != nil {
		return noop, fmt.Errorf("creating trace resource: %w", err)
	}

	ratio := cfg.SampleRatio
	if ratio <= 0 || ratio > 1 {
		ratio = 1
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
		sdktrace.WithSpanProcessor(propagator{}),
		// Spans are queued and exported in the background; a full queue drops spans instead of blocking requests.
		sdktrace.WithBatcher(exporter,
			sdktrace.WithMaxQueueSize(2048),
			sdktrace.WithMaxExportBatchSize(256),
			sdktrace.WithBatchTimeout(5*time.Second),
			sdktrace.WithExportTimeout(10*time.Second),
		),
	)
	// No propagator is installed on purpose: traces start at this API, so a client-sent traceparent
	// can't attach our spans to a parent Langfuse never receives.
	otel.SetTracerProvider(tp)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		log.Printf("[telemetry] %v", err)
	}))
	return tp.Shutdown, nil
}

func newExporter(ctx context.Context, cfg Config) (sdktrace.SpanExporter, error) {
	// Standard OTEL_EXPORTER_OTLP_* variables win; the exporter reads them itself.
	if otlpEnvSet() {
		return otlptracehttp.New(ctx)
	}
	endpoint, headers := langfuseExport(cfg)
	return otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(endpoint),
		otlptracehttp.WithHeaders(headers),
		otlptracehttp.WithTimeout(10*time.Second),
	)
}

// langfuseExport returns the OTLP traces endpoint and auth headers for a Langfuse project.
func langfuseExport(cfg Config) (endpoint string, headers map[string]string) {
	base := strings.TrimRight(cfg.LangfuseBaseURL, "/")
	if base == "" {
		base = "https://cloud.langfuse.com"
	}
	auth := base64.StdEncoding.EncodeToString([]byte(cfg.LangfusePublicKey + ":" + cfg.LangfuseSecretKey))
	return base + "/api/public/otel/v1/traces", map[string]string{
		"Authorization": "Basic " + auth,
		// Without this header Langfuse can take up to 10 minutes to show directly ingested spans.
		"x-langfuse-ingestion-version": "4",
	}
}

func newResource(ctx context.Context, cfg Config) (*resource.Resource, error) {
	attrs := []attribute.KeyValue{
		attribute.String("service.name", defaultServiceName),
	}
	if cfg.Environment != "" {
		attrs = append(attrs,
			attribute.String("deployment.environment.name", cfg.Environment),
			AttrEnvironment.String(cfg.Environment),
		)
	}
	if cfg.Version != "" {
		attrs = append(attrs, attribute.String("service.version", cfg.Version), AttrRelease.String(cfg.Version))
	}
	// WithFromEnv comes last so OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES override the defaults.
	return resource.New(ctx,
		resource.WithAttributes(attrs...),
		resource.WithTelemetrySDK(),
		resource.WithFromEnv(),
	)
}

func otlpEnvSet() bool {
	return strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) != "" ||
		strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")) != ""
}
