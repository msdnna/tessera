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

// adminUserView is the instance-wide user shape shown in the admin panel: enough
// to identify, gauge (admin / active / verified) and manage an account. No
// profile/legal-name detail — this is an operator list, not a profile view.
type adminUserView struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	Name          string    `json:"name"`
	IsAdmin       bool      `json:"is_admin"`
	Active        bool      `json:"active"`
	EmailVerified bool      `json:"email_verified"`
	CreatedAt     time.Time `json:"created_at"`
	AvatarURL     string    `json:"avatar_url,omitempty"`
}

// requireGlobalAdmin resolves the caller and ensures they're a global admin,
// writing 403 and returning ok=false otherwise.
func (h *API) requireGlobalAdmin(c *gin.Context) (db.User, bool) {
	caller, err := h.q.GetUserByID(c, middleware.CurrentUser(c))
	if err != nil || !caller.IsAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "requires admin"})
		return db.User{}, false
	}
	return caller, true
}

// ListAllUsers returns every account on the instance (global admin only) for the
// admin panel.
func (h *API) ListAllUsers(c *gin.Context) {
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	rows, err := h.q.ListUsers(c)
	if err != nil {
		fail(c, err)
		return
	}
	out := make([]adminUserView, 0, len(rows))
	for _, u := range rows {
		v := adminUserView{
			ID: u.ID.String(), Email: u.Email, Name: u.Name, IsAdmin: u.IsAdmin,
			Active: u.Active, EmailVerified: u.EmailVerified, CreatedAt: u.CreatedAt,
		}
		if has, herr := h.q.UserHasAvatar(c, u.ID); herr == nil && has {
			v.AvatarURL = "/api/users/" + u.ID.String() + "/avatar"
		}
		out = append(out, v)
	}
	c.JSON(http.StatusOK, out)
}

// SetUserAdmin grants/revokes the global-admin flag (global admin only). You
// can't change your own — a sole admin can't accidentally lock the instance out.
func (h *API) SetUserAdmin(c *gin.Context) {
	caller, ok := h.requireGlobalAdmin(c)
	if !ok {
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	if id == caller.ID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot change your own admin role"})
		return
	}
	var req struct {
		Admin bool `json:"admin"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.q.SetUserAdmin(c, db.SetUserAdminParams{ID: id, IsAdmin: req.Admin}); err != nil {
		fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// CreateUserResetLink mints a password-reset token for any account (global admin
// only) and returns the link, so an operator can hand it to a user who's locked
// out (works without SMTP). Same token kind/TTL as the self-service flow.
func (h *API) CreateUserResetLink(c *gin.Context) {
	if _, ok := h.requireGlobalAdmin(c); !ok {
		return
	}
	id, ok := parseID(c, "id")
	if !ok {
		return
	}
	user, err := h.q.GetUserByID(c, id)
	if notFound(c, err) {
		return
	}
	if err != nil {
		fail(c, err)
		return
	}
	raw, hash, err := auth.NewRefreshToken()
	if err != nil {
		fail(c, err)
		return
	}
	if _, err := h.q.CreateUserToken(c, db.CreateUserTokenParams{
		UserID: user.ID, Kind: "reset", TokenHash: hash, ExpiresAt: time.Now().Add(resetTokenTTL),
	}); err != nil {
		fail(c, err)
		return
	}
	link := fmt.Sprintf("%s/recover?token=%s", strings.TrimRight(h.publicURL, "/"), raw)
	// Best-effort email too, so the user gets it directly when SMTP is configured.
	// The admin acts, the user reads: the letter is in the user's language.
	subject, body := mail.Compose(mail.KindAdminReset, h.userLang(c, user.ID), mail.Vars{
		Link: link, TTLHours: int(resetTokenTTL / time.Hour),
	})
	mail.SendAsync(h.mailer, user.Email, subject, body)
	c.JSON(http.StatusOK, gin.H{"link": link})
}
