package eval

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// groqStub answers like Groq: after each request it reports the tokens left this minute.
type groqStub struct {
	left   []string
	bodies []string
}

func (g *groqStub) RoundTrip(req *http.Request) (*http.Response, error) {
	b, _ := io.ReadAll(req.Body)
	g.bodies = append(g.bodies, string(b))
	h := http.Header{}
	h.Set("x-ratelimit-remaining-tokens", g.left[len(g.bodies)-1])
	h.Set("x-ratelimit-reset-tokens", "30s")
	return &http.Response{StatusCode: 200, Header: h, Body: io.NopCloser(strings.NewReader("{}"))}, nil
}

func TestPacedTransportWaitsWhenGroqSaysTheMinuteIsUsed(t *testing.T) {
	groq := &groqStub{left: []string{"7000", "50", "8000", "7500"}}
	p := NewPacedTransport()
	p.Base = groq
	send := func(ctx context.Context, model string) error {
		body := `{"model":"` + model + `","messages":["` + strings.Repeat("x", 800) + `"]}`
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.groq.com/x", strings.NewReader(body))
		_, err := p.RoundTrip(req)
		return err
	}

	for i := 0; i < 2; i++ { // the first reply leaves 7,000 tokens, enough for the second request
		if err := send(context.Background(), "agent"); err != nil {
			t.Fatal(err)
		}
	}
	if err := send(context.Background(), "judge"); err != nil {
		t.Fatalf("another model has its own minute and should go straight through: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := send(ctx, "agent"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("with 50 tokens left the agent's next request should wait for the reset, got %v", err)
	}

	p.now = func() time.Time { return time.Now().Add(time.Minute) } // the minute has refilled
	if err := send(context.Background(), "agent"); err != nil || len(groq.bodies) != 4 {
		t.Fatalf("after the reset the request should go: %v (%d sent)", err, len(groq.bodies))
	}
	if !strings.Contains(groq.bodies[0], `"model":"agent"`) {
		t.Fatalf("requests should reach the API unchanged: %s", groq.bodies[0])
	}
}
