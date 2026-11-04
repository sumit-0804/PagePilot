package embeddings

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pagepilot/backend/internal/gemini"
	"google.golang.org/genai"
)

func fakeVectors(n, dim int) [][]float32 {
	out := make([][]float32, n)
	for i := range out {
		out[i] = make([]float32, dim)
	}
	return out
}

// fakeClock lets limiter waits happen instantly while still moving time forward.
type fakeClock struct {
	now   time.Time
	slept time.Duration
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Sleep(_ context.Context, d time.Duration) error {
	c.now = c.now.Add(d)
	c.slept += d
	return nil
}

func newTestEmbedder(call embedCall, rpm, tpm, rpd int) *GeminiEmbedder {
	g, _ := newTestEmbedderWithClock(call, rpm, tpm, rpd)
	return g
}

func newTestEmbedderWithClock(call embedCall, rpm, tpm, rpd int) (*GeminiEmbedder, *fakeClock) {
	clock := &fakeClock{now: time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)}
	limiter := gemini.NewLimiter(rpm, tpm, rpd)
	limiter.SetClock(clock.Now, clock.Sleep)
	g := newGeminiEmbedder(GeminiConfig{Model: "test", Dimension: 4}, call, limiter)
	g.sleep = func(context.Context, time.Duration) error { return nil }
	return g, clock
}

func countingCall(sizes *[]int) embedCall {
	return func(_ context.Context, texts []string, _ string) ([][]float32, error) {
		*sizes = append(*sizes, len(texts))
		return fakeVectors(len(texts), 4), nil
	}
}

func manyTexts(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "short passage"
	}
	return out
}

func TestEachTextCountsTowardRequestsPerMinute(t *testing.T) {
	var sizes []int
	g, clock := newTestEmbedderWithClock(countingCall(&sizes), 100, 1_000_000, 10_000)

	if _, err := g.EmbedDocuments(context.Background(), manyTexts(100)); err != nil {
		t.Fatal(err)
	}
	if clock.slept != 0 {
		t.Fatalf("the first full batch should go straight through, waited %s", clock.slept)
	}
	if _, err := g.EmbedDocuments(context.Background(), manyTexts(52)); err != nil {
		t.Fatal(err)
	}
	// 100 texts used the whole minute, so the next 52 must wait for that minute to pass.
	if clock.slept < 59*time.Second || clock.slept > 61*time.Second {
		t.Fatalf("expected about a minute of waiting, got %s", clock.slept)
	}
}

func TestSendsWhatFitsInsteadOfWaitingForTheWholeBatch(t *testing.T) {
	var sizes []int
	g, clock := newTestEmbedderWithClock(countingCall(&sizes), 100, 1_000_000, 10_000)
	if _, err := g.EmbedQuery(context.Background(), "startup check"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.EmbedDocuments(context.Background(), manyTexts(100)); err != nil {
		t.Fatal(err)
	}
	if len(sizes) != 3 || sizes[1] != 99 || sizes[2] != 1 {
		t.Fatalf("expected 99 texts at once, then 1 when the window frees up; got calls %v", sizes)
	}
	if clock.slept > 61*time.Second {
		t.Fatalf("waited longer than one window: %s", clock.slept)
	}
}

func TestBatchLargerThanRequestsPerMinuteIsSplit(t *testing.T) {
	var sizes []int
	g := newTestEmbedder(countingCall(&sizes), 10, 1_000_000, 10_000)
	vecs, err := g.EmbedDocuments(context.Background(), manyTexts(25))
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 25 {
		t.Fatalf("expected 25 vectors, got %d", len(vecs))
	}
	for _, s := range sizes {
		if s > 10 {
			t.Fatalf("no call may carry more texts than the per-minute limit, got sizes %v", sizes)
		}
	}
}

func TestDailyQuotaIsChargedPerText(t *testing.T) {
	var sizes []int
	g := newTestEmbedder(countingCall(&sizes), 1000, 1_000_000, 150)
	if _, err := g.EmbedDocuments(context.Background(), manyTexts(100)); err != nil {
		t.Fatal(err)
	}
	if _, err := g.EmbedDocuments(context.Background(), manyTexts(100)); !errors.Is(err, ErrDailyQuota) {
		t.Fatalf("100 + 100 texts exceed a 150-per-day quota, expected ErrDailyQuota, got %v", err)
	}
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
	pacific, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 3, 23, 0, 0, 0, pacific)
	g.limiter.SetClock(func() time.Time { return now }, gemini.SleepCtx)

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
