package handlers

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/auth"
	"tessera/middleware"
)

// Refresh-token delivery has two modes, picked by the client:
//
//   - body (default) — the token is returned in the JSON response. Android
//     (Retrofit) and the desktop app talk to the API cross-origin, where a
//     host-only cookie would never be sent, so they keep using this.
//   - cookie — the token is set as an httpOnly cookie and left OUT of the
//     response body entirely. The web SPA is served from the same origin as the
//     API, so the cookie rides along on /api/auth calls while staying invisible
//     to any script injected into the page.
//
// The mode is requested explicitly via the X-Auth-Mode header rather than
// sniffed from Origin: it makes the contract testable and doesn't silently
// change behaviour if the SPA is ever hosted somewhere else.
const (
	refreshCookieName = "tessera_refresh"
	// Scoped to the auth routes: the cookie is only ever needed by refresh,
	// logout and the OAuth callback, so it isn't attached to every API call.
	refreshCookiePath = "/api/auth"
	authModeHeader    = "X-Auth-Mode"
	authModeCookie    = "cookie"

	// The media cookie is the browser's credential for inline images (#2685):
	// an <img> can't carry a bearer header, so without it /api/uploads/… has to
	// stay open to anyone who has the URL. Scoped to the media routes and
	// carrying an audience-tagged token, it grants nothing else. The name lives
	// in middleware/, which is the side that reads it back.
	mediaCookieName = middleware.MediaCookieName
	mediaCookiePath = "/api/uploads"
)

// wantsCookieAuth reports whether the caller asked for cookie delivery.
func wantsCookieAuth(c *gin.Context) bool {
	return strings.EqualFold(c.GetHeader(authModeHeader), authModeCookie)
}

// cookieSecure decides the Secure attribute. Behind nginx the request itself is
// plain http (c.Request.TLS is nil) even though the browser spoke https, so the
// public base URL is the only reliable signal — the same one the OAuth flow uses
// for its state cookie. Guessing wrong the other way (Secure on a plain-http dev
// server) would make the browser drop the cookie without a word.
func (h *AuthHandler) cookieSecure(c *gin.Context) bool {
	return strings.HasPrefix(h.oauthBaseURL(c), "https")
}

// setRefreshCookie stores the refresh token where JavaScript can't reach it.
// SameSite=Strict: the cookie is only ever used by same-site XHR from the SPA,
// so refusing to send it on any cross-site request costs nothing and removes the
// CSRF surface a cookie session would otherwise add.
func (h *AuthHandler) setRefreshCookie(c *gin.Context, token string) {
	// gin applies the SameSite mode set on the context to the NEXT SetCookie
	// call — set it first or the attribute is silently dropped.
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(refreshCookieName, token, int(auth.RefreshTokenTTL.Seconds()),
		refreshCookiePath, "", h.cookieSecure(c), true)
}

// clearRefreshCookie expires the cookie. Attributes must match the ones it was
// set with (path especially), or the browser keeps the original cookie alongside
// the expired one.
func (h *AuthHandler) clearRefreshCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(refreshCookieName, "", -1, refreshCookiePath, "", h.cookieSecure(c), true)
}

// setMediaCookie hands the browser its image credential, minted for the user the
// session belongs to. Failure is not fatal: media serving falls back to whatever
// the install's MEDIA_REQUIRE_AUTH says, and refusing the login instead would
// trade a hardening measure for an outage.
//
// SameSite=Lax rather than Strict: the images this cookie unlocks are also
// opened as plain links (a notification mail, a chat pasting the URL), and Strict
// would blank those out. Nothing under /api/uploads changes state — the upload
// route on the same path is POST and still demands a bearer token — so the
// laxer tier buys the working case without opening a CSRF surface.
func (h *AuthHandler) setMediaCookie(c *gin.Context, userID uuid.UUID) {
	tok, err := auth.NewMediaToken(h.secret, userID)
	if err != nil {
		return
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(mediaCookieName, tok, int(auth.MediaTokenTTL.Seconds()),
		mediaCookiePath, "", h.cookieSecure(c), true)
}

// clearMediaCookie expires the image credential; attributes mirror the ones it
// was set with, or the browser keeps the live cookie next to the expired one.
func (h *AuthHandler) clearMediaCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(mediaCookieName, "", -1, mediaCookiePath, "", h.cookieSecure(c), true)
}

// refreshTokenFromRequest picks the presented refresh token. The cookie wins over
// the body: a client that has one is in cookie mode, and a stale body value (an
// old tab, a replayed script) must not be able to steer the session.
func refreshTokenFromRequest(c *gin.Context, body string) (token string, fromCookie bool) {
	if ck, err := c.Cookie(refreshCookieName); err == nil && ck != "" {
		return ck, true
	}
	return body, false
}
