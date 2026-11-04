package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testIssuer = "http://127.0.0.1:54321/auth/v1"
	testSecret = "test-secret-at-least-32-characters-long"
)

func signToken(t *testing.T, mutate func(*claims)) string {
	t.Helper()
	c := claims{
		Role: "authenticated",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "11111111-1111-1111-1111-111111111111",
			Issuer:    testIssuer,
			Audience:  jwt.ClaimStrings{"authenticated"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	if mutate != nil {
		mutate(&c)
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func testVerifier() *Verifier {
	return NewVerifierWithKeyfunc(func(*jwt.Token) (any, error) { return []byte(testSecret), nil }, testIssuer)
}

func TestMiddleware(t *testing.T) {
	var gotUser string
	handler := testVerifier().Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = UserID(r.Context())
	}))

	cases := []struct {
		name   string
		header string
		status int
	}{
		{"valid token", "Bearer " + signToken(t, nil), http.StatusOK},
		{"missing header", "", http.StatusUnauthorized},
		{"not bearer", "Basic abc", http.StatusUnauthorized},
		{"garbage token", "Bearer not-a-jwt", http.StatusUnauthorized},
		{"expired", "Bearer " + signToken(t, func(c *claims) { c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour)) }), http.StatusUnauthorized},
		{"wrong issuer", "Bearer " + signToken(t, func(c *claims) { c.Issuer = "https://evil.example/auth/v1" }), http.StatusUnauthorized},
		{"anon role", "Bearer " + signToken(t, func(c *claims) { c.Role = "anon" }), http.StatusUnauthorized},
		{"wrong audience", "Bearer " + signToken(t, func(c *claims) { c.Audience = jwt.ClaimStrings{"other"} }), http.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotUser = ""
			req := httptest.NewRequest(http.MethodGet, "/api/documents", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("expected %d, got %d", tc.status, rec.Code)
			}
			if tc.status == http.StatusOK && gotUser != "11111111-1111-1111-1111-111111111111" {
				t.Fatalf("user id not set, got %q", gotUser)
			}
		})
	}
}

func TestGuestsAreIdentifiedAndKeptFromAccountRoutes(t *testing.T) {
	v := testVerifier()
	guest := signToken(t, func(c *claims) { c.IsAnonymous = true })
	id, err := v.Verify(guest)
	if err != nil || !id.Guest {
		t.Fatalf("anonymous token: %+v, %v", id, err)
	}

	reached := false
	handler := v.Middleware(RequireAccount(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })))
	for _, tc := range []struct {
		token  string
		status int
	}{
		{guest, http.StatusForbidden},
		{signToken(t, nil), http.StatusOK},
	} {
		reached = false
		req := httptest.NewRequest(http.MethodPost, "/api/documents/upload", nil)
		req.Header.Set("Authorization", "Bearer "+tc.token)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.status || reached != (tc.status == http.StatusOK) {
			t.Fatalf("expected %d, got %d (reached=%v)", tc.status, rec.Code, reached)
		}
	}
}
