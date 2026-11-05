package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/pagepilot/backend/internal/services"
)

// captureEvents installs a Sentry client that keeps events in memory instead of sending them.
func captureEvents(t *testing.T) func() []*sentry.Event {
	t.Helper()
	var mu sync.Mutex
	var events []*sentry.Event
	err := sentry.Init(sentry.ClientOptions{
		Dsn: "http://public@127.0.0.1:1/1",
		BeforeSend: func(e *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sentry.Init(sentry.ClientOptions{}) })
	return func() []*sentry.Event {
		sentry.Flush(time.Second)
		mu.Lock()
		defer mu.Unlock()
		return events
	}
}

func TestOnlyServerFaultsAreReported(t *testing.T) {
	events := captureEvents(t)
	a := &API{}
	r := chi.NewRouter()
	// Same order as SetupRouter.
	r.Use(middleware.Recoverer)
	r.Use(sentryhttp.New(sentryhttp.Options{Repanic: true}).Handle)
	r.Get("/missing", func(w http.ResponseWriter, r *http.Request) {
		a.fail(w, r, fmt.Errorf("conversation: %w", services.ErrNotFound))
	})
	r.Get("/broken/{id}", func(w http.ResponseWriter, r *http.Request) {
		a.fail(w, r, errors.New("saving assistant message: connection reset"))
	})
	r.Get("/panics", func(http.ResponseWriter, *http.Request) { panic("nil map write") })

	for path, wantStatus := range map[string]int{"/missing": 404, "/broken/7": 500, "/panics": 500} {
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != wantStatus {
			t.Errorf("%s: status %d, want %d", path, rr.Code, wantStatus)
		}
	}

	got := events()
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2 (the 500 and the panic; never the 404)", len(got))
	}
	var sawRoute, sawPanic bool
	for _, e := range got {
		if e.Tags["route"] == "/broken/{id}" && e.Tags["category"] == "http" {
			sawRoute = true
		}
		if e.Level == sentry.LevelFatal {
			sawPanic = true
		}
	}
	if !sawRoute || !sawPanic {
		t.Errorf("events: route tagged %v, panic reported %v", sawRoute, sawPanic)
	}
}
