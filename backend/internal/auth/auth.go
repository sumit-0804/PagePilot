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

// Identity is who made a request. Guests are Supabase anonymous users: real ids, no account yet.
type Identity struct {
	UserID string
	Guest  bool
}

func identity(ctx context.Context) Identity {
	id, _ := ctx.Value(ctxKey{}).(Identity)
	return id
}

// UserID returns the authenticated Supabase user id set by Middleware, or "".
func UserID(ctx context.Context) string {
	return identity(ctx).UserID
}

// IsGuest reports whether the caller signed in anonymously.
func IsGuest(ctx context.Context) bool {
	return identity(ctx).Guest
}

// WithUserID stores a signed-up user's id in the context (used by tests).
func WithUserID(ctx context.Context, userID string) context.Context {
	return WithIdentity(ctx, Identity{UserID: userID})
}

func WithIdentity(ctx context.Context, id Identity) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

type claims struct {
	Role        string `json:"role"`
	IsAnonymous bool   `json:"is_anonymous"`
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

// Verify returns who a valid access token belongs to, signed-up or anonymous.
func (v *Verifier) Verify(token string) (Identity, error) {
	var c claims
	_, err := jwt.ParseWithClaims(token, &c, v.keyfunc,
		jwt.WithValidMethods([]string{"ES256", "RS256", "HS256"}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience("authenticated"),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(30*time.Second),
	)
	if err != nil {
		return Identity{}, err
	}
	if c.Role != "authenticated" || c.Subject == "" {
		return Identity{}, errors.New("token is not for a signed-in user")
	}
	return Identity{UserID: c.Subject, Guest: c.IsAnonymous}, nil
}

// Middleware rejects requests without a valid "Authorization: Bearer <token>" header.
func (v *Verifier) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		token, found := strings.CutPrefix(header, "Bearer ")
		if !found || strings.TrimSpace(token) == "" {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		id, err := v.Verify(strings.TrimSpace(token))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		next.ServeHTTP(w, r.WithContext(WithIdentity(r.Context(), id)))
	})
}

// RequireAccount blocks guests from features that need a signed-up account.
func RequireAccount(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if IsGuest(r.Context()) {
			writeError(w, http.StatusForbidden, "login_required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
