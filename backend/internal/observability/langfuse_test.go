package observability

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLangfuseFeedbackScores(t *testing.T) {
	type call struct {
		method, path, user, pass string
		body                     map[string]any
	}
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, _ := r.BasicAuth()
		c := call{method: r.Method, path: r.URL.Path, user: u, pass: p}
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			_ = json.Unmarshal(b, &c.body)
		}
		calls = append(calls, c)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if NewLangfuseClient(Config{LangfusePublicKey: "pk"}) != nil {
		t.Error("a client needs both keys")
	}
	lf := NewLangfuseClient(Config{LangfusePublicKey: "pk-lf", LangfuseSecretKey: "sk-lf", LangfuseBaseURL: srv.URL + "/", Environment: "test"})
	ctx := context.Background()
	if err := lf.Score(ctx, "trace1", "msg1", -1, "incorrect"); err != nil {
		t.Fatal(err)
	}
	if err := lf.Remove(ctx, "msg1"); err != nil {
		t.Fatal(err)
	}

	if len(calls) != 2 {
		t.Fatalf("got %d calls", len(calls))
	}
	score := calls[0]
	if score.method != http.MethodPost || score.path != "/api/public/scores" || score.user != "pk-lf" || score.pass != "sk-lf" {
		t.Errorf("score request: %+v", score)
	}
	want := map[string]any{"id": "feedback-msg1", "traceId": "trace1", "name": "user_feedback", "value": 0.0, "dataType": "BOOLEAN", "comment": "incorrect", "environment": "test"}
	for k, v := range want {
		if score.body[k] != v {
			t.Errorf("score %s = %v, want %v", k, score.body[k], v)
		}
	}
	if calls[1].method != http.MethodDelete || calls[1].path != "/api/public/scores/feedback-msg1" {
		t.Errorf("remove request: %+v", calls[1])
	}
}

func TestLangfuseErrorsAreReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "invalid traceId", http.StatusBadRequest)
	}))
	defer srv.Close()
	lf := NewLangfuseClient(Config{LangfusePublicKey: "a", LangfuseSecretKey: "b", LangfuseBaseURL: srv.URL})
	if err := lf.Score(context.Background(), "t", "m", 1, ""); err == nil {
		t.Error("a 400 from Langfuse should be an error")
	}
}
