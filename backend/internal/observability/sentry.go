package observability

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
)

// SentryConfig turns on error reporting when DSN is set.
type SentryConfig struct {
	DSN         string
	Environment string
	Release     string
}

// InitSentry sets up error reporting and returns a flush function for shutdown.
// Without a DSN it does nothing, and Capture calls are no-ops.
func InitSentry(cfg SentryConfig) (flush func(time.Duration), err error) {
	noop := func(time.Duration) {}
	if cfg.DSN == "" {
		return noop, nil
	}
	err = sentry.Init(sentry.ClientOptions{
		Dsn:              cfg.DSN,
		Environment:      cfg.Environment,
		Release:          cfg.Release,
		SendDefaultPII:   false,
		AttachStacktrace: true,
		// Tracing goes to Langfuse through OpenTelemetry; Sentry is for errors only.
		EnableTracing: false,
		BeforeSend:    scrubEvent,
	})
	if err != nil {
		return noop, err
	}
	return func(timeout time.Duration) { sentry.Flush(timeout) }, nil
}

// scrubEvent drops anything that could identify a user or carry a secret, as a backstop to SendDefaultPII.
func scrubEvent(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if req := event.Request; req != nil {
		req.Cookies = ""
		req.Data = ""
		req.QueryString = ""
		req.Env = nil
		for name := range req.Headers {
			switch strings.ToLower(name) {
			case "authorization", "cookie", "x-forwarded-for", "x-real-ip", "apikey":
				delete(req.Headers, name)
			}
		}
	}
	event.User = sentry.User{}
	event.ServerName = ""
	return event
}

// Error categories, used as a Sentry tag to group failures.
const (
	CategoryHTTP     = "http"
	CategoryChat     = "chat"
	CategoryTool     = "tool"
	CategoryDocument = "document"
)

// Capture reports an unexpected error with its trace id, so a Sentry issue links to the Langfuse trace.
// Callers decide what is unexpected; user mistakes (bad input, not found, quota) should not be reported.
func Capture(ctx context.Context, err error, category string, tags map[string]string) {
	if err == nil {
		return
	}
	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub().Clone()
	}
	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("category", category)
		if id := TraceID(ctx); id != "" {
			scope.SetTag("trace_id", id)
		}
		for k, v := range tags {
			scope.SetTag(k, v)
		}
		hub.CaptureException(err)
	})
}

// CaptureRequest is Capture for an HTTP handler; it adds the route and method.
func CaptureRequest(r *http.Request, err error, category string) {
	tags := map[string]string{"method": r.Method}
	if route := routePattern(r); route != "" {
		tags["route"] = route
	}
	Capture(r.Context(), err, category, tags)
}
