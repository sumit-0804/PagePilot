package embeddings

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // Windows has no system zoneinfo; needed for the Pacific-time daily reset

	"golang.org/x/time/rate"
	"google.golang.org/genai"
)

// ErrDailyQuota means the requests-per-day limit is used up until the reset at midnight Pacific time.
var ErrDailyQuota = errors.New("daily embedding quota reached; processing can resume after the quota resets at midnight Pacific time")

const (
	taskDocument = "RETRIEVAL_DOCUMENT"
	taskQuery    = "RETRIEVAL_QUERY"
)

type GeminiConfig struct {
	APIKey    string
	Model     string
	Dimension int
	RPM       int
	TPM       int
	RPD       int
}

// embedCall is the raw API call, swappable in tests.
type embedCall func(ctx context.Context, texts []string, taskType string) ([][]float32, error)

type GeminiEmbedder struct {
	model      string
	dim        int
	call       embedCall
	limiter    *Limiter
	sleep      func(ctx context.Context, d time.Duration) error
	maxRetries int
}

func NewGeminiEmbedder(ctx context.Context, cfg GeminiConfig) (*GeminiEmbedder, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: cfg.APIKey, Backend: genai.BackendGeminiAPI})
	if err != nil {
		return nil, fmt.Errorf("creating gemini client: %w", err)
	}
	dim := int32(cfg.Dimension)
	call := func(ctx context.Context, texts []string, taskType string) ([][]float32, error) {
		contents := make([]*genai.Content, len(texts))
		for i, t := range texts {
			contents[i] = genai.NewContentFromText(t, genai.RoleUser)
		}
		resp, err := client.Models.EmbedContent(ctx, cfg.Model, contents, &genai.EmbedContentConfig{
			TaskType:             taskType,
			OutputDimensionality: &dim,
		})
		if err != nil {
			return nil, err
		}
		out := make([][]float32, len(resp.Embeddings))
		for i, e := range resp.Embeddings {
			out[i] = e.Values
		}
		return out, nil
	}
	return newGeminiEmbedder(cfg, call, NewLimiter(cfg.RPM, cfg.TPM, cfg.RPD)), nil
}

func newGeminiEmbedder(cfg GeminiConfig, call embedCall, limiter *Limiter) *GeminiEmbedder {
	return &GeminiEmbedder{
		model:      cfg.Model,
		dim:        cfg.Dimension,
		call:       call,
		limiter:    limiter,
		sleep:      sleepCtx,
		maxRetries: 5,
	}
}

func (g *GeminiEmbedder) Dimension() int { return g.dim }
func (g *GeminiEmbedder) Model() string  { return g.model }

func (g *GeminiEmbedder) EmbedDocuments(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return [][]float32{}, nil
	}
	return g.embed(ctx, texts, taskDocument)
}

func (g *GeminiEmbedder) EmbedQuery(ctx context.Context, text string) ([]float32, error) {
	vecs, err := g.embed(ctx, []string{text}, taskQuery)
	if err != nil {
		return nil, err
	}
	return vecs[0], nil
}

func (g *GeminiEmbedder) embed(ctx context.Context, texts []string, taskType string) ([][]float32, error) {
	tokens := estimateTokens(texts)
	// A batch bigger than the per-minute token budget can never be admitted, so split it.
	if tokens > g.limiter.MaxTokensPerRequest() {
		if len(texts) == 1 {
			return nil, fmt.Errorf("text of ~%d tokens exceeds the per-minute token limit (%d)", tokens, g.limiter.MaxTokensPerRequest())
		}
		mid := len(texts) / 2
		first, err := g.embed(ctx, texts[:mid], taskType)
		if err != nil {
			return nil, err
		}
		second, err := g.embed(ctx, texts[mid:], taskType)
		if err != nil {
			return nil, err
		}
		return append(first, second...), nil
	}

	for attempt := 0; ; attempt++ {
		if err := g.limiter.Wait(ctx, tokens); err != nil {
			return nil, err
		}
		vecs, err := g.call(ctx, texts, taskType)
		if err == nil {
			return g.validate(vecs, len(texts))
		}

		apiErr, ok := asAPIError(err)
		if !ok || apiErr.Code != 429 {
			return nil, fmt.Errorf("gemini embedding request failed: %w", err)
		}
		if isDailyQuotaError(apiErr) {
			g.limiter.ExhaustDay()
			return nil, ErrDailyQuota
		}
		if attempt >= g.maxRetries {
			return nil, fmt.Errorf("gemini embedding still rate limited after %d retries: %w", g.maxRetries, err)
		}
		if err := g.sleep(ctx, retryDelay(apiErr, attempt)); err != nil {
			return nil, err
		}
	}
}

func (g *GeminiEmbedder) validate(vecs [][]float32, want int) ([][]float32, error) {
	if len(vecs) != want {
		return nil, fmt.Errorf("gemini returned %d embeddings for %d inputs", len(vecs), want)
	}
	for i, v := range vecs {
		if len(v) != g.dim {
			return nil, fmt.Errorf("gemini embedding %d has %d dimensions, expected %d", i, len(v), g.dim)
		}
	}
	return vecs, nil
}

// estimateTokens uses ~4 bytes per token, which over-counts non-English text (the safe side).
func estimateTokens(texts []string) int {
	total := 0
	for _, t := range texts {
		total += len(t)/4 + 1
	}
	return total
}

func asAPIError(err error) (genai.APIError, bool) {
	var v genai.APIError
	if errors.As(err, &v) {
		return v, true
	}
	var p *genai.APIError
	if errors.As(err, &p) && p != nil {
		return *p, true
	}
	return genai.APIError{}, false
}

// isDailyQuotaError checks Google's quota id (e.g. "...RequestsPerDayPerProject...") in the error.
func isDailyQuotaError(e genai.APIError) bool {
	return strings.Contains(e.Message, "PerDay") || strings.Contains(fmt.Sprint(e.Details), "PerDay")
}

// retryDelay honors the server's RetryInfo when present, otherwise backs off exponentially.
func retryDelay(e genai.APIError, attempt int) time.Duration {
	for _, d := range e.Details {
		if raw, ok := d["retryDelay"].(string); ok {
			if dur, err := time.ParseDuration(raw); err == nil && dur > 0 {
				return dur
			}
		}
	}
	delay := 2 * time.Second << attempt
	if delay > time.Minute {
		delay = time.Minute
	}
	return delay
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Limiter enforces requests/minute, tokens/minute and requests/day. The daily count is in memory,
// so a restart resets it; a 429 from Google still stops processing correctly.
type Limiter struct {
	rpm *rate.Limiter
	tpm *rate.Limiter
	rpd int

	mu      sync.Mutex
	dayKey  string
	dayUsed int
	now     func() time.Time
	pacific *time.Location
}

func NewLimiter(rpm, tpm, rpd int) *Limiter {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.FixedZone("PST", -8*60*60)
	}
	return &Limiter{
		rpm:     rate.NewLimiter(rate.Limit(float64(rpm)/60), 1),
		tpm:     rate.NewLimiter(rate.Limit(float64(tpm)/60), tpm),
		rpd:     rpd,
		now:     time.Now,
		pacific: loc,
	}
}

func (l *Limiter) MaxTokensPerRequest() int { return l.tpm.Burst() }

// Wait blocks until a request of the given size is allowed, or fails fast when the day's quota is gone.
func (l *Limiter) Wait(ctx context.Context, tokens int) error {
	if err := l.takeDaily(); err != nil {
		return err
	}
	if err := l.rpm.Wait(ctx); err != nil {
		return err
	}
	return l.tpm.WaitN(ctx, tokens)
}

// ExhaustDay marks today's quota as used, after Google reports it is gone.
func (l *Limiter) ExhaustDay() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rollDay()
	l.dayUsed = l.rpd
}

func (l *Limiter) takeDaily() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rollDay()
	if l.dayUsed >= l.rpd {
		return ErrDailyQuota
	}
	l.dayUsed++
	return nil
}

func (l *Limiter) rollDay() {
	key := l.now().In(l.pacific).Format("2006-01-02")
	if key != l.dayKey {
		l.dayKey = key
		l.dayUsed = 0
	}
}
