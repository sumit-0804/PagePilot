// Package websearch looks things up on the web for the chat agent.
package websearch

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/pagepilot/backend/internal/config"
	"github.com/pagepilot/backend/internal/gemini"
	"google.golang.org/genai"
)

// Source is one web page a search result is based on.
type Source struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Result is a short answer to the query plus the pages it came from.
type Result struct {
	Answer  string
	Sources []Source
}

// Searcher answers a query from the live web.
type Searcher interface {
	Search(ctx context.Context, query string) (*Result, error)
}

// askFunc runs one grounded model call; it is swapped out in tests.
type askFunc func(ctx context.Context, query string) (*genai.GenerateContentResponse, error)

// GeminiSearcher uses Gemini with Google Search grounding.
type GeminiSearcher struct {
	ask     askFunc
	limiter *gemini.Limiter
}

// New builds the configured searcher, or returns nil when web search is turned off.
func New(ctx context.Context, cfg *config.Config) (Searcher, error) {
	if cfg.WebSearchProvider != "gemini" {
		return nil, nil
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: cfg.GeminiAPIKey, Backend: genai.BackendGeminiAPI})
	if err != nil {
		return nil, fmt.Errorf("creating gemini client: %w", err)
	}
	ask := func(ctx context.Context, query string) (*genai.GenerateContentResponse, error) {
		return client.Models.GenerateContent(ctx, cfg.WebSearchModel, genai.Text(searchPrompt(query)), &genai.GenerateContentConfig{
			Temperature: genai.Ptr[float32](0.2),
			Tools:       []*genai.Tool{{GoogleSearch: &genai.GoogleSearch{}}},
		})
	}
	// Grounded calls are limited by request count; tokens are not the bottleneck here.
	return newGeminiSearcher(ask, gemini.NewLimiter(cfg.WebSearchRPM, 1<<30, cfg.WebSearchRPD)), nil
}

func newGeminiSearcher(ask askFunc, limiter *gemini.Limiter) *GeminiSearcher {
	return &GeminiSearcher{ask: ask, limiter: limiter}
}

func (g *GeminiSearcher) Search(ctx context.Context, query string) (*Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("query is required")
	}
	if _, err := g.limiter.Admit(ctx, []int{1}); err != nil {
		return nil, err
	}
	resp, err := g.ask(ctx, query)
	if err != nil {
		if apiErr, ok := gemini.AsAPIError(err); ok && apiErr.Code == 429 && gemini.IsDailyQuotaError(apiErr) {
			g.limiter.ExhaustDay()
			return nil, gemini.ErrDailyQuota
		}
		return nil, fmt.Errorf("web search: %w", err)
	}
	return parse(resp), nil
}

func parse(resp *genai.GenerateContentResponse) *Result {
	res := &Result{Answer: strings.TrimSpace(resp.Text())}
	if len(resp.Candidates) == 0 || resp.Candidates[0].GroundingMetadata == nil {
		return res
	}
	seen := map[string]bool{}
	for _, c := range resp.Candidates[0].GroundingMetadata.GroundingChunks {
		if c == nil || c.Web == nil || c.Web.URI == "" || seen[c.Web.URI] {
			continue
		}
		seen[c.Web.URI] = true
		res.Sources = append(res.Sources, Source{Title: c.Web.Title, URL: c.Web.URI})
	}
	return res
}

func searchPrompt(query string) string {
	return "Search the web and answer concisely with the key facts, dates and numbers. " +
		"Say plainly if the results don't answer it.\n\nQuery: " + query
}
