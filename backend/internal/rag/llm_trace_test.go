package rag

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/pagepilot/backend/internal/observability"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type cannedGroq struct {
	body string
	sent string
}

func (c *cannedGroq) RoundTrip(req *http.Request) (*http.Response, error) {
	b, _ := io.ReadAll(req.Body)
	c.sent = string(b)
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(c.body)), Header: http.Header{}}, nil
}

func generationSpan(t *testing.T, call func(ctx context.Context, g *GroqClient) error, body string) (sdktrace.ReadOnlySpan, string) {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec)))
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	canned := &cannedGroq{body: body}
	g := &GroqClient{apiKey: "test", model: "openai/gpt-oss-120b", httpClient: &http.Client{Transport: canned}}
	ctx, root := observability.StartRoot(context.Background(), trace.SpanContext{}, "test", observability.TypeSpan)
	if err := call(ctx, g); err != nil {
		t.Fatal(err)
	}
	root.End()
	for _, s := range rec.Ended() {
		if s.Name() == "chat openai/gpt-oss-120b" {
			return s, canned.sent
		}
	}
	t.Fatal("no generation span")
	return nil, ""
}

func attrs(s sdktrace.ReadOnlySpan) map[string]string {
	out := map[string]string{}
	for _, kv := range s.Attributes() {
		out[string(kv.Key)] = kv.Value.Emit()
	}
	return out
}

func TestGenerateRecordsReportedUsage(t *testing.T) {
	span, _ := generationSpan(t, func(ctx context.Context, g *GroqClient) error {
		_, err := g.Generate(ctx, []ChatMessage{{Role: "user", Content: "hi"}})
		return err
	}, `{"choices":[{"message":{"content":"hello"}}],"usage":{"prompt_tokens":11,"completion_tokens":5}}`)

	a := attrs(span)
	if a["gen_ai.usage.input_tokens"] != "11" || a["gen_ai.usage.output_tokens"] != "5" || a["langfuse.observation.metadata.usage_available"] != "true" {
		t.Errorf("usage attributes: %v", a)
	}
	if a["langfuse.observation.type"] != "generation" || a["gen_ai.request.model"] != "openai/gpt-oss-120b" {
		t.Errorf("generation attributes: %v", a)
	}
}

func TestStreamReadsUsageFromTheLastChunk(t *testing.T) {
	stream := strings.Join([]string{
		`data: {"choices":[{"delta":{"content":"Hel"}}]}`,
		`data: {"choices":[{"delta":{"content":"lo"}}]}`,
		`data: {"choices":[],"usage":{"prompt_tokens":20,"completion_tokens":2}}`,
		`data: [DONE]`,
	}, "\n\n") + "\n\n"
	var got string
	span, sent := generationSpan(t, func(ctx context.Context, g *GroqClient) error {
		var err error
		got, err = g.Stream(ctx, []ChatMessage{{Role: "user", Content: "hi"}}, func(string) error { return nil })
		return err
	}, stream)

	if got != "Hello" {
		t.Errorf("streamed text = %q", got)
	}
	if !strings.Contains(sent, `"include_usage":true`) {
		t.Errorf("stream request should ask for usage: %s", sent)
	}
	a := attrs(span)
	if a["gen_ai.usage.input_tokens"] != "20" || a["gen_ai.usage.output_tokens"] != "2" {
		t.Errorf("usage attributes: %v", a)
	}
}

func TestMissingUsageIsMarkedNotEstimated(t *testing.T) {
	span, _ := generationSpan(t, func(ctx context.Context, g *GroqClient) error {
		_, err := g.Generate(ctx, []ChatMessage{{Role: "user", Content: "hi"}})
		return err
	}, `{"choices":[{"message":{"content":"hello"}}]}`)

	a := attrs(span)
	if _, ok := a["gen_ai.usage.input_tokens"]; ok {
		t.Errorf("token counts must not be invented: %v", a)
	}
	if a["langfuse.observation.metadata.usage_available"] != "false" {
		t.Errorf("usage_available = %q", a["langfuse.observation.metadata.usage_available"])
	}
	if _, ok := a["langfuse.observation.output"]; ok {
		t.Error("output must not be recorded unless content capture is on")
	}
}
