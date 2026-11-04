package websearch

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTavilySearchSendsQueryAndParsesResults(t *testing.T) {
	var got tavilyRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/search" || r.Header.Get("Authorization") != "Bearer tvly-test" {
			t.Errorf("unexpected request: %s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		w.Write([]byte(`{"answer": "Go 1.27 is the latest release.", "results": [
			{"title": "Go releases", "url": "https://go.dev/doc/devel/release", "content": "go1.27 (released 2026-08-11)"},
			{"title": "dup", "url": "https://go.dev/doc/devel/release", "content": "again"},
			{"title": "no url", "url": "", "content": "x"},
			{"title": "Go blog", "url": "https://go.dev/blog", "content": "news"}
		]}`))
	}))
	defer srv.Close()

	res, err := NewTavily("tvly-test", srv.URL, 3).Search(context.Background(), "  latest go release ")
	if err != nil {
		t.Fatal(err)
	}
	if got.Query != "latest go release" || got.MaxResults != 3 || !got.IncludeAnswer || got.SearchDepth != "basic" {
		t.Errorf("request body = %+v", got)
	}
	if res.Answer != "Go 1.27 is the latest release." {
		t.Errorf("answer = %q", res.Answer)
	}
	if len(res.Sources) != 2 || res.Sources[0].Content != "go1.27 (released 2026-08-11)" || res.Sources[1].URL != "https://go.dev/blog" {
		t.Errorf("sources = %+v", res.Sources)
	}
}

func TestTavilyErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		quota  bool
	}{
		{http.StatusTooManyRequests, true},
		{432, true},
		{433, true},
		{http.StatusUnauthorized, false},
		{http.StatusInternalServerError, false},
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			w.Write([]byte(`{"detail": {"error": "nope"}}`))
		}))
		_, err := NewTavily("k", srv.URL, 5).Search(context.Background(), "q")
		srv.Close()
		if err == nil || errors.Is(err, ErrQuota) != tc.quota {
			t.Errorf("status %d: err = %v, want quota=%v", tc.status, err, tc.quota)
		}
	}
	if _, err := NewTavily("k", "http://unused", 5).Search(context.Background(), "  "); err == nil {
		t.Error("empty query should fail")
	}
}
