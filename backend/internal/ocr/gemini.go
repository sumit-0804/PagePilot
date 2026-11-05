package ocr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/pagepilot/backend/internal/gemini"
	"google.golang.org/genai"
)

const (
	// Google bills a PDF page as roughly 258 input tokens; scans can cost more, so this stays on the safe side.
	tokensPerPage = 400
	// Requests above ~20 MB must use the Files API, so larger batches are split instead.
	maxInlineBytes = 18 << 20
)

type GeminiConfig struct {
	// APIKeys are tried in order: a key out of quota, failing or rejected hands the request to the next.
	// Each needs its own Google project, since quota is per project.
	APIKeys      []string
	Model        string
	RPM          int
	TPM          int
	RPD          int
	PagesPerCall int
}

// readCall sends one small PDF and returns one text per page; truncated means the answer hit the output limit.
type readCall func(ctx context.Context, pdfData []byte, pageCount int) (texts []string, truncated bool, err error)

// apiKey is one Gemini key with its own client and rate limits.
type apiKey struct {
	call    readCall
	check   func(ctx context.Context) error
	limiter *gemini.Limiter
}

type GeminiOCR struct {
	model        string
	keys         []*apiKey
	sleep        func(ctx context.Context, d time.Duration) error
	pagesPerCall int
	maxRetries   int
}

func NewGeminiOCR(ctx context.Context, cfg GeminiConfig) (*GeminiOCR, error) {
	if len(cfg.APIKeys) == 0 {
		return nil, errors.New("no Gemini API key for OCR")
	}
	calls := make([]readCall, len(cfg.APIKeys))
	checks := make([]func(context.Context) error, len(cfg.APIKeys))
	for i, key := range cfg.APIKeys {
		client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: key, Backend: genai.BackendGeminiAPI})
		if err != nil {
			return nil, fmt.Errorf("creating gemini client %d: %w", i+1, err)
		}
		calls[i] = readWith(client, cfg.Model)
		checks[i] = func(ctx context.Context) error {
			_, err := client.Models.Get(ctx, cfg.Model, nil)
			return err
		}
	}
	g := newGeminiOCR(cfg, calls...)
	for i, k := range g.keys {
		k.check = checks[i]
	}
	return g, nil
}

// readWith sends one PDF to the model through one key's client.
func readWith(client *genai.Client, modelName string) readCall {
	return func(ctx context.Context, pdfData []byte, pageCount int) ([]string, bool, error) {
		content := genai.NewContentFromParts([]*genai.Part{
			genai.NewPartFromBytes(pdfData, "application/pdf"),
			genai.NewPartFromText(transcribePrompt(pageCount)),
		}, genai.RoleUser)
		resp, err := client.Models.GenerateContent(ctx, modelName, []*genai.Content{content}, &genai.GenerateContentConfig{
			Temperature:      genai.Ptr[float32](0),
			ResponseMIMEType: "application/json",
			ResponseSchema:   pagesSchema,
		})
		if err != nil {
			return nil, false, err
		}
		if len(resp.Candidates) > 0 && resp.Candidates[0].FinishReason == genai.FinishReasonMaxTokens {
			return nil, true, nil
		}
		texts, err := parsePages(resp.Text(), pageCount)
		return texts, false, err
	}
}

// newGeminiOCR gives each key's call its own limiter; the configured limits apply per key.
func newGeminiOCR(cfg GeminiConfig, calls ...readCall) *GeminiOCR {
	pagesPerCall := cfg.PagesPerCall
	if pagesPerCall <= 0 {
		pagesPerCall = 10
	}
	g := &GeminiOCR{
		model:        cfg.Model,
		sleep:        gemini.SleepCtx,
		pagesPerCall: pagesPerCall,
		maxRetries:   5,
	}
	for _, call := range calls {
		g.keys = append(g.keys, &apiKey{call: call, limiter: gemini.NewLimiter(cfg.RPM, cfg.TPM, cfg.RPD)})
	}
	return g
}

// check looks every key up against the model, which uses none of the OCR quota.
func (g *GeminiOCR) check(ctx context.Context) error {
	for i, k := range g.keys {
		if err := k.check(ctx); err != nil {
			return fmt.Errorf("key %d: %w", i+1, err)
		}
	}
	return nil
}

func (g *GeminiOCR) Model() string { return g.model }

func (g *GeminiOCR) Recognize(ctx context.Context, pdfData []byte, pages []int) ([]string, error) {
	// The page selector always keeps the PDF's own page order, so texts only line up for ascending pages.
	for i := 1; i < len(pages); i++ {
		if pages[i] <= pages[i-1] {
			return nil, fmt.Errorf("pages must be in ascending order without repeats, got %v", pages)
		}
	}
	out := make([]string, 0, len(pages))
	for start := 0; start < len(pages); start += g.pagesPerCall {
		batch := pages[start:min(start+g.pagesPerCall, len(pages))]
		started := time.Now()
		texts, err := g.readBatch(ctx, pdfData, batch)
		if err != nil {
			return nil, err
		}
		log.Printf("[ocr] read pages %v in %s", batch, time.Since(started).Round(time.Millisecond))
		out = append(out, texts...)
	}
	return out, nil
}

// readBatch reads a group of pages, halving the group when the request or the answer would be too large.
func (g *GeminiOCR) readBatch(ctx context.Context, pdfData []byte, pages []int) ([]string, error) {
	subset, err := selectPages(pdfData, pages)
	if err != nil {
		return nil, err
	}
	if len(subset) > maxInlineBytes {
		if len(pages) == 1 {
			return nil, fmt.Errorf("page %d is %d MB, too large to send for OCR", pages[0], len(subset)>>20)
		}
		return g.readHalves(ctx, pdfData, pages)
	}
	texts, truncated, err := g.callWithRetry(ctx, subset, len(pages))
	if err != nil {
		return nil, err
	}
	if truncated {
		if len(pages) == 1 {
			return nil, fmt.Errorf("the text of page %d is longer than the OCR model can return", pages[0])
		}
		return g.readHalves(ctx, pdfData, pages)
	}
	return texts, nil
}

func (g *GeminiOCR) readHalves(ctx context.Context, pdfData []byte, pages []int) ([]string, error) {
	mid := len(pages) / 2
	first, err := g.readBatch(ctx, pdfData, pages[:mid])
	if err != nil {
		return nil, err
	}
	second, err := g.readBatch(ctx, pdfData, pages[mid:])
	if err != nil {
		return nil, err
	}
	return append(first, second...), nil
}

// callWithRetry reads with the first key, moving to the next when a key is out of quota, keeps failing
// or is rejected. Errors that any key would hit (a bad request, a cancelled upload) are returned as is.
func (g *GeminiOCR) callWithRetry(ctx context.Context, subset []byte, pageCount int) ([]string, bool, error) {
	for i, k := range g.keys {
		texts, truncated, err := g.callKey(ctx, k, subset, pageCount)
		if err == nil || i == len(g.keys)-1 || !otherKeyMayWork(err) {
			return texts, truncated, err
		}
		log.Printf("[ocr] Gemini key %d failed (%v); trying key %d", i+1, err, i+2)
	}
	return nil, false, errors.New("no Gemini API key for OCR")
}

func otherKeyMayWork(err error) bool {
	if errors.Is(err, ErrDailyQuota) {
		return true
	}
	apiErr, ok := gemini.AsAPIError(err)
	return ok && (apiErr.Code == 401 || apiErr.Code == 403 || apiErr.Code == 429 || apiErr.Code >= 500)
}

// callKey makes one rate-limited request with one key, retrying on per-minute limits and temporary server errors.
func (g *GeminiOCR) callKey(ctx context.Context, k *apiKey, subset []byte, pageCount int) ([]string, bool, error) {
	for attempt := 0; ; attempt++ {
		if _, err := k.limiter.Admit(ctx, []int{tokensPerPage * pageCount}); err != nil {
			if errors.Is(err, gemini.ErrDailyQuota) {
				return nil, false, ErrDailyQuota
			}
			return nil, false, err
		}
		texts, truncated, err := k.call(ctx, subset, pageCount)
		if err == nil {
			return texts, truncated, nil
		}

		apiErr, ok := gemini.AsAPIError(err)
		if !ok || (apiErr.Code != 429 && apiErr.Code != 500 && apiErr.Code != 503) {
			return nil, false, fmt.Errorf("gemini OCR request failed: %w", err)
		}
		if apiErr.Code == 429 && gemini.IsDailyQuotaError(apiErr) {
			k.limiter.ExhaustDay()
			return nil, false, ErrDailyQuota
		}
		if attempt >= g.maxRetries {
			return nil, false, fmt.Errorf("gemini OCR still failing after %d retries: %w", g.maxRetries, err)
		}
		delay := gemini.RetryDelay(apiErr, attempt)
		log.Printf("[ocr] Gemini returned %d for %d pages; retrying in %s: %s", apiErr.Code, pageCount, delay, apiErr.Message)
		if err := g.sleep(ctx, delay); err != nil {
			return nil, false, err
		}
	}
}

func transcribePrompt(pageCount int) string {
	return fmt.Sprintf(`This PDF has %d pages. Transcribe the text of every page exactly as it is written, in reading order.
- Do not summarize, translate, correct or add anything.
- Keep headings, list items and paragraphs on their own lines.
- Write table rows on one line each, with cells separated by " | ".
- For a page with no readable text, use an empty string.
Return one entry per page, with page numbers 1 to %d in order.`, pageCount, pageCount)
}

var pagesSchema = &genai.Schema{
	Type: genai.TypeObject,
	Properties: map[string]*genai.Schema{
		"pages": {
			Type: genai.TypeArray,
			Items: &genai.Schema{
				Type: genai.TypeObject,
				Properties: map[string]*genai.Schema{
					"page": {Type: genai.TypeInteger},
					"text": {Type: genai.TypeString},
				},
				Required: []string{"page", "text"},
			},
		},
	},
	Required: []string{"pages"},
}

// parsePages checks the model returned each page exactly once, since a wrong page number would break citations.
func parsePages(raw string, pageCount int) ([]string, error) {
	var resp struct {
		Pages []struct {
			Page int    `json:"page"`
			Text string `json:"text"`
		} `json:"pages"`
	}
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, fmt.Errorf("OCR response is not valid JSON: %w", err)
	}
	if len(resp.Pages) != pageCount {
		return nil, fmt.Errorf("OCR returned %d pages, expected %d", len(resp.Pages), pageCount)
	}
	texts := make([]string, pageCount)
	seen := make([]bool, pageCount)
	for _, p := range resp.Pages {
		if p.Page < 1 || p.Page > pageCount || seen[p.Page-1] {
			return nil, fmt.Errorf("OCR returned an unexpected or repeated page number %d", p.Page)
		}
		seen[p.Page-1] = true
		texts[p.Page-1] = p.Text
	}
	return texts, nil
}
