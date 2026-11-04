package api

import (
	"errors"
	"fmt"
	"testing"

	"github.com/pagepilot/backend/internal/services"
)

func TestPublicMessageHidesUpstreamErrors(t *testing.T) {
	upstream := errors.New("tutor agent failed: OpenAI API error: 401 Unauthorized")
	if got := publicMessage(upstream); got == upstream.Error() {
		t.Fatalf("upstream error leaked to the user: %q", got)
	}

	invalid := fmt.Errorf("%w: only the current question can be answered", services.ErrInvalidInput)
	if got := publicMessage(invalid); got != invalid.Error() {
		t.Fatalf("errors written for users should pass through, got %q", got)
	}
}
