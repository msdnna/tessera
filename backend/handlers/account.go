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

// acceptPendingInvitations applies any workspace invitations addressed to the
// user's email (called on register so an invited user lands in the workspace).
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
	c.Status(http.StatusNoContent)
}
