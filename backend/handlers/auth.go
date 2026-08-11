package handlers

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgconn"

	"tessera/internal/auth"
	"tessera/internal/db"
	"tessera/internal/mail"
	"tessera/internal/secrets"
	"tessera/middleware"
)

// AuthHandler serves the authentication endpoints (login, refresh, invites, reset,
// GitLab OAuth).
type AuthHandler struct {
	q         *db.Queries
	secret    string
	sealer    *secrets.Sealer // decrypts the OAuth client secret at rest
	mailer    mail.Mailer
	publicURL string
}

// NewAuthHandler returns an AuthHandler with the given queries, signing secret,
// encryption key (for the OAuth client secret), mailer, and public base URL.
func NewAuthHandler(q *db.Queries, secret, encryptionKey string, mailer mail.Mailer, publicURL string) *AuthHandler {
	sealer, err := secrets.NewSealer(encryptionKey)
	if err != nil {
		log.Fatalf("failed to init auth secret sealer: %v", err)
	}
	return &AuthHandler{q: q, secret: secret, sealer: sealer, mailer: mailer, publicURL: publicURL}
}

type authResponse struct {
	AccessToken string `json:"access_token"`
	// Omitted in cookie mode: there the token goes into an httpOnly cookie and
	// must never reach the page's JavaScript. See handlers/auth_cookie.go.
	RefreshToken string   `json:"refresh_token,omitempty"`
	User         userDTO  `json:"user"`
	Preferences  prefsDTO `json:"preferences"`
}

// Register creates a user. The very first registered user becomes an admin.
func (h *AuthHandler) Register(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Name     string `json:"name" binding:"required"`
		Password string `json:"password" binding:"required,min=8"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	count, err := h.q.CountUsers(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	user, err := h.q.CreateUser(c, db.CreateUserParams{
		Email:        strings.ToLower(req.Email),
		Name:         req.Name,
		PasswordHash: hash,
		IsAdmin:      count == 0,
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			c.JSON(http.StatusConflict, gin.H{"error": "email already registered"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}

	// Every new user gets a personal workspace they own. Shared workspaces are
	// created explicitly via POST /workspaces.
	if ws, err := h.q.CreateWorkspace(c, db.CreateWorkspaceParams{
		Name: "Личное пространство", OwnerID: user.ID,
	}); err == nil {
		_, _ = h.q.CreateMembership(c, db.CreateMembershipParams{
			WorkspaceID: ws.ID, UserID: user.ID, Role: "owner",
		})
	}

	// Apply any pending workspace invitations addressed to this email, then send
	// a verification email (no-op mailer just logs the link when SMTP is off).
	h.acceptPendingInvitations(c, user)
	h.sendVerification(c, user)

	h.issue(c, user)
}

// Login authenticates by email + password.
func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.q.GetUserByEmail(c, strings.ToLower(req.Email))
	if err != nil {
		// No such user. The short-circuit below would skip bcrypt entirely, so a
		// missing-email response arrives in microseconds while a wrong-password
		// one takes a full bcrypt round — a timing oracle for email enumeration.
		// Pay the bcrypt cost against a fictional hash regardless; the result is
		// discarded (there is no account to log into) and the response stays the
		// same 401 either way.
		_ = auth.CheckPassword(auth.DummyHash(), req.Password)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if !user.Active {
		c.JSON(http.StatusForbidden, gin.H{"error": "account is deactivated"})
		return
	}

	h.issue(c, user)
}

// Refresh rotates a refresh token: the presented one is revoked and a fresh
// access + refresh pair is issued. The token comes from the httpOnly cookie when
// there is one, otherwise from the request body (Android, desktop, scripts).
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req struct {
		// Not `binding:"required"`: a cookie-mode client posts an empty body,
		// its token travelling in the cookie instead.
		RefreshToken string `json:"refresh_token"`
	}
	// A malformed or absent body is not an error by itself — it only leaves us
	// without a token, which the check below answers with a 401.
	_ = c.ShouldBindJSON(&req)

	token, fromCookie := refreshTokenFromRequest(c, req.RefreshToken)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid refresh token"})
		return
	}
	// A cookie the server won't accept can't be deleted by the SPA (that is the
	// point of httpOnly), so every rejection below has to clear it — otherwise
	// the browser keeps presenting a dead token on every attempt and the user
	// can never get back to a clean login.
	reject := func(status int, msg string) {
		if fromCookie {
			h.clearRefreshCookie(c)
		}
		c.JSON(status, gin.H{"error": msg})
	}

	hash := auth.HashRefreshToken(token)
	rt, err := h.q.GetRefreshToken(c, hash)
	if err != nil {
		reject(http.StatusUnauthorized, "invalid refresh token")
		return
	}
	if rt.RevokedAt != nil || time.Now().After(rt.ExpiresAt) {
		reject(http.StatusUnauthorized, "refresh token expired or revoked")
		return
	}

	if err := h.q.RevokeRefreshToken(c, hash); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	user, err := h.q.GetUserByID(c, rt.UserID)
	if err != nil {
		reject(http.StatusUnauthorized, "user not found")
		return
	}

	// Rotation stays in the channel it arrived on: a cookie-mode client gets the
	// new token back as a cookie even if it forgot the header.
	h.issueMode(c, user, fromCookie || wantsCookieAuth(c))
}

// Logout revokes the presented refresh token and clears the cookie. Without it a
// cookie-mode session could not be ended at all — JavaScript cannot delete an
// httpOnly cookie — and, before this endpoint existed, "logging out" left the
// refresh row usable for its full 30-day lifetime.
func (h *AuthHandler) Logout(c *gin.Context) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = c.ShouldBindJSON(&req)

	token, _ := refreshTokenFromRequest(c, req.RefreshToken)
	// Unconditionally: the caller asked to be logged out, so the cookie goes
	// even if the token turns out to be unknown.
	h.clearRefreshCookie(c)

	if token != "" {
		if err := h.q.RevokeRefreshToken(c, auth.HashRefreshToken(token)); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
	}
	// Idempotent by design: an unknown, expired or already-revoked token is
	// still a successful logout. Answering 404 would only tell an attacker
	// which stolen tokens are live.
	c.Status(http.StatusNoContent)
}

// Me returns the authenticated user's profile.
func (h *AuthHandler) Me(c *gin.Context) {
	user, err := h.q.GetUserByID(c, middleware.CurrentUser(c))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"user":        buildUserDTO(c, h.q, user),
		"preferences": loadPrefsDTO(c, h.q, user.ID),
	})
}

// mintTokens creates an access + refresh pair and stores the refresh hash. On
// failure it writes a 500 and returns ok=false. Shared by issue() (JSON response)
// and the OAuth callback (redirect handoff).
func (h *AuthHandler) mintTokens(c *gin.Context, user db.User) (access, refresh string, ok bool) {
	access, err := auth.NewAccessToken(h.secret, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return "", "", false
	}
	refresh, hash, err := auth.NewRefreshToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return "", "", false
	}
	if _, err := h.q.CreateRefreshToken(c, db.CreateRefreshTokenParams{
		UserID:    user.ID,
		TokenHash: hash,
		ExpiresAt: time.Now().Add(auth.RefreshTokenTTL),
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return "", "", false
	}
	return access, refresh, true
}

// issue mints an access + refresh pair and returns them with the user profile,
// honouring the delivery mode the client asked for.
func (h *AuthHandler) issue(c *gin.Context, user db.User) {
	h.issueMode(c, user, wantsCookieAuth(c))
}

// issueMode is issue() with the delivery mode decided by the caller — Refresh
// needs it, because a token that arrived in a cookie goes back in a cookie
// regardless of headers.
func (h *AuthHandler) issueMode(c *gin.Context, user db.User, cookieMode bool) {
	access, refresh, ok := h.mintTokens(c, user)
	if !ok {
		return
	}
	res := authResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		User:         buildUserDTO(c, h.q, user),
		Preferences:  loadPrefsDTO(c, h.q, user.ID),
	}
	if cookieMode {
		h.setRefreshCookie(c, refresh)
		res.RefreshToken = "" // omitempty drops it from the body entirely
	}
	c.JSON(http.StatusOK, res)
}
