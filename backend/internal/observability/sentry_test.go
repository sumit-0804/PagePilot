package observability

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/trace"
)

// fakeSentry collects the envelopes a Sentry client posts.
func fakeSentry(t *testing.T) (dsn string, received func() string) {
	t.Helper()
	var mu sync.Mutex
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { _ = sentry.Init(sentry.ClientOptions{}) }) // leave no client behind for other tests
	return strings.Replace(srv.URL, "http://", "http://publickey@", 1) + "/1", func() string {
		mu.Lock()
		defer mu.Unlock()
		return strings.Join(bodies, "\n")
	}
}

func TestInitSentryWithoutDSNIsOff(t *testing.T) {
	flush, err := InitSentry(SentryConfig{})
	if err != nil {
		t.Fatal(err)
	}
	flush(time.Millisecond)

	// A malformed DSN is reported to the caller, which logs it and carries on.
	flush, err = InitSentry(SentryConfig{DSN: "not a dsn"})
	if err == nil {
		t.Error("a malformed DSN should return an error")
	}
	flush(time.Millisecond)
}

func TestCaptureSendsTagsButNoSecrets(t *testing.T) {
	record(t)
	dsn, received := fakeSentry(t)
	flush, err := InitSentry(SentryConfig{DSN: dsn, Environment: "test", Release: "abc123"})
	if err != nil {
		t.Fatal(err)
	}

	var traceID string
	r := chi.NewRouter()
	r.Use(sentryhttp.New(sentryhttp.Options{Repanic: true}).Handle)
	r.With(HTTP("/api/chat")).Post("/api/conversations/{id}", func(w http.ResponseWriter, r *http.Request) {
		traceID = TraceID(r.Context())
		CaptureRequest(r, errors.New("vector search failed: connection reset"), CategoryHTTP)
		w.WriteHeader(http.StatusInternalServerError)
	})
	req := httptest.NewRequest(http.MethodPost, "/api/conversations/42?token=secret-query", strings.NewReader(`{"message":"private notes"}`))
	req.Header.Set("Authorization", "Bearer secret-jwt")
	req.Header.Set("Cookie", "session=secret-cookie")
	r.ServeHTTP(httptest.NewRecorder(), req)
	flush(2 * time.Second)

	body := received()
	for _, want := range []string{"vector search failed", `"category":"http"`, `"trace_id":"` + traceID + `"`, `"route":"/api/conversations/{id}"`, `"environment":"test"`, `"release":"abc123"`} {
		if !strings.Contains(body, want) {
			t.Errorf("event is missing %s", want)
		}
	}
	for _, secret := range []string{"secret-jwt", "secret-cookie", "secret-query", "private notes"} {
		if strings.Contains(body, secret) {
			t.Errorf("event leaked %q", secret)
		}
	}
}

func TestCaptureOutsideARequestStillReports(t *testing.T) {
	dsn, received := fakeSentry(t)
	flush, err := InitSentry(SentryConfig{DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	ctx, span := StartRoot(context.Background(), trace.SpanContext{}, "document.process", TypeChain)
	Capture(ctx, errors.New("embedding batch failed"), CategoryDocument, map[string]string{"document_format": "pdf"})
	span.End()
	flush(2 * time.Second)

	if body := received(); !strings.Contains(body, "embedding batch failed") || !strings.Contains(body, `"category":"document"`) {
		t.Errorf("background error not reported: %s", body)
	}
}

func TestCaptureIgnoresNil(t *testing.T) {
	dsn, received := fakeSentry(t)
	flush, _ := InitSentry(SentryConfig{DSN: dsn})
	Capture(context.Background(), nil, CategoryHTTP, nil)
	flush(500 * time.Millisecond)
	if received() != "" {
		t.Error("a nil error must not create an event")
	}
}
