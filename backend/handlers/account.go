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

// sendVerification issues an email-verification link and a one-time code
// (#3057) and emails both: the link for whoever reads mail on another device,
// the code for the «Проверьте почту» cells on this one.
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
	code, err := h.issueCode(c, user.ID, kindVerifyCode, nil, true)
	if err != nil {
		soft(c, "issueCode", err)
	}
	link := fmt.Sprintf("%s/verify-email?token=%s", strings.TrimRight(h.publicURL, "/"), raw)
	subject, body := mail.Compose(mail.KindVerify, h.userLang(c, user.ID), mail.Vars{
		Link: link, TTLHours: int(verifyTokenTTL / time.Hour),
		Code: code, CodeMinutes: int(codeTTL / time.Minute),
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
	granted := false
	for _, inv := range invites {
		// A pending invitation may raise the role, never lower one granted since
		// (see invitationRaisesRole) — this path fires on every OAuth sign-in, so a
		// stale one would demote the same admin again after each re-grant.
		if !invitationRaisesRole(c, h.q, inv.WorkspaceID, user.ID, inv.Role) {
			soft(c, "MarkInvitationAccepted", h.q.MarkInvitationAccepted(c, inv.ID))
			continue
		}
		if _, err := h.q.CreateMembership(c, db.CreateMembershipParams{
			WorkspaceID: inv.WorkspaceID, UserID: user.ID, Role: inv.Role,
		}); err == nil {
			granted = true
			soft(c, "MarkInvitationAccepted", h.q.MarkInvitationAccepted(c, inv.ID))
		}
	}
	// The verification link is clicked with the app already open in another tab:
	// that session's socket was scoped before this grant and will never hear
	// about the workspace it just joined (see AuthHandler.rescope).
	if granted {
		h.rescope(user.ID)
	}
}

// What became of an invitation token presented at registration. Reported back
// to the client (authResponse.Invite) because the account is created either way:
// the outcome is the only thing that tells the newcomer apart from someone who
// signed up plainly, and it is never visible in the session itself.
const (
	inviteApplied       = "applied"
	inviteEmailMismatch = "email_mismatch"
	inviteInvalid       = "invalid"
	inviteFailed        = "failed"
)

// acceptInvitationToken applies the single invitation the raw token identifies,
// used at registration so an invitee who followed the emailed link lands in the
// workspace straight away. The token is the proof of mailbox ownership, so the
// invitation must be addressed to exactly the email being registered; anything
// off (unknown, expired, already accepted, different address) grants nothing —
// registration itself still succeeds.
//
// Returns what happened, "" when no token was presented. Granting nothing used
// to be silent, and the address is the one thing the invitee cannot check: the
// link is routinely copied out of CreateInvitation's response and pasted into a
// chat (that is what it is returned for when SMTP is off), so the invited
// mailbox is never seen and the register form gets whatever address its owner
// usually types. The reply was then indistinguishable from a joined one, and
// they landed on the board with a personal workspace and no membership, the only
// copy of the token left behind in the message (#2932).
func (h *AuthHandler) acceptInvitationToken(c *gin.Context, user db.User, rawToken string) string {
	if rawToken == "" {
		return ""
	}
	hash := auth.HashRefreshToken(rawToken)
	inv, err := h.q.GetInvitationByHash(c, hash)
	if err != nil {
		// The workspace's shared link (#3062) is not addressed to anyone: the
		// link itself is the whole credential, so it applies to whoever signs up
		// with it, up to its limit.
		return h.joinByLinkToken(c, user, hash)
	}
	if !invitationLive(inv, time.Now()) {
		return inviteInvalid
	}
	if !strings.EqualFold(inv.Email, user.Email) {
		return inviteEmailMismatch
	}
	if !invitationRaisesRole(c, h.q, inv.WorkspaceID, user.ID, inv.Role) {
		// Spent with nothing left to grant, which for the invitee is the same as
		// applied: they are a member of that workspace by the time they see the
		// board.
		soft(c, "MarkInvitationAccepted", h.q.MarkInvitationAccepted(c, inv.ID))
		return inviteApplied
	}
	if _, err := h.q.CreateMembership(c, db.CreateMembershipParams{
		WorkspaceID: inv.WorkspaceID, UserID: user.ID, Role: inv.Role,
	}); err != nil {
		return inviteFailed
	}
	soft(c, "MarkInvitationAccepted", h.q.MarkInvitationAccepted(c, inv.ID))
	return inviteApplied
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
	// The code sent with the link has nothing left to prove.
	soft(c, "SupersedeUserCodes", h.q.SupersedeUserCodes(c, db.SupersedeUserCodesParams{UserID: tok.UserID, Kinds: []string{kindVerifyCode}}))
	// Mailbox ownership is proven now, so anything still addressed to it can be
	// applied — this is how an invitee who registered without following the
	// emailed link finally lands in the workspace.
	if user, err := h.q.GetUserByID(c, tok.UserID); err == nil {
		h.acceptPendingInvitations(c, user)
	}
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

// CheckResetToken tells the «Новый пароль» screen whether its link still works
// and until when (#3060: «Ссылка действует ещё 54 минуты»), without spending
// it. The token is the credential — 256 random bits, throttled by IP like the
// reset itself — so the answer says nothing about any account: a live link
// gets its deadline, anything else the same refusal ResetPassword gives.
func (h *AuthHandler) CheckResetToken(c *gin.Context) {
	var req struct {
		Token string `json:"token" binding:"required"`
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
	c.JSON(http.StatusOK, gin.H{"expires_at": tok.ExpiresAt})
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
	// Checked before the token is looked up, let alone marked used: a reset link
	// is single-use, and spending it on a password that was never storable would
	// take away the user's one way back into the account.
	if rejectBadPassword(c, req.NewPassword) {
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
	if rejectBadPassword(c, req.NewPassword) {
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
