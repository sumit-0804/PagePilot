package websearch

import (
	"context"
	"errors"
	"testing"

	"github.com/pagepilot/backend/internal/gemini"
	"google.golang.org/genai"
)

func TestSearchReturnsAnswerAndUniqueSources(t *testing.T) {
	ask := func(context.Context, string) (*genai.GenerateContentResponse, error) {
		return &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
			Content: genai.NewContentFromText("Go 1.27 is the latest release.", genai.RoleModel),
			GroundingMetadata: &genai.GroundingMetadata{GroundingChunks: []*genai.GroundingChunk{
				{Web: &genai.GroundingChunkWeb{Title: "go.dev", URI: "https://go.dev/doc/devel/release"}},
				{Web: &genai.GroundingChunkWeb{Title: "go.dev", URI: "https://go.dev/doc/devel/release"}},
				{Web: &genai.GroundingChunkWeb{Title: "blog", URI: "https://go.dev/blog"}},
				{},
			}},
		}}}, nil
	}
	s := newGeminiSearcher(ask, gemini.NewLimiter(10, 1<<30, 10))

	res, err := s.Search(context.Background(), "latest go release")
	if err != nil {
		t.Fatal(err)
	}
	if res.Answer != "Go 1.27 is the latest release." {
		t.Errorf("answer = %q", res.Answer)
	}
	if len(res.Sources) != 2 || res.Sources[1].URL != "https://go.dev/blog" {
		t.Errorf("sources = %+v", res.Sources)
	}
}

func TestSearchStopsAtDailyQuota(t *testing.T) {
	calls := 0
	ask := func(context.Context, string) (*genai.GenerateContentResponse, error) {
		calls++
		return &genai.GenerateContentResponse{}, nil
	}
	s := newGeminiSearcher(ask, gemini.NewLimiter(10, 1<<30, 1))
	if _, err := s.Search(context.Background(), "a"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Search(context.Background(), "b"); !errors.Is(err, gemini.ErrDailyQuota) {
		t.Fatalf("err = %v, want daily quota", err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
	if _, err := s.Search(context.Background(), "  "); err == nil {
		t.Error("empty query should fail")
	}
}
