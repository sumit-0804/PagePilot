package services

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/pagepilot/backend/internal/embeddings"
)

func TestOnlyFaultsAreReportable(t *testing.T) {
	for _, c := range []struct {
		name string
		err  error
		want bool
	}{
		{"unreadable file", failWith("This PDF couldn't be read.", errors.New("xref out of range")), false},
		{"no text", failWith("No text was found in this file.", errors.New("no extractable text")), false},
		{"embedding provider down", failInfra("Indexing this file failed.", errors.New("gemini returned 503")), true},
		{"daily quota", failInfra("Indexing this file failed.", fmt.Errorf("batch 2: %w", embeddings.ErrDailyQuota)), false},
		{"database error", errors.New("saving chunks: connection reset"), true},
		{"cancelled", fmt.Errorf("reading stored file: %w", context.Canceled), false},
	} {
		if got := reportable(c.err); got != c.want {
			t.Errorf("%s: reportable = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIsUserError(t *testing.T) {
	if !IsUserError(fmt.Errorf("conversation: %w", ErrNotFound)) || !IsUserError(ErrLoginRequired) {
		t.Error("wrapped sentinel errors are user errors")
	}
	if IsUserError(errors.New("groq returned status 500")) {
		t.Error("a provider failure is not a user error")
	}
}
