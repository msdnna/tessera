package handlers

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	// One-time codes for the desktop OAuth deep link — see oauth_handoff.go.
	handoff *oauthHandoffStore
	// revokeSockets cuts the user's live WebSockets when their password changes
	// (#2932). Optional: tests that build an AuthHandler directly leave it nil,
	// and a nil revoker simply means "no sockets to cut here".
	revokeSockets func(uuid.UUID, string)
	// rescopeSockets re-resolves the workspaces a user's live board sockets may
	// receive events for, after this layer has *granted* a membership (#2932).
	// Optional, like revokeSockets.
	rescopeSockets func(uuid.UUID)
}

// WireSocketRevoker injects the socket revocation behaviour owned by API (see
// API.SocketRevoker). Called once at router construction, before any request.
func (h *AuthHandler) WireSocketRevoker(revoke func(uuid.UUID, string)) { h.revokeSockets = revoke }

// WireScopeRefresher injects the re-scoping behaviour owned by API (see
// API.ScopeRefresher). Called once at router construction, before any request.
func (h *AuthHandler) WireScopeRefresher(rescope func(uuid.UUID)) { h.rescopeSockets = rescope }

// dropSockets ends every live connection the user holds. Both password doors
// call it: a socket authenticates once, at the upgrade, and then lives on
// ping/pong for as long as the process does — so revoking the refresh chain
// alone leaves whoever the password change was aimed at still connected,
// receiving the account's events in real time.
func (h *AuthHandler) dropSockets(userID uuid.UUID, reason string) {
	if h.revokeSockets != nil {
		h.revokeSockets(userID, reason)
	}
}

// rescope tells the user's live board sockets that their workspace list grew.
//
// The scopes are resolved once, at the upgrade, and are immutable for the life
// of the connection — so every membership change has to reach the socket from
// the outside. Removal is loud (revokeSockets) and the workspace endpoints
// already drop the user on add/accept; this is the same reflex for the two
// grants that happen *inside* the auth layer, where a session is typically
// already open: applying a pending invitation on email verification, and the
// OAuth org map on sign-in. Without it the newcomer's board renders (the list
// refetches over HTTP) and then sits frozen until a reload they have no reason
// to perform.
//
// Deliberately not revokeSockets: nothing was taken away, so ending the
// person's call and evicting them from the document they have open would be a
// worse answer than the bug.
func (h *AuthHandler) rescope(userID uuid.UUID) {
	if h.rescopeSockets != nil {
		h.rescopeSockets(userID)
	}
}

// NewAuthHandler returns an AuthHandler with the given queries, signing secret,
// encryption key (for the OAuth client secret), mailer, and public base URL.
func NewAuthHandler(q *db.Queries, secret, encryptionKey string, mailer mail.Mailer, publicURL string) *AuthHandler {
	sealer, err := secrets.NewSealer(encryptionKey)
	if err != nil {
		log.Fatalf("failed to init auth secret sealer: %v", err)
	}
	return &AuthHandler{q: q, secret: secret, sealer: sealer, mailer: mailer, publicURL: publicURL, handoff: newOAuthHandoffStore()}
}

type authResponse struct {
	AccessToken string `json:"access_token"`
	// Omitted in cookie mode: there the token goes into an httpOnly cookie and
	// must never reach the page's JavaScript. See handlers/auth_cookie.go.
	RefreshToken string   `json:"refresh_token,omitempty"`
	User         userDTO  `json:"user"`
	Preferences  prefsDTO `json:"preferences"`
	// What became of an invitation token presented at sign-up (see
	// acceptInvitationToken). Absent everywhere else — only Register takes one.
	Invite string `json:"invite,omitempty"`
}

// rejectBadPassword answers 400 when a new password cannot be stored, and
// reports whether it did.
//
// The upper bound is bcrypt's and it is counted in bytes, which no binding tag
// can express — `max=72` counts runes, so a 37-letter Russian passphrase (74
// bytes) would sail past it and fail inside GenerateFromPassword. That failure
// used to reach the 500 branch of all three doors: the user read "internal
// error" about a password that was merely long, and the operator got a logged
// server error that was nothing of the kind (#2932).
//
// The strings are fixed English sentinels because that is how this API hands
// reasons to a client — see SENTINELS in frontend/src/utils/errors.js, which
// turns them into the reader's language.
func rejectBadPassword(c *gin.Context, plain string) bool {
	switch err := auth.CheckPasswordPolicy(plain); {
	case errors.Is(err, auth.ErrPasswordTooLong):
		c.JSON(http.StatusBadRequest, gin.H{"error": "password is too long"})
		return true
	case errors.Is(err, auth.ErrPasswordTooShort):
		c.JSON(http.StatusBadRequest, gin.H{"error": "password is too short"})
		return true
	}
	return false
}

// Register creates a user. The very first registered user becomes an admin.
func (h *AuthHandler) Register(c *gin.Context) {
	var req struct {
		Email    string `json:"email" binding:"required,email"`
		Name     string `json:"name" binding:"required"`
		Password string `json:"password" binding:"required,min=8"`
		// Optional: the token from an invitation email, carried here by the
		// invite page when the invitee has no account yet.
		InviteToken string `json:"invite_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if rejectBadPassword(c, req.Password) {
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
		Name: personalWorkspaceName, NameKey: ptr(personalWorkspaceKey), OwnerID: user.ID,
	}); err == nil {
		_, _ = h.q.CreateMembership(c, db.CreateMembershipParams{
			WorkspaceID: ws.ID, UserID: user.ID, Role: "owner",
		})
	}

	// Only the invitation whose token was presented is applied: the address by
	// itself proves nothing, and invitations sit pending for a week, so joining
	// on a bare email match would hand the workspace to whoever registers the
	// invitee's address first. The rest wait for email verification.
	invite := h.acceptInvitationToken(c, user, req.InviteToken)
	h.sendVerification(c, user)

	// The outcome rides back with the session: the account exists either way, and
	// a client that presented a token has no other way to learn whether it took.
	h.issueMode(c, user, wantsCookieAuth(c), invite)
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
		// A database blip is not a verdict on the session. Answering 401 here
		// would clear the cookie and destroy a perfectly valid login over a
		// momentary storage failure, with no way back except a fresh sign-in.
		if !errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "storage unavailable"})
			return
		}
		reject(http.StatusUnauthorized, "invalid refresh token")
		return
	}
	usable, replay := refreshTokenUsable(rt, time.Now())
	if !usable {
		reject(http.StatusUnauthorized, "refresh token expired or revoked")
		return
	}

	// On a replay the row is already revoked — re-revoking it would only push the
	// grace window forward on every retry, turning a one-off answer to a lost
	// response into an indefinitely reusable token.
	if !replay {
		if err := h.q.RevokeRefreshToken(c, hash); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}
	} else {
		log.Printf("auth: refresh token replayed within grace window (user %s)", rt.UserID)
	}
	user, err := h.q.GetUserByID(c, rt.UserID)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "storage unavailable"})
			return
		}
		reject(http.StatusUnauthorized, "user not found")
		return
	}
	// Deactivation has to reach the session, not just the login form. A refresh
	// token renews itself for as long as it is used, so an account switched off
	// while someone was signed in would otherwise never actually leave — rotation
	// alone would carry it indefinitely. 403 matches Login's answer for the same
	// state, and every client already reads a non-5xx refusal here as "the
	// session is over" (frontend/src/api/index.js refreshFailure).
	if !user.Active {
		reject(http.StatusForbidden, "account is deactivated")
		return
	}

	// Rotation stays in the channel it arrived on: a cookie-mode client gets the
	// new token back as a cookie even if it forgot the header.
	h.issueMode(c, user, fromCookie || wantsCookieAuth(c), "")
}

// refreshGrace is how long a just-rotated refresh token keeps working. Rotation
// cannot be atomic with delivery: the server revokes the presented token and
// only then writes the new pair, so a connection dropped in between leaves the
// client holding a token the server has already killed — the user is signed out
// by a lost response, not by anything wrong with their session. Within this
// window the old token answers once more with a fresh pair.
//
// The window is deliberately short: it is sized for a retry of an interrupted
// request, not for a stolen token to be useful.
const refreshGrace = 30 * time.Second

// refreshTokenUsable decides whether a stored refresh token may still be traded
// for a new pair. `replay` marks the grace case — the token was revoked, but so
// recently that this is a retry of a rotation whose response never landed.
func refreshTokenUsable(rt db.RefreshToken, now time.Time) (usable, replay bool) {
	// Expiry is absolute: the grace window never resurrects a token past its TTL.
	if now.After(rt.ExpiresAt) {
		return false, false
	}
	if rt.RevokedAt == nil {
		return true, false
	}
	if now.Sub(*rt.RevokedAt) <= refreshGrace {
		return true, true
	}
	return false, false
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
	// Unconditionally: the caller asked to be logged out, so the cookies go
	// even if the token turns out to be unknown.
	h.clearRefreshCookie(c)
	h.clearMediaCookie(c)

	if token != "" {
		// Hard revoke, not the rotation one: an explicit sign-out must not leave
		// the token usable for the length of the grace window.
		if err := h.q.ExpireRefreshToken(c, auth.HashRefreshToken(token)); err != nil {
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
	h.issueMode(c, user, wantsCookieAuth(c), "")
}

// issueMode is issue() with the delivery mode decided by the caller — Refresh
// needs it, because a token that arrived in a cookie goes back in a cookie
// regardless of headers — and with the fate of a presented invitation token,
// which only Register has one of.
func (h *AuthHandler) issueMode(c *gin.Context, user db.User, cookieMode bool, invite string) {
	access, refresh, ok := h.mintTokens(c, user)
	if !ok {
		return
	}
	res := authResponse{
		AccessToken:  access,
		RefreshToken: refresh,
		User:         buildUserDTO(c, h.q, user),
		Preferences:  loadPrefsDTO(c, h.q, user.ID),
		Invite:       invite,
	}
	if cookieMode {
		h.setRefreshCookie(c, refresh)
		res.RefreshToken = "" // omitempty drops it from the body entirely
		// Same client, same trip: the browser also needs the credential that
		// <img> requests to /api/uploads can carry (#2685). Reissued on every
		// refresh, so a long-lived session never outlives its media cookie.
		h.setMediaCookie(c, user.ID)
	}
	c.JSON(http.StatusOK, res)
}
