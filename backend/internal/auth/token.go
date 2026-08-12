// Package auth issues and validates JWTs and opaque refresh tokens.
//
// Access tokens are short-lived (15 min) HS256 JWTs carrying the user id.
// Refresh tokens are random opaque strings; only their SHA-256 hash is stored,
// and they rotate on every use (each /auth/refresh revokes the old token and
// issues a new one).
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Token lifetimes: a short-lived access token and a long-lived rotating refresh token.
const (
	AccessTokenTTL  = 15 * time.Minute
	RefreshTokenTTL = 30 * 24 * time.Hour
)

// Claims is the access-token payload.
type Claims struct {
	jwt.RegisteredClaims
}

// NewAccessToken signs a short-lived JWT for the given user.
func NewAccessToken(secret string, userID uuid.UUID) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// ParseAccessToken validates a JWT and returns the user id from its subject.
// An audience is a hard reject: media tokens (below) are signed with the same
// key but live for 30 days, so accepting one here would turn a cookie meant for
// fetching images into a full API session.
func ParseAccessToken(secret, tokenStr string) (uuid.UUID, error) {
	claims, err := parseSigned(secret, tokenStr)
	if err != nil {
		return uuid.Nil, err
	}
	if len(claims.Audience) != 0 {
		return uuid.Nil, fmt.Errorf("token is not an access token")
	}
	return uuid.Parse(claims.Subject)
}

// Media tokens back the httpOnly cookie that authorises inline-image requests
// (/api/uploads/…). An <img> can send neither a bearer header nor a body, so the
// cookie is the only credential the browser will attach — see
// handlers/auth_cookie.go. The lifetime matches the refresh token: the cookie is
// meant to last exactly as long as the session that minted it.
const (
	MediaTokenTTL = RefreshTokenTTL
	// mediaAudience separates media tokens from access tokens on the same key.
	mediaAudience = "media"
)

// NewMediaToken signs the long-lived, image-only credential for a user.
func NewMediaToken(secret string, userID uuid.UUID) (string, error) {
	now := time.Now()
	claims := Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{mediaAudience},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(MediaTokenTTL)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
}

// ParseMediaToken validates a media token and returns its owner. The audience
// must be present, so an access token can't be replayed as one (and, with the
// check in ParseAccessToken, not the other way round either).
func ParseMediaToken(secret, tokenStr string) (uuid.UUID, error) {
	claims, err := parseSigned(secret, tokenStr)
	if err != nil {
		return uuid.Nil, err
	}
	if !hasAudience(claims.Audience, mediaAudience) {
		return uuid.Nil, fmt.Errorf("token is not a media token")
	}
	return uuid.Parse(claims.Subject)
}

// hasAudience reports whether the claim carries the given audience.
func hasAudience(aud jwt.ClaimStrings, want string) bool {
	for _, a := range aud {
		if a == want {
			return true
		}
	}
	return false
}

// parseSigned verifies signature, algorithm and expiry, leaving the caller to
// decide which kind of token it was willing to accept.
func parseSigned(secret, tokenStr string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// NewRefreshToken returns a random opaque token and its storage hash.
func NewRefreshToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	token = hex.EncodeToString(b)
	return token, HashRefreshToken(token), nil
}

// HashRefreshToken returns the SHA-256 hex digest stored for a refresh token.
func HashRefreshToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// PATPrefix marks personal access tokens so the auth middleware can tell them
// apart from JWTs (which contain dots) and opaque refresh tokens.
const PATPrefix = "tsra_"

// NewPAT returns a random personal access token (prefixed, revocable, long-lived)
// and its storage hash. Only the hash is persisted; the plaintext is shown once.
func NewPAT() (token, hash string, err error) {
	b := make([]byte, 24)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	token = PATPrefix + hex.EncodeToString(b)
	return token, HashToken(token), nil
}

// HashToken returns the SHA-256 hex digest stored for a personal access token.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
