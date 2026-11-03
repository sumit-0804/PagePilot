package embeddings

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
	"google.golang.org/genai"
)

func fakeVectors(n, dim int) [][]float32 {
	out := make([][]float32, n)
	for i := range out {
		out[i] = make([]float32, dim)
	}
	return out
}

func newTestEmbedder(call embedCall, rpm, tpm, rpd int) *GeminiEmbedder {
	g := newGeminiEmbedder(GeminiConfig{Model: "test", Dimension: 4}, call, NewLimiter(rpm, tpm, rpd))
	g.sleep = func(context.Context, time.Duration) error { return nil }
	return g
}

func TestDailyQuotaStopsRequests(t *testing.T) {
	calls := 0
	g := newTestEmbedder(func(_ context.Context, texts []string, _ string) ([][]float32, error) {
		calls++
		return fakeVectors(len(texts), 4), nil
	}, 6000, 100000, 2)

	for i := 0; i < 2; i++ {
		if _, err := g.EmbedQuery(context.Background(), "hello"); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if _, err := g.EmbedQuery(context.Background(), "hello"); !errors.Is(err, ErrDailyQuota) {
		t.Fatalf("expected ErrDailyQuota, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected 2 API calls, got %d", calls)
	}
}

func TestDailyQuotaResetsNextPacificDay(t *testing.T) {
	g := newTestEmbedder(func(_ context.Context, texts []string, _ string) ([][]float32, error) {
		return fakeVectors(len(texts), 4), nil
	}, 6000, 100000, 1)
	now := time.Date(2026, 11, 3, 23, 0, 0, 0, g.limiter.pacific)
	g.limiter.now = func() time.Time { return now }

	if _, err := g.EmbedQuery(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.EmbedQuery(context.Background(), "b"); !errors.Is(err, ErrDailyQuota) {
		t.Fatalf("expected ErrDailyQuota, got %v", err)
	}
	now = now.Add(2 * time.Hour) // past midnight Pacific
	if _, err := g.EmbedQuery(context.Background(), "c"); err != nil {
		t.Fatalf("expected quota reset, got %v", err)
	}
}

func TestRateLimitedRequestIsRetried(t *testing.T) {
	calls := 0
	var slept []time.Duration
	g := newTestEmbedder(func(_ context.Context, texts []string, _ string) ([][]float32, error) {
		calls++
		if calls == 1 {
			return nil, genai.APIError{Code: 429, Message: "Resource exhausted", Details: []map[string]any{{"retryDelay": "7s"}}}
		}
		return fakeVectors(len(texts), 4), nil
	}, 6000, 100000, 100)
	g.sleep = func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }

	if _, err := g.EmbedDocuments(context.Background(), []string{"x", "y"}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(slept) != 1 || slept[0] != 7*time.Second {
		t.Fatalf("expected one retry after 7s, got calls=%d slept=%v", calls, slept)
	}
}

func TestDailyQuota429FailsImmediately(t *testing.T) {
	g := newTestEmbedder(func(context.Context, []string, string) ([][]float32, error) {
		return nil, genai.APIError{Code: 429, Message: "Quota exceeded for metric EmbedContentRequestsPerDayPerProjectPerModel"}
	}, 6000, 100000, 100)

	if _, err := g.EmbedQuery(context.Background(), "x"); !errors.Is(err, ErrDailyQuota) {
		t.Fatalf("expected ErrDailyQuota, got %v", err)
	}
	if _, err := g.EmbedQuery(context.Background(), "x"); !errors.Is(err, ErrDailyQuota) {
		t.Fatalf("expected local quota to stay exhausted, got %v", err)
	}
}

func TestOversizedBatchIsSplit(t *testing.T) {
	var batchSizes []int
	g := newTestEmbedder(func(_ context.Context, texts []string, _ string) ([][]float32, error) {
		batchSizes = append(batchSizes, len(texts))
		return fakeVectors(len(texts), 4), nil
	}, 6000, 60, 100)
	g.limiter.tpm.SetLimit(rate.Inf) // keep the 60-token burst that forces the split, without real waiting

	texts := []string{strings.Repeat("a", 100), strings.Repeat("b", 100), strings.Repeat("c", 100)} // ~26 tokens each
	vecs, err := g.EmbedDocuments(context.Background(), texts)
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 3 || len(batchSizes) < 2 {
		t.Fatalf("expected the batch to be split, got sizes %v", batchSizes)
	}
}

func TestWrongDimensionIsRejected(t *testing.T) {
	g := newTestEmbedder(func(_ context.Context, texts []string, _ string) ([][]float32, error) {
		return fakeVectors(len(texts), 3), nil
	}, 6000, 100000, 100)
	if _, err := g.EmbedQuery(context.Background(), "x"); err == nil {
		t.Fatal("expected a dimension error")
	}
}

func TestTaskTypesDifferForQueriesAndDocuments(t *testing.T) {
	var tasks []string
	g := newTestEmbedder(func(_ context.Context, texts []string, task string) ([][]float32, error) {
		tasks = append(tasks, task)
		return fakeVectors(len(texts), 4), nil
	}, 6000, 100000, 100)
	_, _ = g.EmbedDocuments(context.Background(), []string{"doc"})
	_, _ = g.EmbedQuery(context.Background(), "query")
	if len(tasks) != 2 || tasks[0] != taskDocument || tasks[1] != taskQuery {
		t.Fatalf("unexpected task types %v", tasks)
	}
}
