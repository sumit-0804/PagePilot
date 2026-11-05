package agents

import (
	"context"
	"strings"
	"testing"

	"github.com/pagepilot/backend/internal/observability"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/adk/v2/model"
)

// TestAgentTurnIsOneTrace checks that ADK's spans and ours nest inside the request's trace:
// request -> agent -> tool -> retrieval -> embedding and vector search.
func TestAgentTurnIsOneTrace(t *testing.T) {
	f := setup(t, jsonLLM{})
	// ADK binds its tracer to the first global provider, so this package installs exactly one.
	rec := tracetest.NewSpanRecorder()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))

	m := &scriptedModel{steps: []func(*model.LLMRequest) ([]*model.LLMResponse, error){
		callTool("search_documents", map[string]any{"query": "who first described cells"}),
		answer("Robert Hooke [1]."),
	}}
	ctx, root := observability.StartRoot(context.Background(), trace.SpanContext{}, "POST /api/chat", observability.TypeSpan)
	if _, err := f.engine(t, m).Respond(ctx, f.request("Who first described cells?", ""), func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	root.End()

	byID := map[trace.SpanID]sdktrace.ReadOnlySpan{}
	var names []string
	for _, s := range rec.Ended() {
		if s.SpanContext().TraceID() == root.SpanContext().TraceID() {
			byID[s.SpanContext().SpanID()] = s
			names = append(names, s.Name())
		}
	}
	find := func(prefix string) sdktrace.ReadOnlySpan {
		t.Helper()
		for _, s := range byID {
			if strings.HasPrefix(s.Name(), prefix) {
				return s
			}
		}
		t.Fatalf("no %q span in the trace; got %v", prefix, names)
		return nil
	}
	// descends reports whether span sits somewhere below ancestor.
	descends := func(span, ancestor sdktrace.ReadOnlySpan) bool {
		for p := span.Parent(); p.IsValid(); {
			if p.SpanID() == ancestor.SpanContext().SpanID() {
				return true
			}
			parent, ok := byID[p.SpanID()]
			if !ok {
				return false
			}
			p = parent.Parent()
		}
		return false
	}

	request := byID[root.SpanContext().SpanID()]
	agent := find("invoke_agent " + agentName)
	generation := find("generate_content")
	tool := find("execute_tool search_documents")
	retrieve := find("rag.retrieve")
	embed := find("embed_query")
	search := find("vector_search")

	for _, c := range []struct{ child, parent sdktrace.ReadOnlySpan }{
		{agent, request}, {generation, agent}, {tool, agent}, {retrieve, tool}, {embed, retrieve}, {search, retrieve},
	} {
		if !descends(c.child, c.parent) {
			t.Errorf("%q should be under %q", c.child.Name(), c.parent.Name())
		}
	}
	for _, kv := range retrieve.Attributes() {
		if kv.Key == observability.AttrInput {
			t.Error("the search query must not be recorded unless content capture is on")
		}
	}
}
