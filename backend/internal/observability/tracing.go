package observability

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/pagepilot/backend"

// ObservationType tells Langfuse how to show a span.
type ObservationType string

const (
	TypeSpan       ObservationType = "span"
	TypeGeneration ObservationType = "generation"
	TypeAgent      ObservationType = "agent"
	TypeTool       ObservationType = "tool"
	TypeChain      ObservationType = "chain"
	TypeRetriever  ObservationType = "retriever"
	TypeEmbedding  ObservationType = "embedding"
)

// Tracer reads the global provider on each call, so it picks up the one Setup installs.
func Tracer() trace.Tracer { return otel.Tracer(tracerName) }

// Start opens a child span of the given Langfuse observation type. Outside a traced operation it
// records nothing, so untraced routes (and health checks) never start traces of their own.
func Start(ctx context.Context, name string, typ ObservationType, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if !trace.SpanContextFromContext(ctx).IsValid() {
		return ctx, trace.SpanFromContext(ctx)
	}
	attrs = append(attrs, AttrObservationType.String(string(typ)))
	return Tracer().Start(ctx, name, trace.WithAttributes(attrs...))
}

// StartRoot opens a span that starts a new trace, linked to `from` when it is valid.
// It is for background work, such as document processing, that outlives the request that started it.
func StartRoot(ctx context.Context, from trace.SpanContext, name string, typ ObservationType, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	opts := []trace.SpanStartOption{trace.WithNewRoot()}
	if from.IsValid() {
		opts = append(opts, trace.WithLinks(trace.Link{SpanContext: from}))
	}
	attrs = append(attrs, AttrObservationType.String(string(typ)), AttrTraceName.String(name))
	opts = append(opts, trace.WithAttributes(attrs...))
	return Tracer().Start(ctx, name, opts...)
}

// End records err on the span (if any) and ends it. A cancelled request is a warning, not an error.
func End(span trace.Span, err error) {
	if err != nil {
		msg := truncate(err.Error(), 500)
		if errors.Is(err, context.Canceled) {
			span.SetAttributes(AttrLevel.String("WARNING"), AttrStatusMessage.String(msg))
		} else {
			span.RecordError(err)
			span.SetStatus(codes.Error, msg)
			span.SetAttributes(AttrLevel.String("ERROR"))
		}
	}
	span.End()
}

type propagatedKey struct{}

// WithTraceAttributes puts attributes in ctx that every span started below it receives,
// including ADK's spans. Langfuse stores user, session and tags per observation, so they must
// be on each span for filters to match whole traces.
func WithTraceAttributes(ctx context.Context, attrs ...attribute.KeyValue) context.Context {
	prev, _ := ctx.Value(propagatedKey{}).([]attribute.KeyValue)
	merged := append(append([]attribute.KeyValue{}, prev...), attrs...)
	return context.WithValue(ctx, propagatedKey{}, merged)
}

// propagator is a span processor that applies WithTraceAttributes to new spans.
type propagator struct{}

func (propagator) OnStart(parent context.Context, s sdktrace.ReadWriteSpan) {
	if attrs, ok := parent.Value(propagatedKey{}).([]attribute.KeyValue); ok {
		s.SetAttributes(attrs...)
	}
}
func (propagator) OnEnd(sdktrace.ReadOnlySpan)      {}
func (propagator) Shutdown(context.Context) error   { return nil }
func (propagator) ForceFlush(context.Context) error { return nil }

// TraceID returns the current trace id, or "" when the context has no recording span.
func TraceID(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() || !sc.IsSampled() {
		return ""
	}
	return sc.TraceID().String()
}

// UserRef turns a user id into a stable pseudonym, so traces can be grouped by user without the real id.
func UserRef(userID string) string {
	if userID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("pagepilot-user:" + userID))
	return hex.EncodeToString(sum[:8])
}

// CaptureContent reports whether prompts and outputs may be put on spans. It shares ADK's switch,
// so one setting controls both: OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT=SPAN_ONLY.
func CaptureContent() bool {
	switch strings.ToUpper(strings.TrimSpace(os.Getenv("OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT"))) {
	case "SPAN_ONLY", "SPAN_AND_EVENT":
		return true
	}
	return false
}

// SetContent puts input/output text on the span only when content capture is on.
func SetContent(span trace.Span, key attribute.Key, text string) {
	if text != "" && CaptureContent() {
		span.SetAttributes(key.String(truncate(text, 4000)))
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
