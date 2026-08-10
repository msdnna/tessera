// Package handlers implements the HTTP resource handlers for the Tessera API,
// sharing dependencies and helpers through the API type.
package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"tessera/internal/db"
	"tessera/internal/jobs"
	"tessera/internal/mail"
	"tessera/internal/notify"
	"tessera/internal/realtime"
	"tessera/internal/secrets"
	"tessera/middleware"
)

// API holds the dependencies shared by all resource handlers.
type API struct {
	q         *db.Queries
	pool      *pgxpool.Pool // for multi-statement transactions (e.g. project transfer)
	hub       *realtime.Hub
	uploadDir string
	sealer    *secrets.Sealer          // encrypts secrets at rest (GitLab PATs, channel secrets)
	assetKey  []byte                   // HMAC key for signed GitLab asset-proxy URLs
	mailer    mail.Mailer              // transactional email (invitations); no-op when SMTP unset
	publicURL string                   // external base URL for links in emails
	senders   map[string]notify.Sender // notification channel transports, keyed by type
	jobs      *jobs.Registry           // in-memory registry of background jobs (observability + cancel)
}

// NewAPI wires the shared handler dependencies, building the secret sealer from
// the configured encryption key.
func NewAPI(q *db.Queries, pool *pgxpool.Pool, hub *realtime.Hub, uploadDir, encryptionKey string, mailer mail.Mailer, publicURL string) *API {
	sealer, err := secrets.NewSealer(encryptionKey)
	if err != nil {
		// config.New guarantees a non-empty key, so this is unreachable in practice.
		log.Fatalf("failed to init secret sealer: %v", err)
	}
	ak := sha256.Sum256([]byte(encryptionKey + ":gitlab-asset"))
	// A TextHandler renders the background-task summary in logfmt (key=value), with
	// the call site as source=… — the format the ops log asks for.
	jobLog := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{AddSource: true}))
	return &API{
		q: q, pool: pool, hub: hub, uploadDir: uploadDir, sealer: sealer, assetKey: ak[:],
		mailer: mailer, publicURL: publicURL, senders: buildSenders(mailer), jobs: jobs.New(jobLog),
	}
}

// positionGap is the spacing used when appending to the end of a list.
const positionGap = 65536.0

// positionBetween computes a float position for an item dropped between two
// neighbours (float midpoint). nil prev = top of list, nil next = end.
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

// membershipResult is the cached outcome of a per-request membership lookup.
type membershipResult struct {
	role string
	ok   bool
}

// lookupMembership resolves the caller's membership in a workspace, memoized for
// the lifetime of the request. A single request often checks membership more than
// once (e.g. requireMember in a handler, then memberRole for a permission gate, or
// nested resource resolution) — without this each check was its own SQL round-trip.
func (h *API) lookupMembership(c *gin.Context, workspaceID uuid.UUID) membershipResult {
	key := "membership:" + workspaceID.String()
	if v, exists := c.Get(key); exists {
		return v.(membershipResult)
	}
	res := membershipResult{}
	m, err := h.q.GetMembership(c, db.GetMembershipParams{
		WorkspaceID: workspaceID,
		UserID:      middleware.CurrentUser(c),
	})
	if err == nil {
		res.role = m.Role
		res.ok = true
	}
	c.Set(key, res)
	return res
}

// requireMember authorizes the current user against a workspace. It writes a
// 403 and returns false when the user is not a member.
func (h *API) requireMember(c *gin.Context, workspaceID uuid.UUID) bool {
	if h.lookupMembership(c, workspaceID).ok {
		return true
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "not a member of this workspace"})
	return false
}

// memberRole returns the caller's role in a workspace, or "" if not a member.
func (h *API) memberRole(c *gin.Context, workspaceID uuid.UUID) string {
	return h.lookupMembership(c, workspaceID).role
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
	h.broadcastEvent(workspaceID, eventType, "", payload)
}

// broadcastAs is broadcast with the acting user attached, so clients can
// attribute the event to a person (board-activity toasts). Use it for
// user-initiated actions that carry a gin.Context.
func (h *API) broadcastAs(c *gin.Context, workspaceID uuid.UUID, eventType string, payload any) {
	h.broadcastEvent(workspaceID, eventType, middleware.CurrentUser(c).String(), payload)
}

// broadcastEvent marshals the payload and fans the event out; actor may be "".
func (h *API) broadcastEvent(workspaceID uuid.UUID, eventType, actor string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	h.hub.Broadcast(realtime.Event{
		Scope: workspaceID.String(), Type: eventType, Actor: actor, Data: data,
	})
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
// fail writes a generic 500 to the client and logs the real cause server-side,
// tagged with the request's method/path/id so a production log line can be tied
// back to a user report. The client only ever sees "internal error" — the cause
// never leaks. err may be nil (a handful of call sites fail without one).
func fail(c *gin.Context, err error) {
	slog.Error("request failed",
		"err", err,
		"method", c.Request.Method,
		"path", c.Request.URL.Path,
		"request_id", middleware.GetRequestID(c),
	)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
}

// soft records a best-effort operation's failure without touching the response.
// Ignoring the result is often deliberate (a due-date already stamped, a log row
// that didn't write), but staying silent about it makes "why did the sync
// overwrite this" unanswerable. The op label names the action for the log; a nil
// error is silent. ctx is a gin.Context or a worker's context.Context.
func soft(ctx context.Context, op string, err error) {
	if err == nil {
		return
	}
	slog.Warn("best-effort op failed",
		"op", op,
		"err", err,
		"request_id", middleware.GetRequestID(ctx),
	)
}

// inTx runs fn inside a database transaction, rolling back if fn returns an
// error (or panics on the way out) and committing otherwise. It centralises the
// begin/rollback/commit boilerplate so multi-statement mutations stay
// all-or-nothing — a partial apply leaves orphans (a workspace with no
// membership) or drops data (a DELETE that commits before its re-INSERT).
func (h *API) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after a successful Commit
	if err := fn(h.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
