package gemini

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"
)

func AsAPIError(err error) (genai.APIError, bool) {
	var v genai.APIError
	if errors.As(err, &v) {
		return v, true
	}
	var p *genai.APIError
	if errors.As(err, &p) && p != nil {
		return *p, true
	}
	return genai.APIError{}, false
}

// IsDailyQuotaError checks Google's quota id (e.g. "...RequestsPerDayPerProject...") in the error.
func IsDailyQuotaError(e genai.APIError) bool {
	return strings.Contains(e.Message, "PerDay") || strings.Contains(fmt.Sprint(e.Details), "PerDay")
}

// RetryDelay honors the server's RetryInfo when present, otherwise backs off exponentially.
func RetryDelay(e genai.APIError, attempt int) time.Duration {
	for _, d := range e.Details {
		if raw, ok := d["retryDelay"].(string); ok {
			if dur, err := time.ParseDuration(raw); err == nil && dur > 0 {
				return dur
			}
		}
	}
	delay := 2 * time.Second << attempt
	if delay > time.Minute {
		delay = time.Minute
	}
	return delay
}
