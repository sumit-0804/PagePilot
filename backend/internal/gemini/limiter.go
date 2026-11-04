// Package gemini holds what every Gemini API caller shares: per-project rate limits and 429 handling.
package gemini

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
	_ "time/tzdata" // Windows has no system zoneinfo; needed for the Pacific-time daily reset
)

// ErrDailyQuota means a requests-per-day limit is used up until the reset at midnight Pacific time.
var ErrDailyQuota = errors.New("daily Gemini quota reached")

// Limiter enforces Gemini's requests/minute, tokens/minute and requests/day for one model. Admit takes
// one entry per request; the embedder passes one per text because Google counts every text in a batch.
// The daily count is in memory, so a restart resets it; a daily-quota 429 from Google still stops work.
type Limiter struct {
	rpm *window
	tpm *window
	rpd int

	mu      sync.Mutex
	dayKey  string
	dayUsed int
	now     func() time.Time
	sleep   func(ctx context.Context, d time.Duration) error
	pacific *time.Location
}

func NewLimiter(rpm, tpm, rpd int) *Limiter {
	loc, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		loc = time.FixedZone("PST", -8*60*60)
	}
	return &Limiter{
		rpm:     &window{limit: rpm},
		tpm:     &window{limit: tpm},
		rpd:     rpd,
		now:     time.Now,
		sleep:   SleepCtx,
		pacific: loc,
	}
}

// SetClock replaces the clock and sleep function, so tests can move time without waiting.
func (l *Limiter) SetClock(now func() time.Time, sleep func(ctx context.Context, d time.Duration) error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.now, l.sleep = now, sleep
}

// Admit waits until at least the first request fits the current minute, then reserves as many of the
// leading requests as fit now (by count, tokens and what's left of the day) and returns how many.
func (l *Limiter) Admit(ctx context.Context, tokens []int) (int, error) {
	if tokens[0] > l.tpm.limit {
		return 0, fmt.Errorf("request of ~%d tokens exceeds the per-minute token limit (%d)", tokens[0], l.tpm.limit)
	}
	for {
		n, delay, err := l.tryAdmit(tokens)
		if err != nil || n > 0 {
			return n, err
		}
		if err := l.sleep(ctx, delay); err != nil {
			return 0, err
		}
	}
}

func (l *Limiter) tryAdmit(tokens []int) (int, time.Duration, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rollDay()
	if l.dayUsed >= l.rpd {
		return 0, 0, ErrDailyQuota
	}
	now := l.now()
	roomRequests, roomTokens := l.rpm.room(now), l.tpm.room(now)
	n, sum := 0, 0
	for n < len(tokens) && n < roomRequests && n < l.rpd-l.dayUsed && sum+tokens[n] <= roomTokens {
		sum += tokens[n]
		n++
	}
	if n == 0 {
		return 0, max(l.rpm.waitFor(now, 1), l.tpm.waitFor(now, tokens[0])), nil
	}
	l.rpm.add(now, n)
	l.tpm.add(now, sum)
	l.dayUsed += n
	return n, 0, nil
}

// ExhaustDay marks today's quota as used, after Google reports it is gone.
func (l *Limiter) ExhaustDay() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rollDay()
	l.dayUsed = l.rpd
}

func (l *Limiter) rollDay() {
	key := l.now().In(l.pacific).Format("2006-01-02")
	if key != l.dayKey {
		l.dayKey = key
		l.dayUsed = 0
	}
}

// window is a one-minute sliding window: unlike a token bucket it never admits more than limit
// in any 60 seconds, which is how Google applies per-minute quotas.
type window struct {
	limit  int
	events []windowEvent
}

type windowEvent struct {
	at time.Time
	n  int
}

// room drops entries older than a minute and returns how much is left in the window.
func (w *window) room(now time.Time) int {
	cutoff := now.Add(-time.Minute)
	kept := w.events[:0]
	used := 0
	for _, e := range w.events {
		if e.at.After(cutoff) {
			kept = append(kept, e)
			used += e.n
		}
	}
	w.events = kept
	return w.limit - used
}

// waitFor returns 0 if n more fit right now, otherwise how long until the oldest entries expire.
func (w *window) waitFor(now time.Time, n int) time.Duration {
	used := w.limit - w.room(now)
	if used+n <= w.limit {
		return 0
	}
	free := w.limit - used
	for _, e := range w.events {
		free += e.n
		if free >= n {
			return e.at.Add(time.Minute).Sub(now) + 10*time.Millisecond
		}
	}
	return time.Minute
}

func (w *window) add(now time.Time, n int) {
	w.events = append(w.events, windowEvent{at: now, n: n})
}

func SleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
