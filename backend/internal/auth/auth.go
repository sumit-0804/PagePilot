package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

type ctxKey struct{}

// UserID returns the authenticated Supabase user id set by Middleware, or "".
func UserID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

// WithUserID stores a user id in the context (used by Middleware and tests).
func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ctxKey{}, userID)
}

type claims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// Verifier checks Supabase access tokens.
type Verifier struct {
	keyfunc jwt.Keyfunc
	issuer  string
}

// NewVerifier trusts the project's published signing keys (JWKS) and, if given, the legacy HS256 secret.
func NewVerifier(ctx context.Context, supabaseURL, hs256Secret string) (*Verifier, error) {
	jwksURL := supabaseURL + "/auth/v1/.well-known/jwks.json"
	jwks, err := keyfunc.NewDefaultCtx(ctx, []string{jwksURL})
	if err != nil && hs256Secret == "" {
		return nil, fmt.Errorf("loading signing keys from %s: %w", jwksURL, err)
	}

	kf := func(token *jwt.Token) (any, error) {
		if _, isHMAC := token.Method.(*jwt.SigningMethodHMAC); isHMAC {
			if hs256Secret == "" {
				return nil, errors.New("HS256 token but SUPABASE_JWT_SECRET is not set")
			}
			return []byte(hs256Secret), nil
		}
		if jwks == nil {
			return nil, errors.New("no signing keys available")
		}
		return jwks.Keyfunc(token)
	}
	return NewVerifierWithKeyfunc(kf, supabaseURL+"/auth/v1"), nil
}

func NewVerifierWithKeyfunc(kf jwt.Keyfunc, issuer string) *Verifier {
	return &Verifier{keyfunc: kf, issuer: issuer}
}

// Verify returns the user id (sub) of a valid, signed-in user's access token.
func (v *Verifier) Verify(token string) (string, error) {
	var c claims
	_, err := jwt.ParseWithClaims(token, &c, v.keyfunc,
		jwt.WithValidMethods([]string{"ES256", "RS256", "HS256"}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience("authenticated"),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(30*time.Second),
	)
	if err != nil {
		return "", err
	}
	if c.Role != "authenticated" || c.Subject == "" {
		return "", errors.New("token is not for a signed-in user")
	}
	return c.Subject, nil
}

// Middleware rejects requests without a valid "Authorization: Bearer <token>" header.
func (v *Verifier) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token, found := strings.CutPrefix(header, "Bearer ")
		if !found || strings.TrimSpace(token) == "" {
			unauthorized(w, "missing bearer token")
			return
		}
		userID, err := v.Verify(strings.TrimSpace(token))
		if err != nil {
			unauthorized(w, "invalid or expired token")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), userID)))
	})
}

func unauthorized(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
