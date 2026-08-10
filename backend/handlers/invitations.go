package handlers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

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
	mail.SendAsync(h.mailer, inv.Email, "Приглашение в пространство — Tessera",
		"Вас пригласили в рабочее пространство Tessera. Присоединиться:\n\n"+link+"\n\nСсылка действует 7 дней.")
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
	if err := h.q.DeleteInvitation(c, invID); err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
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
		if _, err := q.CreateMembership(c, db.CreateMembershipParams{
			WorkspaceID: inv.WorkspaceID, UserID: user.ID, Role: inv.Role,
		}); err != nil {
			return err
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
