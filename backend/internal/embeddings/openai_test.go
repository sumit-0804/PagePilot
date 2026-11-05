package embeddings

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeProvider answers like an OpenAI-compatible /embeddings endpoint, after any scripted failures.
func fakeProvider(t *testing.T, failures []func(w http.ResponseWriter)) (*OpenAIEmbedder, *atomic.Int32, *[]string) {
	t.Helper()
	var calls atomic.Int32
	var inputs []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := int(calls.Add(1))
		if r.URL.Path != "/ai/v1/embeddings" || r.Header.Get("Authorization") != "Bearer token" {
			http.Error(w, "bad request path or auth", http.StatusBadRequest)
			return
		}
		if n <= len(failures) {
			failures[n-1](w)
			return
		}
		var req struct {
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		inputs = append(inputs, req.Input...)
		type item struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		resp := struct {
			Data []item `json:"data"`
		}{}
		for i := range req.Input {
			resp.Data = append(resp.Data, item{Index: i, Embedding: []float32{float32(i), 1, 0}})
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	e := NewOpenAIEmbedder(srv.URL+"/ai/v1", "token", "@cf/qwen/qwen3-embedding-0.6b", 3, "Instruct: find passages\nQuery:")
	e.backoff = time.Millisecond
	return e, &calls, &inputs
}

func status(code int, body string) func(w http.ResponseWriter) {
	return func(w http.ResponseWriter) { http.Error(w, body, code) }
}

func TestCloudflareDailyAllowanceIsAQuotaError(t *testing.T) {
	e, calls, _ := fakeProvider(t, []func(http.ResponseWriter){
		status(429, `{"errors":[{"code":3036,"message":"You have used up your daily free allocation of 10,000 neurons."}]}`),
	})
	_, err := e.EmbedDocuments(context.Background(), []string{"a"})
	if !errors.Is(err, ErrDailyQuota) {
		t.Fatalf("got %v, want ErrDailyQuota", err)
	}
	if calls.Load() != 1 {
		t.Errorf("a used-up daily allowance must not be retried; calls = %d", calls.Load())
	}
}

func TestBusyAndFailingProvidersAreRetried(t *testing.T) {
	e, calls, _ := fakeProvider(t, []func(http.ResponseWriter){
		status(429, `{"errors":[{"code":3040,"message":"Capacity temporarily exceeded, please try again."}]}`),
		status(503, "unavailable"),
	})
	vecs, err := e.EmbedDocuments(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vecs) != 2 || vecs[1][0] != 1 || calls.Load() != 3 {
		t.Errorf("vectors %v after %d calls", vecs, calls.Load())
	}
}

func TestRetriesGiveUp(t *testing.T) {
	var fails []func(http.ResponseWriter)
	for i := 0; i <= maxEmbedRetries; i++ {
		fails = append(fails, status(502, "bad gateway"))
	}
	e, calls, _ := fakeProvider(t, fails)
	_, err := e.EmbedDocuments(context.Background(), []string{"a"})
	if err == nil || !strings.Contains(err.Error(), "502") || int(calls.Load()) != maxEmbedRetries+1 {
		t.Errorf("err %v after %d calls", err, calls.Load())
	}
}

func TestClientErrorsAreNotRetried(t *testing.T) {
	e, calls, _ := fakeProvider(t, []func(http.ResponseWriter){status(401, "invalid token")})
	_, err := e.EmbedDocuments(context.Background(), []string{"a"})
	if err == nil || !strings.Contains(err.Error(), "401") || calls.Load() != 1 {
		t.Errorf("err %v after %d calls", err, calls.Load())
	}
}

func TestQueryPrefixIsOnlyForQueries(t *testing.T) {
	e, _, inputs := fakeProvider(t, nil)
	ctx := context.Background()
	if _, err := e.EmbedQuery(ctx, "what is osmosis?"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.EmbedDocuments(ctx, []string{"Osmosis is the diffusion of water."}); err != nil {
		t.Fatal(err)
	}
	want := []string{"Instruct: find passages\nQuery:what is osmosis?", "Osmosis is the diffusion of water."}
	if len(*inputs) != 2 || (*inputs)[0] != want[0] || (*inputs)[1] != want[1] {
		t.Errorf("inputs sent = %q, want %q", *inputs, want)
	}
}
