// Package middleware holds gin middleware (JWT auth, request context helpers).
package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/auth"
	"tessera/internal/db"
)

// ContextUserID is the gin context key holding the authenticated user's uuid.
const ContextUserID = "user_id"

// Auth validates the Bearer credential and stores the user id in the context.
// It accepts two credential kinds transparently on every protected route:
//   - a short-lived access JWT (browser/app sessions), or
//   - a Personal Access Token (tsra_… prefix) for headless clients (MCP, CI).
func Auth(secret string, q *db.Queries) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		tok := strings.TrimPrefix(h, "Bearer ")

		if strings.HasPrefix(tok, auth.PATPrefix) {
			uid, ok := authenticatePAT(c, q, tok)
			if !ok {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
				return
			}
			c.Set(ContextUserID, uid)
			c.Next()
			return
		}

		uid, err := auth.ParseAccessToken(secret, tok)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		c.Set(ContextUserID, uid)
		c.Next()
	}
}

// authenticatePAT resolves a personal access token to its owner, rejecting
// revoked or expired tokens. On success it fires a best-effort last-used touch.
func authenticatePAT(c *gin.Context, q *db.Queries, tok string) (uuid.UUID, bool) {
	if q == nil {
		return uuid.Nil, false
	}
	pat, err := q.GetPATByHash(c, auth.HashToken(tok))
	if err != nil {
		return uuid.Nil, false
	}
	if pat.RevokedAt != nil || (pat.ExpiresAt != nil && time.Now().After(*pat.ExpiresAt)) {
		return uuid.Nil, false
	}
	// Best-effort, non-blocking: record usage on a detached context so it
	// survives past the request lifecycle without delaying the response.
	go func(id uuid.UUID) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = q.TouchPATLastUsed(ctx, id)
	}(pat.ID)
	return pat.UserID, true
}

// CurrentUser returns the authenticated user id set by Auth (uuid.Nil if absent).
func CurrentUser(c *gin.Context) uuid.UUID {
	v, _ := c.Get(ContextUserID)
	id, _ := v.(uuid.UUID)
	return id
}
