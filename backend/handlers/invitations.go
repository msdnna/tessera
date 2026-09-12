package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/auth"
	"tessera/internal/db"
	"tessera/internal/mail"
	"tessera/middleware"
)

const invitationTTL = 7 * 24 * time.Hour

// invitationDTO is the public shape of an invitation (no token hash).
func invitationDTO(inv db.WorkspaceInvitation) gin.H {
	return gin.H{
		"id": inv.ID, "workspace_id": inv.WorkspaceID, "email": inv.Email,
		"role": inv.Role, "created_at": inv.CreatedAt, "expires_at": inv.ExpiresAt,
	}
}

// CreateInvitation invites an email to a workspace (owner/admin). The invitee
// need not have an account — registering with that email auto-joins. Emails the
// link (no-op mailer logs it); the link is also returned so the inviter can copy
// it when SMTP is off.
func (h *API) CreateInvitation(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireManager(c, id) {
		return
	}
	var req struct {
		Email string `json:"email" binding:"required,email"`
		Role  string `json:"role"`
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
	raw, hash, err := auth.NewRefreshToken()
	if err != nil {
		fail(c, err)
		return
	}
	inviter := middleware.CurrentUser(c)
	inv, err := h.q.CreateInvitation(c, db.CreateInvitationParams{
		WorkspaceID: id, Email: strings.ToLower(req.Email), Role: role,
		TokenHash: hash, InvitedBy: &inviter, ExpiresAt: time.Now().Add(invitationTTL),
	})
	if err != nil {
		fail(c, err)
		return
	}
	link := fmt.Sprintf("%s/invite?token=%s", strings.TrimRight(h.publicURL, "/"), raw)
	subject, body := mail.Compose(mail.KindInvitation, emailLang(c, h.q, inv.Email), mail.Vars{
		Link: link, TTLHours: int(invitationTTL / time.Hour),
	})
	mail.SendAsync(h.mailer, inv.Email, subject, body)
	out := invitationDTO(inv)
	out["link"] = link
	c.JSON(http.StatusCreated, out)
}

// ListInvitations lists a workspace's pending invitations (owner/admin).
func (h *API) ListInvitations(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireManager(c, id) {
		return
	}
	invs, err := h.q.ListInvitations(c, id)
	if err != nil {
		fail(c, err)
		return
	}
	out := make([]gin.H, 0, len(invs))
	for _, inv := range invs {
		out = append(out, invitationDTO(inv))
	}
	c.JSON(http.StatusOK, out)
}

// DeleteInvitation revokes a pending invitation (owner/admin of its workspace).
//
// The delete is scoped by the workspace from the URL, which is the one the
// caller's role was checked against. Unscoped, managing any workspace — one's
// own is enough — let a caller burn a pending invitation anywhere on the
// install; the door that needs no guessing is a demoted admin, who read the ids
// out of ListInvitations while the role lasted and can spend them for the whole
// week the invitations stay live.
func (h *API) DeleteInvitation(c *gin.Context) {
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if !h.requireManager(c, id) {
		return
	}
	invID, ok := parseID(c, "invId")
	if !ok {
		return
	}
	n, err := h.q.DeleteInvitation(c, db.DeleteInvitationParams{ID: invID, WorkspaceID: id})
	if err != nil {
		fail(c, err)
		return
	}
	if n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "invitation not found in this workspace"})
		return
	}
	c.Status(http.StatusNoContent)
}

// invitationRaisesRole reports whether an invitation still has something to
// grant — i.e. whether its role is above the one the user already holds here.
//
// The role in an invitation is a snapshot of an intention up to a week old,
// while CreateMembership is an upsert: applied as-is it overwrites whatever the
// members screen says today, so a stale "member" invitation silently takes back
// an admin granted after it was written (#2932). The statement already refuses
// to move an owner; this is the same rule for every role below it, and the same
// one org_map follows — an outside grant may raise a role, never lower it.
// Deliberate demotion keeps its own door: UpdateMemberRole.
//
// A user with no membership (and an unreadable one) counts as raising: there the
// invitation is the whole grant.
func invitationRaisesRole(ctx context.Context, q *db.Queries, wsID, userID uuid.UUID, role string) bool {
	existing, err := q.GetMembershipRole(ctx, db.GetMembershipRoleParams{WorkspaceID: wsID, UserID: userID})
	if err != nil {
		return true
	}
	return membershipRank(role) > membershipRank(existing)
}

// AcceptInvitation joins the current user to the invited workspace via a token.
func (h *API) AcceptInvitation(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	inv, err := h.q.GetInvitationByHash(c, auth.HashRefreshToken(req.Token))
	if err != nil || inv.AcceptedAt != nil || time.Now().After(inv.ExpiresAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired invitation"})
		return
	}
	user, err := h.q.GetUserByID(c, middleware.CurrentUser(c))
	if err != nil {
		fail(c, err)
		return
	}
	if !strings.EqualFold(user.Email, inv.Email) {
		c.JSON(http.StatusForbidden, gin.H{"error": "this invitation is for a different email"})
		return
	}
	// Transactional: membership and the accepted-mark must move together. Split,
	// a crash leaves either a membership with a still-live invite, or a burned
	// invite with no access granted.
	if err := h.inTx(c, func(q *db.Queries) error {
		// Burned either way: the invitation was spent, it just had nothing left to
		// grant. Leaving it pending would keep the demotion waiting another week.
		if invitationRaisesRole(c, q, inv.WorkspaceID, user.ID, inv.Role) {
			if _, err := q.CreateMembership(c, db.CreateMembershipParams{
				WorkspaceID: inv.WorkspaceID, UserID: user.ID, Role: inv.Role,
			}); err != nil {
				return err
			}
		}
		return q.MarkInvitationAccepted(c, inv.ID)
	}); err != nil {
		fail(c, err)
		return
	}
	// Live sockets snapshot their workspace set at connect; drop this user's so
	// they reconnect into the workspace they just joined.
	h.hub.DropUser(user.ID)
	ws, err := h.q.GetWorkspace(c, inv.WorkspaceID)
	if err != nil {
		c.Status(http.StatusNoContent)
		return
	}
	c.JSON(http.StatusOK, ws)
}
