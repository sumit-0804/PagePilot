package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pagepilot/backend/internal/services"
)

func TestHealthEndpoints(t *testing.T) {
	down := false
	a := &API{healthChecks: map[string]func(context.Context) error{
		"database": func(context.Context) error {
			if down {
				return errors.New("dial tcp db.internal.example:5432: connection refused")
			}
			return nil
		},
	}}
	router := SetupRouter(a, nil, nil)
	get := func(path string) *httptest.ResponseRecorder {
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		return rr
	}

	for _, path := range []string{"/health/live", "/health/ready", "/health", "/api/health"} {
		if rr := get(path); rr.Code != http.StatusOK {
			t.Errorf("%s with everything up: %d", path, rr.Code)
		}
	}

	down = true
	if rr := get("/health/live"); rr.Code != http.StatusOK {
		t.Errorf("liveness must not depend on the database: %d", rr.Code)
	}
	rr := get("/health/ready")
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("readiness with the database down: %d", rr.Code)
	}
	if body := rr.Body.String(); strings.Contains(body, "db.internal") || !strings.Contains(body, `"database":"error"`) {
		t.Errorf("readiness should name the failed check without its details: %s", body)
	}
}

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
