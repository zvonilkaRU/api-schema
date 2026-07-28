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
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

// Verifier fetches JWKS from Users service and verifies JWT tokens.
type Verifier struct {
	key    *ecdsa.PublicKey
	issuer string
}

// NewVerifier fetches the public key from the Users JWKS endpoint.
func NewVerifier(ctx context.Context, jwksURL string) (*Verifier, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth: fetch jwks: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
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
	x, _ := base64.RawURLEncoding.DecodeString(k.X)
	y, _ := base64.RawURLEncoding.DecodeString(k.Y)

	return &Verifier{
		key: &ecdsa.PublicKey{
			Curve: elliptic.P256(),
			X:     new(big.Int).SetBytes(x),
			Y:     new(big.Int).SetBytes(y),
		},
		issuer: "users.zvonilka.space",
	}, nil
}

// NewVerifierFromKey creates a Verifier from an in-memory public key (no HTTP fetch).
// Use this in services that already have the key — e.g. the Users service itself.
func NewVerifierFromKey(key *ecdsa.PublicKey, issuer string) *Verifier {
	return &Verifier{key: key, issuer: issuer}
}

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
			return v.key, nil
		},
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience("zvonilka"),
		jwt.WithLeeway(30*time.Second),
	)
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("auth: invalid token")
	}
	return &TokenClaims{Subject: claims.Subject, Nickname: claims.Nickname}, nil
}

// UserID extracts the user ID from a request context (set by Middleware).
func UserID(ctx context.Context) string {
	if v := ctx.Value("user_id"); v != nil {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

// Nickname extracts the user nickname from a request context (set by Middleware).
func Nickname(ctx context.Context) string {
	if v := ctx.Value("nickname"); v != nil {
		if n, ok := v.(string); ok {
			return n
		}
	}
	return ""
}

// RawToken extracts the raw JWT token from a request context (set by Middleware).
func RawToken(ctx context.Context) string {
	if v := ctx.Value("raw_token"); v != nil {
		if t, ok := v.(string); ok {
			return t
		}
	}
	return ""
}

// Middleware returns an Echo middleware that validates JWT tokens,
// skipping the given public paths.
func Middleware(v *Verifier, publicPaths map[string]bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if publicPaths[c.Request().URL.Path] {
				return next(c)
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
			ctx := context.WithValue(c.Request().Context(), "user_id", claims.Subject)
			ctx = context.WithValue(ctx, "nickname", claims.Nickname)
			ctx = context.WithValue(ctx, "raw_token", parts[1])
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	}
}
