// Package auth provides shared JWT authentication middleware.
// Services (rooms, iam) verify tokens from the Users/Auth service via JWKS.
package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

// Verifier fetches JWKS from Users service and verifies JWT tokens.
type Verifier struct {
	mu              sync.RWMutex
	key             *ecdsa.PublicKey
	issuer          string
	jwksURL         string
	refreshInterval time.Duration
}

// contextKey is an unexported type for context keys.
type contextKey string

const (
	ctxUserID   contextKey = "user_id"
	ctxNickname contextKey = "nickname"
	ctxRawToken contextKey = "raw_token"
)

// NewVerifier fetches the public key from the Users JWKS endpoint.
// It retries up to 5 times with exponential backoff, then starts a
// background goroutine that periodically refreshes the key.
func NewVerifier(ctx context.Context, jwksURL string) (*Verifier, error) {
	v := &Verifier{
		issuer:          "users.zvonilka.space",
		jwksURL:         jwksURL,
		refreshInterval: 15 * time.Minute,
	}

	// Fetch JWKS with retry + exponential backoff (up to 5 retries).
	var key *ecdsa.PublicKey
	var lastErr error
	backoff := 1 * time.Second
	for i := 0; i < 5; i++ {
		fetchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		key, lastErr = v.fetchJWKS(fetchCtx)
		cancel()
		if lastErr == nil {
			break
		}
		if i < 4 {
			log.Printf("auth: JWKS fetch attempt %d/5 failed: %v (retrying in %v)", i+1, lastErr, backoff)
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, fmt.Errorf("auth: context cancelled during JWKS fetch: %w", ctx.Err())
			}
			backoff *= 2
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("auth: JWKS fetch failed after 5 retries: %w", lastErr)
	}

	v.key = key

	// Start background refresh goroutine.
	go v.refreshLoop()

	return v, nil
}

// fetchJWKS fetches and parses the JWKS endpoint, returning the first P-256 public key.
func (v *Verifier) fetchJWKS(ctx context.Context) (*ecdsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("auth: create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth: fetch jwks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth: jwks returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("auth: read jwks body: %w", err)
	}
	var jwks struct {
		Keys []struct {
			Crv string `json:"crv"`
			X   string `json:"x"`
			Y   string `json:"y"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(body, &jwks); err != nil {
		return nil, fmt.Errorf("auth: parse jwks: %w", err)
	}
	if len(jwks.Keys) == 0 {
		return nil, fmt.Errorf("auth: no keys in jwks")
	}

	k := jwks.Keys[0]
	if k.Crv != "P-256" {
		return nil, fmt.Errorf("auth: unexpected key curve: %s", k.Crv)
	}
	x, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil {
		return nil, fmt.Errorf("auth: decode key x: %w", err)
	}
	y, err := base64.RawURLEncoding.DecodeString(k.Y)
	if err != nil {
		return nil, fmt.Errorf("auth: decode key y: %w", err)
	}

	return &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(x),
		Y:     new(big.Int).SetBytes(y),
	}, nil
}

// refreshLoop periodically re-fetches JWKS. On failure it keeps the current key
// and logs a warning, avoiding auth outage from a transient refresh failure.
func (v *Verifier) refreshLoop() {
	ticker := time.NewTicker(v.refreshInterval)
	defer ticker.Stop()
	for range ticker.C {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		key, err := v.fetchJWKS(ctx)
		cancel()
		if err != nil {
			log.Printf("auth: JWKS refresh failed (keeping current key): %v", err)
			continue
		}
		v.mu.Lock()
		v.key = key
		v.mu.Unlock()
	}
}

// getKey returns the current public key under read lock.
func (v *Verifier) getKey() *ecdsa.PublicKey {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.key
}

// NewVerifierFromKey creates a Verifier from an in-memory public key (no HTTP fetch).
// Use this in services that already have the key — e.g. the Users service itself.
func NewVerifierFromKey(key *ecdsa.PublicKey, issuer string) *Verifier {
	return &Verifier{key: key, issuer: issuer}
}

// PublicPaths is a set of paths that skip authentication.
type PublicPaths map[string]bool

// TokenClaims holds the verified JWT claims we care about.
type TokenClaims struct {
	Subject  string
	Nickname string
}

// Verify checks a JWT token and returns the claims.
func (v *Verifier) Verify(tokenString string) (*TokenClaims, error) {
	var claims struct {
		jwt.RegisteredClaims
		Nickname string `json:"nickname,omitempty"`
	}
	token, err := jwt.ParseWithClaims(tokenString, &claims,
		func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodECDSA); !ok {
				return nil, fmt.Errorf("auth: unexpected signing method")
			}
			return v.getKey(), nil
		},
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience("zvonilka"),
		jwt.WithLeeway(5*time.Second),
	)
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("auth: invalid token")
	}
	return &TokenClaims{Subject: claims.Subject, Nickname: claims.Nickname}, nil
}

// WithUserID returns a context with the given user ID set.
func WithUserID(ctx context.Context, uid string) context.Context {
	return context.WithValue(ctx, ctxUserID, uid)
}

// WithNickname returns a context with the given nickname set.
func WithNickname(ctx context.Context, nick string) context.Context {
	return context.WithValue(ctx, ctxNickname, nick)
}

// UserID extracts the user ID from a request context (set by Middleware).
func UserID(ctx context.Context) string {
	if v := ctx.Value(ctxUserID); v != nil {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

// Nickname extracts the user nickname from a request context (set by Middleware).
func Nickname(ctx context.Context) string {
	if v := ctx.Value(ctxNickname); v != nil {
		if n, ok := v.(string); ok {
			return n
		}
	}
	return ""
}

// RawToken extracts the raw JWT token from a request context (set by Middleware).
func RawToken(ctx context.Context) string {
	if v := ctx.Value(ctxRawToken); v != nil {
		if t, ok := v.(string); ok {
			return t
		}
	}
	return ""
}

// Middleware returns an Echo middleware that validates JWT tokens,
// skipping the given public paths. Additional skippers (e.g. for
// WebSocket endpoints that pass tokens via query param) can be provided.
func Middleware(v *Verifier, publicPaths PublicPaths, skippers ...func(c echo.Context) bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			path := strings.TrimRight(c.Request().URL.Path, "/")
			if publicPaths[path] {
				return next(c)
			}
			for _, skip := range skippers {
				if skip(c) {
					return next(c)
				}
			}
			auth := c.Request().Header.Get("Authorization")
			if auth == "" {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "missing token"})
			}
			parts := strings.SplitN(auth, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid format"})
			}
			claims, err := v.Verify(parts[1])
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			}
			ctx := context.WithValue(c.Request().Context(), ctxUserID, claims.Subject)
			ctx = context.WithValue(ctx, ctxNickname, claims.Nickname)
			ctx = context.WithValue(ctx, ctxRawToken, parts[1])
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}
