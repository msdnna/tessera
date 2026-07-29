package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/auth"
	"tessera/internal/db"
	"tessera/middleware"
)

// patDTO is the safe, listable view of a personal access token — never the
// plaintext or the stored hash.
type patDTO struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	LastFour   string     `json:"last_four"`
	ExpiresAt  *time.Time `json:"expires_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
	CreatedAt  time.Time  `json:"created_at"`
}

// CreatePAT mints a new personal access token for the caller. The plaintext
// token is returned exactly once (only its hash is stored).
func (h *API) CreatePAT(c *gin.Context) {
	var req struct {
		Name      string     `json:"name" binding:"required"`
		ExpiresAt *time.Time `json:"expires_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	token, hash, err := auth.NewPAT()
	if err != nil {
		fail(c)
		return
	}
	pat, err := h.q.CreatePAT(c, db.CreatePATParams{
		UserID:    middleware.CurrentUser(c),
		Name:      req.Name,
		TokenHash: hash,
		LastFour:  token[len(token)-4:],
		ExpiresAt: req.ExpiresAt,
	})
	if err != nil {
		fail(c)
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		// The plaintext is shown only here — the client must store it now.
		"token": token,
		"pat": patDTO{
			ID: pat.ID, Name: pat.Name, LastFour: pat.LastFour,
			ExpiresAt: pat.ExpiresAt, RevokedAt: pat.RevokedAt,
			LastUsedAt: pat.LastUsedAt, CreatedAt: pat.CreatedAt,
		},
	})
}

// ListPATs returns the caller's personal access tokens (no plaintext).
func (h *API) ListPATs(c *gin.Context) {
	rows, err := h.q.ListPATsByUser(c, middleware.CurrentUser(c))
	if err != nil {
		fail(c)
		return
	}
	out := make([]patDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, patDTO{
			ID: r.ID, Name: r.Name, LastFour: r.LastFour,
			ExpiresAt: r.ExpiresAt, RevokedAt: r.RevokedAt,
			LastUsedAt: r.LastUsedAt, CreatedAt: r.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, out)
}

// RevokePAT revokes one of the caller's tokens. Idempotent: revoking an already
// revoked or unknown token still returns 204 (the token is not usable either way).
func (h *API) RevokePAT(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if err := h.q.RevokePAT(c, db.RevokePATParams{
		ID:     id,
		UserID: middleware.CurrentUser(c),
	}); err != nil {
		fail(c)
		return
	}
	c.Status(http.StatusNoContent)
}
