// Package ocr reads the text of scanned PDF pages that have no text layer.
package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/pagepilot/backend/internal/config"
	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// ErrDailyQuota means the OCR model's requests-per-day limit is used up until midnight Pacific time.
var ErrDailyQuota = errors.New("daily OCR quota reached")

// Recognizer reads scanned pages. Pages are 1-based and the texts come back in the same order.
type Recognizer interface {
	Recognize(ctx context.Context, pdfData []byte, pages []int) ([]string, error)
	Model() string
}

// New builds the configured recognizer, or returns nil when OCR is turned off.
func New(ctx context.Context, cfg *config.Config) (Recognizer, error) {
	if cfg.OCRProvider != "gemini" {
		return nil, nil
	}
	r, err := NewGeminiOCR(ctx, GeminiConfig{
		APIKey:       cfg.GeminiAPIKey,
		Model:        cfg.OCRModel,
		RPM:          cfg.OCRRPM,
		TPM:          cfg.OCRTPM,
		RPD:          cfg.OCRRPD,
		PagesPerCall: cfg.OCRPagesPerCall,
	})
	if err != nil {
		return nil, err
	}
	// Looking the model up checks the key and model name without using any of the OCR quota.
	if err := r.check(ctx); err != nil {
		return nil, fmt.Errorf("OCR model check failed: %w", err)
	}
	return r, nil
}

// selectPages copies the given pages into a new, smaller PDF, so each request carries only scanned pages.
func selectPages(data []byte, pages []int) ([]byte, error) {
	sel := make([]string, len(pages))
	for i, p := range pages {
		sel[i] = strconv.Itoa(p)
	}
	var out bytes.Buffer
	// The stateless configuration keeps pdfcpu from writing a config folder to disk.
	if err := api.Trim(context.Background(), bytes.NewReader(data), &out, sel, model.NewStatelessConfiguration()); err != nil {
		return nil, fmt.Errorf("selecting pages %v: %w", pages, err)
	}
	return out.Bytes(), nil
}
