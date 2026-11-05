package observability

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// TraceHeader carries the trace id back to the client, so a frontend error report can point at the trace.
const TraceHeader = "X-Trace-Id"

// HTTP traces one route. It is applied per route, so only the AI paths use Langfuse quota.
// It wraps the writer with chi's wrapper rather than otelhttp's, because chi's keeps Unwrap,
// which the chat stream needs to clear its write deadline.
func HTTP(route string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, span := Tracer().Start(r.Context(), fmt.Sprintf("%s %s", r.Method, route),
				trace.WithSpanKind(trace.SpanKindServer),
				trace.WithAttributes(
					AttrHTTPMethod.String(r.Method),
					AttrHTTPRoute.String(route),
					AttrHTTPRequestSize.Int64(r.ContentLength),
				),
			)
			defer span.End()

			if id := TraceID(ctx); id != "" {
				w.Header().Set(TraceHeader, id)
			}
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r.WithContext(ctx))

			status := ww.Status()
			if status == 0 {
				status = http.StatusOK
			}
			span.SetAttributes(AttrHTTPStatusCode.Int(status))
			if status >= 500 {
				span.SetStatus(codes.Error, http.StatusText(status))
				span.SetAttributes(AttrLevel.String("ERROR"))
			} else if status >= 400 {
				span.SetAttributes(AttrLevel.String("WARNING"))
			}
		})
	}
}
