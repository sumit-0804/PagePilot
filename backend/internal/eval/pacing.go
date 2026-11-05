package eval

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// PacedTransport holds a chat request when Groq's last reply for that model said too few tokens are
// left this minute, until its stated reset time. Groq's free tier gives each model 8,000 tokens a
// minute, so cases running at once queue up instead of failing; the clients still retry any 429.
type PacedTransport struct {
	Base http.RoundTripper
	mu   sync.Mutex
	// Per model: tokens left this minute (minus requests in flight) and when the minute's count refills.
	left    map[string]int
	resetAt map[string]time.Time
	now     func() time.Time
}

func NewPacedTransport() *PacedTransport {
	return &PacedTransport{Base: http.DefaultTransport, left: map[string]int{}, resetAt: map[string]time.Time{}, now: time.Now}
}

func (p *PacedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body == nil || req.Method != http.MethodPost {
		return p.Base.RoundTrip(req)
	}
	body, err := io.ReadAll(req.Body)
	req.Body.Close()
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }

	var head struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &head)
	// A rough guess (JSON runs about four bytes a token); it only decides whether to wait.
	if err := p.reserve(req.Context(), head.Model, len(body)/4); err != nil {
		return nil, err
	}
	resp, err := p.Base.RoundTrip(req)
	if err == nil {
		p.update(head.Model, resp.Header)
	}
	return resp, err
}

// reserve waits until the model's minute has room for need tokens, or Groq has said it refilled.
func (p *PacedTransport) reserve(ctx context.Context, model string, need int) error {
	for {
		p.mu.Lock()
		reset, known := p.resetAt[model]
		wait := reset.Sub(p.now())
		if !known || wait <= 0 || p.left[model] >= need {
			p.left[model] -= need
			p.mu.Unlock()
			return nil
		}
		p.mu.Unlock()
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
			p.mu.Lock()
			delete(p.resetAt, model) // refilled; the next reply will say how much is left
			p.mu.Unlock()
		}
	}
}

// update records what Groq reported: tokens left this minute and the time until they are refilled.
func (p *PacedTransport) update(model string, h http.Header) {
	left, err1 := strconv.Atoi(strings.TrimSpace(h.Get("x-ratelimit-remaining-tokens")))
	reset, err2 := time.ParseDuration(strings.TrimSpace(h.Get("x-ratelimit-reset-tokens")))
	if err1 != nil || err2 != nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.left[model] = left
	p.resetAt[model] = p.now().Add(reset)
}
