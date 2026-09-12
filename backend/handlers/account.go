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

const (
	verifyTokenTTL = 48 * time.Hour
	resetTokenTTL  = 1 * time.Hour
)

// sendVerification issues an email-verification token and emails the link.
func (h *AuthHandler) sendVerification(c *gin.Context, user db.User) {
	if user.EmailVerified {
		return
	}
	raw, hash, err := auth.NewRefreshToken()
	if err != nil {
		return
	}
	if _, err := h.q.CreateUserToken(c, db.CreateUserTokenParams{
		UserID: user.ID, Kind: "verify", TokenHash: hash, ExpiresAt: time.Now().Add(verifyTokenTTL),
	}); err != nil {
		return
	}
	link := fmt.Sprintf("%s/verify-email?token=%s", strings.TrimRight(h.publicURL, "/"), raw)
	subject, body := mail.Compose(mail.KindVerify, h.userLang(c, user.ID), mail.Vars{
		Link: link, TTLHours: int(verifyTokenTTL / time.Hour),
	})
	mail.SendAsync(h.mailer, user.Email, subject, body)
}

// acceptPendingInvitations applies every workspace invitation addressed to the
// user's email. The email address alone is the key here, so this may only run
// once the account has proven it owns that mailbox: after email verification,
// or right after OAuth provisioning (the provider vouches for the address).
// Plain registration must not call it — typing an address is not owning it, and
// invitations stay pending for a week (see acceptInvitationToken).
func (h *AuthHandler) acceptPendingInvitations(c *gin.Context, user db.User) {
	invites, err := h.q.ListPendingInvitationsByEmail(c, user.Email)
	if err != nil {
		return
	}
	for _, inv := range invites {
		if _, err := h.q.CreateMembership(c, db.CreateMembershipParams{
			WorkspaceID: inv.WorkspaceID, UserID: user.ID, Role: inv.Role,
		}); err == nil {
			soft(c, "MarkInvitationAccepted", h.q.MarkInvitationAccepted(c, inv.ID))
		}
	}
}

// acceptInvitationToken applies the single invitation the raw token identifies,
// used at registration so an invitee who followed the emailed link lands in the
// workspace straight away. The token is the proof of mailbox ownership, so the
// invitation must be addressed to exactly the email being registered; anything
// off (unknown, expired, already accepted, different address) silently grants
// nothing — registration itself still succeeds.
func (h *AuthHandler) acceptInvitationToken(c *gin.Context, user db.User, rawToken string) {
	if rawToken == "" {
		return
	}
	inv, err := h.q.GetInvitationByHash(c, auth.HashRefreshToken(rawToken))
	if err != nil || inv.AcceptedAt != nil || time.Now().After(inv.ExpiresAt) {
		return
	}
	if !strings.EqualFold(inv.Email, user.Email) {
		return
	}
	if _, err := h.q.CreateMembership(c, db.CreateMembershipParams{
		WorkspaceID: inv.WorkspaceID, UserID: user.ID, Role: inv.Role,
	}); err == nil {
		soft(c, "MarkInvitationAccepted", h.q.MarkInvitationAccepted(c, inv.ID))
	}
}

// VerifyEmail consumes a verification token and marks the email verified.
func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tok, err := h.q.GetUserToken(c, db.GetUserTokenParams{TokenHash: auth.HashRefreshToken(req.Token), Kind: "verify"})
	if err != nil || tok.UsedAt != nil || time.Now().After(tok.ExpiresAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired token"})
		return
	}
	if err := h.q.MarkEmailVerified(c, tok.UserID); err != nil {
		fail(c, err)
		return
	}
	soft(c, "MarkUserTokenUsed", h.q.MarkUserTokenUsed(c, tok.ID))
	// Mailbox ownership is proven now, so anything still addressed to it can be
	// applied — this is how an invitee who registered without following the
	// emailed link finally lands in the workspace.
	if user, err := h.q.GetUserByID(c, tok.UserID); err == nil {
		h.acceptPendingInvitations(c, user)
	}
	c.Status(http.StatusNoContent)
}

// ResendVerification re-sends the verification email to the current user.
func (h *AuthHandler) ResendVerification(c *gin.Context) {
	user, err := h.q.GetUserByID(c, middleware.CurrentUser(c))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	soft(c, "DeleteUserTokensOfKind", h.q.DeleteUserTokensOfKind(c, db.DeleteUserTokensOfKindParams{UserID: user.ID, Kind: "verify"}))
	h.sendVerification(c, user)
	c.Status(http.StatusNoContent)
}

// ForgotPassword emails a reset link. Always returns 200 (no account enumeration).
func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	var req struct {
		Email string `json:"email" binding:"required,email"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if user, err := h.q.GetUserByEmail(c, strings.ToLower(req.Email)); err == nil {
		soft(c, "DeleteUserTokensOfKind", h.q.DeleteUserTokensOfKind(c, db.DeleteUserTokensOfKindParams{UserID: user.ID, Kind: "reset"}))
		if raw, hash, e := auth.NewRefreshToken(); e == nil {
			if _, e := h.q.CreateUserToken(c, db.CreateUserTokenParams{
				UserID: user.ID, Kind: "reset", TokenHash: hash, ExpiresAt: time.Now().Add(resetTokenTTL),
			}); e == nil {
				link := fmt.Sprintf("%s/recover?token=%s", strings.TrimRight(h.publicURL, "/"), raw)
				subject, body := mail.Compose(mail.KindReset, h.userLang(c, user.ID), mail.Vars{
					Link: link, TTLHours: int(resetTokenTTL / time.Hour),
				})
				mail.SendAsync(h.mailer, user.Email, subject, body)
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ResetPassword sets a new password from a reset token and revokes sessions.
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req struct {
		Token       string `json:"token" binding:"required"`
		NewPassword string `json:"new_password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tok, err := h.q.GetUserToken(c, db.GetUserTokenParams{TokenHash: auth.HashRefreshToken(req.Token), Kind: "reset"})
	if err != nil || tok.UsedAt != nil || time.Now().After(tok.ExpiresAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid or expired token"})
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		fail(c, err)
		return
	}
	if err := h.q.UpdateUserPassword(c, db.UpdateUserPasswordParams{ID: tok.UserID, PasswordHash: hash}); err != nil {
		fail(c, err)
		return
	}
	soft(c, "MarkUserTokenUsed", h.q.MarkUserTokenUsed(c, tok.ID))
	soft(c, "RevokeAllUserTokens", h.q.RevokeAllUserTokens(c, tok.UserID)) // log out other sessions
	h.dropSockets(tok.UserID, "password reset")
	c.Status(http.StatusNoContent)
}

// ChangeMyPassword changes the caller's password after verifying the current one
// and, like ResetPassword above, ends every session the old password was behind.
//
// The two are the same act from different doors — one from the login form, one
// from a live session — and the in-session one is the door a user reaches for on
// noticing someone else is in the account. Leaving the refresh chain alone there
// meant the intruder's token went on rotating itself for its full thirty days,
// renewing past the new password indefinitely; the user, meanwhile, had done the
// one thing the interface offers for exactly this.
//
// Revocation is unconditional, which includes the caller's own pair — the access
// token carries no session id, so there is nothing to tell their chain apart
// from anyone else's. The replacement is handed back in the response instead, so
// the device performing the change stays signed in on a chain the old password
// never reached. That makes this a 200 carrying a session where it used to be a
// 204; a client that ignores the body is no worse off than one whose user reset
// the password from the login form.
//
// Revocation reaches the sockets too, not just the refresh chain (#2932). A
// WebSocket authorises exactly once, at the upgrade, and then lives on ping/pong
// indefinitely — so an intruder's open tab went on receiving every event of every
// workspace the account belongs to, holding block locks in documents and sitting
// in calls, long after the password that let them in had been replaced. The
// caller's own sockets go down with the rest; every client reconnects by itself,
// now on the session this response hands back.
//
// Personal access tokens are deliberately left alone. They are separate
// credentials with their own revoke button, minted for CI and MCP clients that
// no one is watching, and a routine password rotation is not a statement about
// them. Reset does not touch them either.
func (h *AuthHandler) ChangeMyPassword(c *gin.Context) {
	uid := middleware.CurrentUser(c)
	var req struct {
		CurrentPassword string `json:"current_password" binding:"required"`
		NewPassword     string `json:"new_password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	u, err := h.q.GetUserByID(c, uid)
	if err != nil {
		fail(c, err)
		return
	}
	if !auth.CheckPassword(u.PasswordHash, req.CurrentPassword) {
		c.JSON(http.StatusForbidden, gin.H{"error": "current password is incorrect"})
		return
	}
	hash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		fail(c, err)
		return
	}
	if err := h.q.UpdateUserPassword(c, db.UpdateUserPasswordParams{ID: uid, PasswordHash: hash}); err != nil {
		fail(c, err)
		return
	}
	soft(c, "RevokeAllUserTokens", h.q.RevokeAllUserTokens(c, uid))
	h.dropSockets(uid, "password changed")
	h.issue(c, u)
}
