package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"google.golang.org/genai"
)

func readFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/scanned.pdf")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func pageCount(t *testing.T, data []byte) int {
	t.Helper()
	n, err := api.PageCount(context.Background(), bytes.NewReader(data), model.NewStatelessConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func newTestOCR(pagesPerCall int, call readCall) *GeminiOCR {
	g := newGeminiOCR(GeminiConfig{Model: "test", RPM: 1000, TPM: 1_000_000, RPD: 1000, PagesPerCall: pagesPerCall}, call)
	g.sleep = func(context.Context, time.Duration) error { return nil }
	return g
}

// echoCall answers "call N page K" for each page, and checks the request holds only the pages asked for.
func echoCall(t *testing.T, calls *[]int) readCall {
	return func(_ context.Context, data []byte, n int) ([]string, bool, error) {
		if got := pageCount(t, data); got != n {
			t.Errorf("request carries %d pages, expected %d", got, n)
		}
		*calls = append(*calls, n)
		texts := make([]string, n)
		for i := range texts {
			texts[i] = fmt.Sprintf("call %d page %d", len(*calls), i+1)
		}
		return texts, false, nil
	}
}

func TestPagesAreSentInBatchesAndKeepTheirOrder(t *testing.T) {
	data := readFixture(t)
	if pageCount(t, data) < 5 {
		t.Fatal("fixture needs at least 5 pages")
	}
	var calls []int
	g := newTestOCR(2, echoCall(t, &calls))

	texts, err := g.Recognize(context.Background(), data, []int{1, 2, 3, 4, 5})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"call 1 page 1", "call 1 page 2", "call 2 page 1", "call 2 page 2", "call 3 page 1"}
	if strings.Join(texts, ",") != strings.Join(want, ",") || len(calls) != 3 {
		t.Fatalf("got %v from calls %v", texts, calls)
	}
	if _, err := g.Recognize(context.Background(), data, []int{3, 1}); err == nil {
		t.Fatal("pages out of order must be rejected, since texts would land on the wrong pages")
	}
}

func TestCutOffAnswerIsRetriedInSmallerBatches(t *testing.T) {
	var calls []int
	echo := echoCall(t, &calls)
	g := newTestOCR(4, func(ctx context.Context, data []byte, n int) ([]string, bool, error) {
		if n > 2 {
			calls = append(calls, n)
			return nil, true, nil
		}
		return echo(ctx, data, n)
	})
	texts, err := g.Recognize(context.Background(), readFixture(t), []int{1, 2, 3, 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(texts) != 4 || fmt.Sprint(calls) != "[4 2 2]" {
		t.Fatalf("expected one cut-off call then two halves, got calls %v texts %v", calls, texts)
	}
}

func TestDailyQuotaStopsOCR(t *testing.T) {
	g := newTestOCR(10, func(context.Context, []byte, int) ([]string, bool, error) {
		return nil, false, genai.APIError{Code: 429, Message: "Quota exceeded for metric GenerateRequestsPerDayPerProjectPerModel"}
	})
	if _, err := g.Recognize(context.Background(), readFixture(t), []int{1}); !errors.Is(err, ErrDailyQuota) {
		t.Fatalf("expected ErrDailyQuota, got %v", err)
	}
	if _, err := g.Recognize(context.Background(), readFixture(t), []int{1}); !errors.Is(err, ErrDailyQuota) {
		t.Fatalf("expected the local quota to stay used up, got %v", err)
	}
}

func TestOverloadedServerIsRetried(t *testing.T) {
	attempts := 0
	g := newTestOCR(10, func(_ context.Context, _ []byte, n int) ([]string, bool, error) {
		attempts++
		if attempts == 1 {
			return nil, false, genai.APIError{Code: 503, Message: "The model is overloaded"}
		}
		return make([]string, n), false, nil
	})
	if _, err := g.Recognize(context.Background(), readFixture(t), []int{1}); err != nil || attempts != 2 {
		t.Fatalf("expected success on the second attempt, got attempts=%d err=%v", attempts, err)
	}
}

func TestParsePagesRejectsMismatchedPages(t *testing.T) {
	cases := map[string]string{
		"too few":  `{"pages":[{"page":1,"text":"a"}]}`,
		"repeated": `{"pages":[{"page":1,"text":"a"},{"page":1,"text":"b"}]}`,
		"unknown":  `{"pages":[{"page":1,"text":"a"},{"page":3,"text":"b"}]}`,
		"not json": `pages: a, b`,
	}
	for name, raw := range cases {
		if _, err := parsePages(raw, 2); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
	texts, err := parsePages(`{"pages":[{"page":2,"text":"b"},{"page":1,"text":"a"}]}`, 2)
	if err != nil || texts[0] != "a" || texts[1] != "b" {
		t.Fatalf("pages should be placed by number, got %v, %v", texts, err)
	}
}
