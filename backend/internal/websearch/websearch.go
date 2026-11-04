// Package websearch looks things up on the web for the chat agent.
package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pagepilot/backend/internal/config"
)

// ErrQuota means the search provider's plan or rate limit is used up.
var ErrQuota = errors.New("web search limit reached; try again later")

// Source is one web page a search result is based on.
type Source struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"` // the relevant excerpt from the page
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

// New builds the configured searcher, or returns nil when web search is turned off.
func New(cfg *config.Config) Searcher {
	if cfg.WebSearchProvider != "tavily" {
		return nil
	}
	return NewTavily(cfg.TavilyAPIKey, "https://api.tavily.com", cfg.WebSearchMaxResults)
}

// Tavily calls Tavily's search API (https://docs.tavily.com), which is built for LLM agents.
type Tavily struct {
	apiKey     string
	baseURL    string
	maxResults int
	client     *http.Client
}

func NewTavily(apiKey, baseURL string, maxResults int) *Tavily {
	if maxResults <= 0 {
		maxResults = 5
	}
	return &Tavily{apiKey: apiKey, baseURL: strings.TrimRight(baseURL, "/"), maxResults: maxResults, client: &http.Client{Timeout: 30 * time.Second}}
}

type tavilyRequest struct {
	Query         string `json:"query"`
	SearchDepth   string `json:"search_depth"`
	MaxResults    int    `json:"max_results"`
	IncludeAnswer bool   `json:"include_answer"`
}

type tavilyResponse struct {
	Answer  string `json:"answer"`
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	} `json:"results"`
}

func (t *Tavily) Search(ctx context.Context, query string) (*Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("query is required")
	}
	body, err := json.Marshal(tavilyRequest{Query: query, SearchDepth: "basic", MaxResults: t.maxResults, IncludeAnswer: true})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.baseURL+"/search", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("web search: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("web search: reading response: %w", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusTooManyRequests, 432, 433: // 432/433: plan or pay-as-you-go limit
		return nil, ErrQuota
	case http.StatusUnauthorized:
		return nil, errors.New("web search: the Tavily API key is missing or invalid")
	default:
		return nil, fmt.Errorf("web search: Tavily returned %d: %s", resp.StatusCode, snippet(data))
	}

	var parsed tavilyResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("web search: unreadable response: %w", err)
	}
	res := &Result{Answer: strings.TrimSpace(parsed.Answer)}
	seen := map[string]bool{}
	for _, r := range parsed.Results {
		if r.URL == "" || seen[r.URL] {
			continue
		}
		seen[r.URL] = true
		res.Sources = append(res.Sources, Source{Title: r.Title, URL: r.URL, Content: r.Content})
	}
	return res, nil
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
