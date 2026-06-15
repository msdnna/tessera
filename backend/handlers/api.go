// Package handlers implements the HTTP resource handlers for the Tessera API,
// sharing dependencies and helpers through the API type.
package handlers

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/internal/mail"
	"tessera/internal/realtime"
	"tessera/internal/secrets"
	"tessera/middleware"
)

// API holds the dependencies shared by all resource handlers.
type API struct {
	q         *db.Queries
	hub       *realtime.Hub
	uploadDir string
	sealer    *secrets.Sealer // encrypts secrets at rest (GitLab PATs)
	assetKey  []byte          // HMAC key for signed GitLab asset-proxy URLs
	mailer    mail.Mailer     // transactional email (invitations); no-op when SMTP unset
	publicURL string          // external base URL for links in emails
}

// NewAPI wires the shared handler dependencies, building the secret sealer from
// the configured encryption key.
func NewAPI(q *db.Queries, hub *realtime.Hub, uploadDir, encryptionKey string, mailer mail.Mailer, publicURL string) *API {
	sealer, err := secrets.NewSealer(encryptionKey)
	if err != nil {
		// config.New guarantees a non-empty key, so this is unreachable in practice.
		log.Fatalf("failed to init secret sealer: %v", err)
	}
	ak := sha256.Sum256([]byte(encryptionKey + ":gitlab-asset"))
	return &API{q: q, hub: hub, uploadDir: uploadDir, sealer: sealer, assetKey: ak[:], mailer: mailer, publicURL: publicURL}
}

// positionGap is the spacing used when appending to the end of a list.
const positionGap = 65536.0

// positionBetween computes a float position for an item dropped between two
// neighbours (classic kanban-style midpoint). nil prev = top of list, nil next = end.
func positionBetween(prev, next *float64) float64 {
	switch {
	case prev != nil && next != nil:
		return (*prev + *next) / 2
	case prev != nil:
		return *prev + positionGap
	case next != nil:
		return *next / 2
	default:
		return positionGap
	}
}

// requireMember authorizes the current user against a workspace. It writes a
// 403 and returns false when the user is not a member.
func (h *API) requireMember(c *gin.Context, workspaceID uuid.UUID) bool {
	_, err := h.q.GetMembership(c, db.GetMembershipParams{
		WorkspaceID: workspaceID,
		UserID:      middleware.CurrentUser(c),
	})
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "not a member of this workspace"})
		return false
	}
	return true
}

// memberRole returns the caller's role in a workspace, or "" if not a member.
func (h *API) memberRole(c *gin.Context, workspaceID uuid.UUID) string {
	m, err := h.q.GetMembership(c, db.GetMembershipParams{
		WorkspaceID: workspaceID,
		UserID:      middleware.CurrentUser(c),
	})
	if err != nil {
		return ""
	}
	return m.Role
}

// requireManager authorizes the caller as owner or admin of a workspace — the
// roles that manage members, roles and workspace settings (permission matrix,
// U1). Writes a 403 and returns false otherwise (non-members included).
func (h *API) requireManager(c *gin.Context, workspaceID uuid.UUID) bool {
	switch h.memberRole(c, workspaceID) {
	case "owner", "admin":
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "requires admin or owner"})
	return false
}

// broadcast fans a domain event out to connected clients scoped to a workspace.
func (h *API) broadcast(workspaceID uuid.UUID, eventType string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.hub.Broadcast(realtime.Event{Scope: workspaceID.String(), Type: eventType, Data: data})
}

// parseID reads a uuid path param, writing 400 on failure.
func parseID(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid " + name})
		return uuid.Nil, false
	}
	return id, true
}

// notFound reports whether err is pgx.ErrNoRows and, if so, writes 404.
func notFound(c *gin.Context, err error) bool {
	if errors.Is(err, pgx.ErrNoRows) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return true
	}
	return false
}

// fail writes a generic 500.
func fail(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
}
