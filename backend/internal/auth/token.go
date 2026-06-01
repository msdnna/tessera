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
func ParseAccessToken(secret, tokenStr string) (uuid.UUID, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return uuid.Nil, err
	}
	return uuid.Parse(claims.Subject)
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
