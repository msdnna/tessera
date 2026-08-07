// Package middleware holds gin middleware (JWT auth, request context helpers).
package middleware

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/auth"
	"tessera/internal/db"
)

// ContextUserID is the gin context key holding the authenticated user's uuid.
const ContextUserID = "user_id"

// patToucher throttles last_used_at writes. Without it every PAT request spawns
// a goroutine that takes a pool connection for a single UPDATE — a batching
// client (MCP, CI) drains the pool with writes nobody reads at second precision.
type patToucher struct {
	mu       sync.Mutex
	last     map[uuid.UUID]time.Time
	interval time.Duration
	now      func() time.Time // injectable for tests
}

func newPATToucher(interval time.Duration) *patToucher {
	return &patToucher{last: make(map[uuid.UUID]time.Time), interval: interval, now: time.Now}
}

// shouldTouch reports whether this token's last_used_at is stale enough to be
// written, recording the write when it says yes.
func (p *patToucher) shouldTouch(id uuid.UUID) bool {
	if p == nil || p.interval <= 0 {
		return true // throttle disabled — preserve the write-every-request behaviour
	}
	now := p.now()
	p.mu.Lock()
	defer p.mu.Unlock()
	if seen, ok := p.last[id]; ok && now.Sub(seen) < p.interval {
		return false
	}
	// Evict tokens untouched for well over a window, so the map can't grow into
	// a leak of its own on an install that mints many short-lived tokens.
	if len(p.last) > patTouchEvictAt {
		for k, seen := range p.last {
			if now.Sub(seen) > 2*p.interval {
				delete(p.last, k)
			}
		}
	}
	p.last[id] = now
	return true
}

// patTouchEvictAt is the map size past which shouldTouch sweeps stale entries.
const patTouchEvictAt = 1024

// Auth validates the Bearer credential and stores the user id in the context.
// It accepts two credential kinds transparently on every protected route:
//   - a short-lived access JWT (browser/app sessions), or
//   - a Personal Access Token (tsra_… prefix) for headless clients (MCP, CI).
//
// patTouchInterval bounds how often a PAT's last_used_at is rewritten; 0 writes
// on every request.
func Auth(secret string, q *db.Queries, patTouchInterval time.Duration) gin.HandlerFunc {
	toucher := newPATToucher(patTouchInterval)
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing bearer token"})
			return
		}
		tok := strings.TrimPrefix(h, "Bearer ")

		if strings.HasPrefix(tok, auth.PATPrefix) {
			uid, ok := authenticatePAT(c, q, tok, toucher)
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
// revoked or expired tokens. On success it fires a best-effort last-used touch,
// throttled so a busy client doesn't spend a pool connection per request.
func authenticatePAT(c *gin.Context, q *db.Queries, tok string, toucher *patToucher) (uuid.UUID, bool) {
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
	if toucher.shouldTouch(pat.ID) {
		go func(id uuid.UUID) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = q.TouchPATLastUsed(ctx, id)
		}(pat.ID)
	}
	return pat.UserID, true
}

// CurrentUser returns the authenticated user id set by Auth (uuid.Nil if absent).
func CurrentUser(c *gin.Context) uuid.UUID {
	v, _ := c.Get(ContextUserID)
	id, _ := v.(uuid.UUID)
	return id
}
