package embeddings

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/pagepilot/backend/internal/gemini"
	"google.golang.org/genai"
)

// ErrDailyQuota means the provider's daily allowance is used up (Gemini resets at midnight Pacific,
// Cloudflare Workers AI at 00:00 UTC).
var ErrDailyQuota = errors.New("daily embedding quota reached; processing can resume after the provider's daily reset")

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
	limiter    *gemini.Limiter
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
	return newGeminiEmbedder(cfg, call, gemini.NewLimiter(cfg.RPM, cfg.TPM, cfg.RPD)), nil
}

func newGeminiEmbedder(cfg GeminiConfig, call embedCall, limiter *gemini.Limiter) *GeminiEmbedder {
	return &GeminiEmbedder{
		model:      cfg.Model,
		dim:        cfg.Dimension,
		call:       call,
		limiter:    limiter,
		sleep:      gemini.SleepCtx,
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

// embed sends texts in as many calls as the rate limits need: each call carries as many of the
// remaining texts as fit in the current minute, so nothing waits for room it doesn't need.
func (g *GeminiEmbedder) embed(ctx context.Context, texts []string, taskType string) ([][]float32, error) {
	tokens := make([]int, len(texts))
	for i, t := range texts {
		tokens[i] = estimateTokens(t)
	}
	out := make([][]float32, 0, len(texts))
	for len(texts) > 0 {
		n, err := g.limiter.Admit(ctx, tokens)
		if errors.Is(err, gemini.ErrDailyQuota) {
			return nil, ErrDailyQuota
		}
		if err != nil {
			return nil, err
		}
		vecs, err := g.callWithRetry(ctx, texts[:n], taskType)
		if err != nil {
			return nil, err
		}
		out = append(out, vecs...)
		texts, tokens = texts[n:], tokens[n:]
	}
	return out, nil
}

// callWithRetry makes one admitted call, retrying if Google still reports a per-minute limit.
func (g *GeminiEmbedder) callWithRetry(ctx context.Context, texts []string, taskType string) ([][]float32, error) {
	for attempt := 0; ; attempt++ {
		vecs, err := g.call(ctx, texts, taskType)
		if err == nil {
			return g.validate(vecs, len(texts))
		}

		apiErr, ok := gemini.AsAPIError(err)
		if !ok || apiErr.Code != 429 {
			return nil, fmt.Errorf("gemini embedding request failed: %w", err)
		}
		if gemini.IsDailyQuotaError(apiErr) {
			g.limiter.ExhaustDay()
			return nil, ErrDailyQuota
		}
		if attempt >= g.maxRetries {
			return nil, fmt.Errorf("gemini embedding still rate limited after %d retries: %w", g.maxRetries, err)
		}
		delay := gemini.RetryDelay(apiErr, attempt)
		log.Printf("[embeddings] Gemini rate limit hit for a batch of %d texts; retrying in %s: %s", len(texts), delay, apiErr.Message)
		if err := g.sleep(ctx, delay); err != nil {
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
func estimateTokens(text string) int {
	return len(text)/4 + 1
}
