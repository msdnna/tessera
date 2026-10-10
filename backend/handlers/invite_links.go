package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/auth"
	"tessera/internal/db"
	"tessera/middleware"
)

// The workspace's shared invitation link (#3062) and the public face of every
// invitation — the preview and «Отклонить» (#3059).
//
// A shared link is the opposite of an addressed invitation: it names nobody, so
// the link itself is the whole credential. What keeps it from being an open
// door is everything around it: only its hash is stored (the raw link is shown
// once, to the manager who made it), it expires, it admits at most max_uses
// people — counted atomically — and «Сбросить» revokes it in the same breath as
// it issues the next one. Each join is recorded against the link that granted
// it, which is the audit trail of who came in by which door.
const (
	inviteLinkDefaultTTL  = 7 * 24 * time.Hour
	inviteLinkMaxTTLDays  = 30
	inviteLinkDefaultUses = 25
	inviteLinkMaxUses     = 500
)

// errLinkSpent is a shared link that admits nobody any more.
var errLinkSpent = errors.New("invite link spent")

// invitationLive reports whether an addressed invitation can still be redeemed.
func invitationLive(inv db.WorkspaceInvitation, now time.Time) bool {
	return inv.AcceptedAt == nil && inv.DeclinedAt == nil && now.Before(inv.ExpiresAt)
}

// inviteLinkState names what a shared link can do now: "pending" admits.
func inviteLinkState(l db.WorkspaceInviteLink, now time.Time) string {
	switch {
	case l.RevokedAt != nil:
		return "revoked"
	case !now.Before(l.ExpiresAt):
		return "expired"
	case l.Uses >= l.MaxUses:
		return "exhausted"
	}
	return "pending"
}

// joinByLink admits the user through a shared link. Someone already in the
// workspace at the link's role or above spends no place — opening the link
// twice must not use up the seats meant for others.
func joinByLink(ctx context.Context, q *db.Queries, link db.WorkspaceInviteLink, userID uuid.UUID) (bool, error) {
	if inviteLinkState(link, time.Now()) != "pending" {
		return false, errLinkSpent
	}
	if !invitationRaisesRole(ctx, q, link.WorkspaceID, userID, link.Role) {
		return false, nil
	}
	if _, err := q.ConsumeInviteLink(ctx, link.ID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, errLinkSpent
		}
		return false, err
	}
	if _, err := q.CreateMembership(ctx, db.CreateMembershipParams{
		WorkspaceID: link.WorkspaceID, UserID: userID, Role: link.Role,
	}); err != nil {
		return false, err
	}
	if err := q.RecordInviteLinkUse(ctx, db.RecordInviteLinkUseParams{LinkID: link.ID, UserID: userID}); err != nil {
		return false, err
	}
	return true, nil
}

// joinByLinkToken is the registration leg: a shared link presented at sign-up.
// Reports the outcome in acceptInvitationToken's vocabulary.
func (h *AuthHandler) joinByLinkToken(c *gin.Context, user db.User, hash string) string {
	link, err := h.q.GetInviteLinkByHash(c, hash)
	if err != nil {
		return inviteInvalid
	}
	if _, err := joinByLink(c, h.q, link, user.ID); err != nil {
		if errors.Is(err, errLinkSpent) {
			return inviteInvalid
		}
		return inviteFailed
	}
	return inviteApplied
}

// acceptInviteLink is AcceptInvitation's leg for a shared link.
func (h *API) acceptInviteLink(c *gin.Context, hash string) {
	link, err := h.q.GetInviteLinkByHash(c, hash)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired invitation"})
		return
	}
	uid := middleware.CurrentUser(c)
	var joined bool
	if err := h.inTx(c, func(q *db.Queries) error {
		var jerr error
		joined, jerr = joinByLink(c, q, link, uid)
		return jerr
	}); err != nil {
		if errors.Is(err, errLinkSpent) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired invitation"})
			return
		}
		fail(c, err)
		return
	}
	if joined {
		h.hub.DropUser(uid)
	}
	ws, err := h.q.GetWorkspace(c, link.WorkspaceID)
	if err != nil {
		c.Status(http.StatusNoContent)
		return
	}
	c.JSON(http.StatusOK, ws)
}

// LookupInvitation is the invitation page's preview (#3059): who invites, into
// what, with which role — for an addressed invitation and a shared link alike.
// Public: the token is the credential, and the invitee often has no account
// yet. It only ever describes a door that is still open; a spent one answers
// with its state and nothing else, and an unknown token with the same refusal
// accepting it would give. Counts, never names: a link pasted into a chat must
// not become the workspace's roster.
func (h *API) LookupInvitation(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	now := time.Now()
	hash := auth.HashRefreshToken(req.Token)
	var (
		kind, state, role, email string
		wsID                     uuid.UUID
		inviter                  *uuid.UUID
		expires                  time.Time
	)
	if inv, err := h.q.GetInvitationByHash(c, hash); err == nil {
		kind, role, email, wsID, inviter, expires = "email", inv.Role, inv.Email, inv.WorkspaceID, inv.InvitedBy, inv.ExpiresAt
		switch {
		case inv.AcceptedAt != nil:
			state = "accepted"
		case inv.DeclinedAt != nil:
			state = "declined"
		case !now.Before(inv.ExpiresAt):
			state = "expired"
		default:
			state = "pending"
		}
	} else if link, lerr := h.q.GetInviteLinkByHash(c, hash); lerr == nil {
		kind, role, wsID, inviter, expires = "link", link.Role, link.WorkspaceID, link.CreatedBy, link.ExpiresAt
		state = inviteLinkState(link, now)
	} else {
		c.JSON(http.StatusNotFound, gin.H{"error": "invalid or expired invitation"})
		return
	}
	if state != "pending" {
		c.JSON(http.StatusOK, gin.H{"kind": kind, "state": state})
		return
	}
	ws, err := h.q.GetWorkspace(c, wsID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "invalid or expired invitation"})
		return
	}
	counts, err := h.q.InvitePreviewCounts(c, wsID)
	if err != nil {
		fail(c, err)
		return
	}
	out := gin.H{
		"kind": kind, "state": state, "role": role, "expires_at": expires,
		"workspace": gin.H{"name": ws.Name, "name_key": ws.NameKey, "color": ws.Color},
		"projects":  counts.Projects, "members": counts.Members,
	}
	if email != "" {
		out["email"] = email
	}
	if inviter != nil {
		if u, err := h.q.GetUserByID(c, *inviter); err == nil {
			out["inviter"] = gin.H{"name": u.Name}
		}
	}
	c.JSON(http.StatusOK, out)
}

// DeclineInvitation is «Отклонить» (#3059). Public like the preview: whoever
// holds the token may turn the invitation down — they could as well accept it.
// The invitation stays declined until a manager sends it again; proving the
// mailbox later does not accept it behind its owner's back
// (ListPendingInvitationsByEmail skips declined ones). A shared link names
// nobody, so there is nothing to decline: its page just leaves.
func (h *API) DeclineInvitation(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	inv, err := h.q.GetInvitationByHash(c, auth.HashRefreshToken(req.Token))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired invitation"})
		return
	}
	n, err := h.q.DeclineInvitation(c, inv.ID)
	if err != nil {
		fail(c, err)
		return
	}
	if n == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired invitation"})
		return
	}
	c.Status(http.StatusNoContent)
}

// inviteLinkDTO is the public shape of a shared link — never its token.
func inviteLinkDTO(l db.WorkspaceInviteLink) gin.H {
	return gin.H{
		"id": l.ID, "role": l.Role, "max_uses": l.MaxUses, "uses": l.Uses,
		"expires_at": l.ExpiresAt, "created_at": l.CreatedAt, "created_by": l.CreatedBy,
		"state": inviteLinkState(l, time.Now()),
	}
}

// GetInviteLink returns the workspace's live shared link, or {"link": null}.
// The URL itself is not part of it: only its hash is kept (see the top).
func (h *API) GetInviteLink(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireManager(c, id) {
		return
	}
	l, err := h.q.GetLiveInviteLink(c, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusOK, gin.H{"link": nil})
			return
		}
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"link": inviteLinkDTO(l)})
}

// CreateInviteLink issues the workspace's shared link — and is «Сбросить» too:
// the previous link is revoked in the same transaction, so there is never more
// than one door open. The raw URL is in this response and nowhere else.
func (h *API) CreateInviteLink(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireManager(c, id) {
		return
	}
	var req struct {
		Role    string `json:"role"`
		TTLDays int    `json:"ttl_days"`
		MaxUses int    `json:"max_uses"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	role := req.Role
	if role == "" {
		role = "member"
	}
	if !manageableRole(role) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be admin or member"})
		return
	}
	ttl := inviteLinkDefaultTTL
	if req.TTLDays != 0 {
		if req.TTLDays < 1 || req.TTLDays > inviteLinkMaxTTLDays {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("ttl_days must be between 1 and %d", inviteLinkMaxTTLDays)})
			return
		}
		ttl = time.Duration(req.TTLDays) * 24 * time.Hour
	}
	uses := req.MaxUses
	if uses == 0 {
		uses = inviteLinkDefaultUses
	}
	if uses < 1 || uses > inviteLinkMaxUses {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("max_uses must be between 1 and %d", inviteLinkMaxUses)})
		return
	}
	raw, hash, err := auth.NewRefreshToken()
	if err != nil {
		fail(c, err)
		return
	}
	uid := middleware.CurrentUser(c)
	var link db.WorkspaceInviteLink
	if err := h.inTx(c, func(q *db.Queries) error {
		if _, err := q.RevokeInviteLinks(c, db.RevokeInviteLinksParams{WorkspaceID: id, RevokedBy: &uid}); err != nil {
			return err
		}
		var cerr error
		link, cerr = q.CreateInviteLink(c, db.CreateInviteLinkParams{
			WorkspaceID: id, TokenHash: hash, Role: role, MaxUses: int32(uses), //nolint:gosec // bounded above
			CreatedBy: &uid, ExpiresAt: time.Now().Add(ttl),
		})
		return cerr
	}); err != nil {
		fail(c, err)
		return
	}
	out := inviteLinkDTO(link)
	out["url"] = fmt.Sprintf("%s/join/%s", strings.TrimRight(h.publicURL, "/"), raw)
	c.JSON(http.StatusCreated, gin.H{"link": out})
}

// RevokeInviteLink switches the shared link off; nobody else gets in by it.
func (h *API) RevokeInviteLink(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireManager(c, id) {
		return
	}
	uid := middleware.CurrentUser(c)
	if _, err := h.q.RevokeInviteLinks(c, db.RevokeInviteLinksParams{WorkspaceID: id, RevokedBy: &uid}); err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
