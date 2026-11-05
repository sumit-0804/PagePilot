package observability

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

// record installs an in-memory tracer provider for one test.
func record(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })
	return rec
}

func attr(s sdktrace.ReadOnlySpan, key string) string {
	for _, kv := range s.Attributes() {
		if string(kv.Key) == key {
			return kv.Value.Emit()
		}
	}
	return ""
}

func TestSetupIsOffWithoutConfig(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	cfg := Config{LangfusePublicKey: "pk-only"}
	if cfg.Enabled() {
		t.Fatal("a public key alone should not enable tracing")
	}
	shutdown, err := Setup(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestLangfuseExport(t *testing.T) {
	endpoint, headers := langfuseExport(Config{LangfusePublicKey: "pk-lf-1", LangfuseSecretKey: "sk-lf-2", LangfuseBaseURL: "https://us.cloud.langfuse.com/"})
	if endpoint != "https://us.cloud.langfuse.com/api/public/otel/v1/traces" {
		t.Errorf("endpoint = %q", endpoint)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("pk-lf-1:sk-lf-2"))
	if headers["Authorization"] != want {
		t.Errorf("Authorization = %q, want %q", headers["Authorization"], want)
	}
	if headers["x-langfuse-ingestion-version"] != "4" {
		t.Errorf("missing ingestion version header: %v", headers)
	}

	endpoint, _ = langfuseExport(Config{LangfusePublicKey: "a", LangfuseSecretKey: "b"})
	if !strings.HasPrefix(endpoint, "https://cloud.langfuse.com/") {
		t.Errorf("default endpoint = %q", endpoint)
	}
}

func TestUserRef(t *testing.T) {
	id := "6f1c1b8e-6a0e-4b8e-9b8e-1c2d3e4f5a6b"
	ref := UserRef(id)
	if ref == "" || ref != UserRef(id) || strings.Contains(ref, id[:8]) || len(ref) != 16 {
		t.Errorf("UserRef(%q) = %q; want a stable 16-char pseudonym", id, ref)
	}
	if UserRef("") != "" {
		t.Error("an empty id should give an empty ref")
	}
}

func TestStartRecordsNothingOutsideATrace(t *testing.T) {
	rec := record(t)
	_, span := Start(context.Background(), "orphan", TypeSpan)
	End(span, errors.New("boom"))
	if n := len(rec.Ended()); n != 0 {
		t.Errorf("got %d spans; Start must not open new traces", n)
	}
}

func TestTraceAttributesReachEveryChildSpan(t *testing.T) {
	rec := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(propagator{}), sdktrace.WithSpanProcessor(rec)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	ctx, root := StartRoot(context.Background(), trace.SpanContext{}, "request", TypeSpan)
	ctx = WithTraceAttributes(ctx, AttrUserID.String("u1"), AttrSessionID.String("c1"))
	ctx, child := Start(ctx, "chat.turn", TypeAgent)
	// A span from another library (like ADK's) started below the context gets them too.
	_, foreign := otel.Tracer("other").Start(ctx, "invoke_agent")
	foreign.End()
	child.End()
	root.End()

	for _, s := range rec.Ended() {
		want := s.Name() != "request"
		if got := attr(s, "langfuse.user.id") == "u1" && attr(s, "langfuse.session.id") == "c1"; got != want {
			t.Errorf("%s: user/session present = %v, want %v", s.Name(), got, want)
		}
	}
}

func TestEndRecordsErrorsButNotCancellation(t *testing.T) {
	rec := record(t)
	ctx, root := StartRoot(context.Background(), trace.SpanContext{}, "root", TypeSpan)
	_, failed := Start(ctx, "failed", TypeTool)
	End(failed, errors.New("boom"))
	_, cancelled := Start(ctx, "cancelled", TypeTool)
	End(cancelled, context.Canceled)
	root.End()

	spans := rec.Ended()
	if len(spans) != 3 {
		t.Fatalf("got %d spans", len(spans))
	}
	if spans[0].Status().Code != codes.Error || attr(spans[0], "langfuse.observation.level") != "ERROR" {
		t.Errorf("failed span: status %v, level %q", spans[0].Status(), attr(spans[0], "langfuse.observation.level"))
	}
	if attr(spans[0], "langfuse.observation.type") != "tool" {
		t.Errorf("observation type = %q", attr(spans[0], "langfuse.observation.type"))
	}
	if spans[1].Status().Code == codes.Error || attr(spans[1], "langfuse.observation.level") != "WARNING" {
		t.Errorf("cancelled span: status %v, level %q", spans[1].Status(), attr(spans[1], "langfuse.observation.level"))
	}
}

func TestStartRootLinksToTheRequest(t *testing.T) {
	rec := record(t)
	reqCtx, req := StartRoot(context.Background(), trace.SpanContext{}, "upload", TypeSpan)
	_, bg := StartRoot(context.Background(), req.SpanContext(), "document.process", TypeChain)
	bg.End()
	req.End()

	var got sdktrace.ReadOnlySpan
	for _, s := range rec.Ended() {
		if s.Name() == "document.process" {
			got = s
		}
	}
	if got == nil {
		t.Fatal("no document.process span")
	}
	if got.Parent().IsValid() || got.SpanContext().TraceID() == req.SpanContext().TraceID() {
		t.Error("background span should start its own trace")
	}
	if len(got.Links()) != 1 || got.Links()[0].SpanContext.TraceID() != req.SpanContext().TraceID() {
		t.Errorf("links = %v", got.Links())
	}
	if TraceID(reqCtx) != req.SpanContext().TraceID().String() {
		t.Error("TraceID should read the span in the context")
	}
}

func TestHTTPMiddleware(t *testing.T) {
	rec := record(t)
	var flushable, unwrappable bool
	h := HTTP("/api/chat")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, flushable = w.(http.Flusher)
		_, unwrappable = w.(interface{ Unwrap() http.ResponseWriter })
		w.WriteHeader(http.StatusBadGateway)
	}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/api/chat", nil))

	if !flushable || !unwrappable {
		t.Errorf("writer lost Flush (%v) or Unwrap (%v); chat streaming needs both", flushable, unwrappable)
	}
	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("got %d spans", len(spans))
	}
	s := spans[0]
	if s.Name() != "POST /api/chat" || attr(s, "http.response.status_code") != "502" || s.Status().Code != codes.Error {
		t.Errorf("span %q status %v code attr %q", s.Name(), s.Status(), attr(s, "http.response.status_code"))
	}
	if rr.Header().Get(TraceHeader) != s.SpanContext().TraceID().String() {
		t.Errorf("%s = %q", TraceHeader, rr.Header().Get(TraceHeader))
	}
}

func TestCaptureContentIsOffByDefault(t *testing.T) {
	t.Setenv("OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT", "")
	if CaptureContent() {
		t.Error("content capture should be off by default")
	}
	t.Setenv("OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT", "span_only")
	if !CaptureContent() {
		t.Error("SPAN_ONLY should turn content capture on")
	}
}
