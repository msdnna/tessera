package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/gitlab"
)

const oauthStateCookie = "gl_oauth_state"

// oauthNativeScheme is the custom-scheme deep link the native clients register. When
// the flow is initiated from one of them (?platform=android|desktop), the callback hands
// the session (or error) back through this URI instead of the web SPA route — the Chrome
// Custom Tab / system browser redirects to it, and the app picks up the fragment. The
// confidential-app secret never leaves the server, so native clients stay pure public
// consumers of tokens. Android and the desktop shell share one scheme: they never run on
// the same OS, and a single handoff format keeps the callback branch-free.
const oauthNativeScheme = "tessera://oauth/callback"

// oauthMobileState / oauthDesktopState mark a `state` value as originating from a native
// client. The marker rides through GitLab and comes back in the callback query, so the
// callback knows which handoff target to use. It is inside the CSRF-checked state
// (mirrored in the cookie), so it can't be forged independently of the state itself. The
// two platforms share a handoff URI but keep distinct markers so they can diverge later.
const (
	oauthMobileState  = "m."
	oauthDesktopState = "d."
)

// oauthHandoff decides how a flow hands the session back. On authorize, `platform` is the
// query parameter and `state` is empty; on callback it is the other way round — the marker
// travels inside the state. Native clients get the deep link and carry the refresh token
// themselves; the web SPA gets a same-origin redirect plus the httpOnly refresh cookie.
func oauthHandoff(platform, state string) (native bool, prefix string) {
	switch platform {
	case "android":
		return true, oauthMobileState
	case "desktop":
		return true, oauthDesktopState
	}
	for _, p := range []string{oauthMobileState, oauthDesktopState} {
		if strings.HasPrefix(state, p) {
			return true, p
		}
	}
	return false, ""
}

// oauthAppStateMax bounds the client nonce echoed back in the handoff. Long enough for
// 32 hex chars with room to spare, short enough that nobody can stuff a payload into the
// deep link through it.
const oauthAppStateMax = 64

// oauthAppState sanitizes the native client's own CSRF nonce (`?app_state=`). The state
// GitLab echoes back proves the *browser* leg wasn't forged — it is mirrored in an
// httpOnly cookie — but the last hop is a plain OS-wide URL: any web page can fire a
// tessera:// link at the installed app. Without a value only the app knows, an
// unsolicited `tessera://oauth/callback#access_token=…` silently signs the user into the
// attacker's account (session fixation, #2932). So the app generates a nonce, sends it
// here, and accepts the handoff only when it comes back.
//
// Alphanumeric only: the nonce is stored inside the state as its own dot-separated field,
// and it must not be able to forge that structure or smuggle URL syntax into the
// redirect. Anything else is dropped rather than mangled — a client that sent garbage
// gets a handoff it will reject, which is the safe direction.
func oauthAppState(raw string) string {
	if raw == "" || len(raw) > oauthAppStateMax {
		return ""
	}
	for _, r := range raw {
		if (r < '0' || r > '9') && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return ""
		}
	}
	return raw
}

// oauthState builds the value that travels to GitLab and back. Paired with
// oauthStateNonce below — the two have to agree on the layout or every real login is
// rejected, so they are kept next to each other and round-tripped in the tests.
func oauthState(prefix, nonce, random string) string {
	if nonce == "" {
		return prefix + random
	}
	return prefix + nonce + "." + random
}

// oauthStateNonce pulls the client nonce back out of the state GitLab echoed. Layout is
// `<marker><nonce>.<random>` when the client sent one and `<marker><random>` when it
// didn't (Android, and desktop builds older than #2932) — the random half is hex, so the
// separator is unambiguous.
func oauthStateNonce(prefix, state string) string {
	rest := strings.TrimPrefix(state, prefix)
	i := strings.Index(rest, ".")
	if i <= 0 {
		return ""
	}
	return oauthAppState(rest[:i])
}

// oauthBaseURL is the externally-reachable origin used to build redirect URIs and
// the post-login handoff. Prefers the configured PublicURL; falls back to the
// request's scheme+host (dev).
func (h *AuthHandler) oauthBaseURL(c *gin.Context) string {
	if h.publicURL != "" {
		return strings.TrimRight(h.publicURL, "/")
	}
	scheme := "http"
	// A proxy chain appends rather than replaces, so the header can arrive as
	// "https, http" (outer edge first) — the client-facing scheme is the first
	// token, not the whole string an equality check would miss.
	xfProto := strings.TrimSpace(strings.SplitN(c.GetHeader("X-Forwarded-Proto"), ",", 2)[0])
	if c.Request.TLS != nil || strings.EqualFold(xfProto, "https") {
		scheme = "https"
	}
	// Behind a reverse proxy, the Host header may be rewritten without the public
	// port (nginx `proxy_set_header Host $host` drops it), which would generate a
	// redirect_uri on the wrong port. Prefer X-Forwarded-Host (carries host:port)
	// when the proxy sets it; fall back to the request Host. Setting PUBLIC_URL
	// always wins and is the recommended production config.
	host := c.Request.Host
	if xfh := c.GetHeader("X-Forwarded-Host"); xfh != "" {
		host = xfh
	}
	return scheme + "://" + host
}

func (h *AuthHandler) gitlabRedirectURI(c *gin.Context) string {
	return h.oauthBaseURL(c) + "/api/auth/gitlab/callback"
}

// Providers reports which external login providers are configured+enabled, so the
// login page can show/hide the "Continue with GitLab" button. Public.
func (h *AuthHandler) Providers(c *gin.Context) {
	out := gin.H{"gitlab": false}
	if p, err := h.q.GetOAuthProvider(c, "gitlab"); err == nil {
		out["gitlab"] = p.Enabled && p.ClientID != "" && p.GlBaseUrl != ""
	}
	c.JSON(http.StatusOK, out)
}

// GitlabAuthorize redirects the browser to GitLab's authorization endpoint with a
// CSRF `state` mirrored in a short-lived cookie.
func (h *AuthHandler) GitlabAuthorize(c *gin.Context) {
	native, statePrefix := oauthHandoff(c.Query("platform"), "")
	p, err := h.q.GetOAuthProvider(c, "gitlab")
	if err != nil || !p.Enabled || p.ClientID == "" || p.GlBaseUrl == "" {
		if native {
			c.Redirect(http.StatusFound, oauthNativeScheme+"#"+url.Values{"oauth_error": {"not_configured"}}.Encode())
			return
		}
		c.Redirect(http.StatusFound, h.oauthBaseURL(c)+"/login?oauth_error=not_configured")
		return
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	// The client nonce rides *inside* the CSRF state, so it inherits the cookie check and
	// can't be swapped for another flow's value on the way back. Only native flows have a
	// deep link to protect; the web handoff is a same-origin redirect.
	nonce := ""
	if native {
		nonce = oauthAppState(c.Query("app_state"))
	}
	state := oauthState(statePrefix, nonce, hex.EncodeToString(buf))
	secure := strings.HasPrefix(h.oauthBaseURL(c), "https")
	// SameSite=Lax so the cookie survives GitLab's top-level GET redirect back.
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(oauthStateCookie, state, 600, "/api/auth/gitlab", "", secure, true)

	q := url.Values{
		"client_id":     {p.ClientID},
		"redirect_uri":  {h.gitlabRedirectURI(c)},
		"response_type": {"code"},
		"state":         {state},
		"scope":         {"read_api"}, // read_api covers /user AND /groups (for org_map)
	}
	c.Redirect(http.StatusFound, strings.TrimRight(p.GlBaseUrl, "/")+"/oauth/authorize?"+q.Encode())
}

// GitlabCallback completes the flow: verify state, exchange the code, fetch the
// GitLab user, provision/link the Tessera account, apply the org_map, then hand the
// session back to the SPA via the URL fragment (never logged/sent to the server).
func (h *AuthHandler) GitlabCallback(c *gin.Context) {
	base := h.oauthBaseURL(c)
	// The native marker lives inside the state value, which is echoed back untouched by
	// GitLab — safe to read here for choosing the handoff target even before the CSRF
	// check (a forged prefix only changes where an *error* is delivered).
	native, statePrefix := oauthHandoff("", c.Query("state"))
	// Echoed back to the app so it can tell *its* handoff from one a web page fired at
	// the tessera:// scheme. Read before the CSRF check for the same reason the marker is:
	// a forged value only changes where an error lands, and the app rejects it anyway.
	nonce := oauthStateNonce(statePrefix, c.Query("state"))
	redirectErr := func(reason string) {
		if native {
			v := url.Values{"oauth_error": {reason}}
			if nonce != "" {
				v.Set("state", nonce)
			}
			c.Redirect(http.StatusFound, oauthNativeScheme+"#"+v.Encode())
			return
		}
		c.Redirect(http.StatusFound, base+"/login?oauth_error="+url.QueryEscape(reason))
	}

	// CSRF: query state must match the cookie; clear the cookie either way.
	cookieState, _ := c.Cookie(oauthStateCookie)
	secure := strings.HasPrefix(base, "https")
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(oauthStateCookie, "", -1, "/api/auth/gitlab", "", secure, true)
	if s := c.Query("state"); s == "" || cookieState == "" || s != cookieState {
		redirectErr("state_mismatch")
		return
	}
	code := c.Query("code")
	if code == "" {
		redirectErr(c.DefaultQuery("error", "no_code"))
		return
	}

	p, err := h.q.GetOAuthProvider(c, "gitlab")
	if err != nil || !p.Enabled {
		redirectErr("not_configured")
		return
	}
	secret, err := h.sealer.Decrypt(p.ClientSecretEnc)
	if err != nil {
		redirectErr("server_error")
		return
	}

	token, err := gitlab.ExchangeOAuthCode(c, p.GlBaseUrl, p.ClientID, secret, code, h.gitlabRedirectURI(c))
	if err != nil {
		redirectErr("exchange_failed")
		return
	}
	glUser, err := gitlab.OAuthUser(c, p.GlBaseUrl, token)
	if err != nil || glUser.ID == 0 {
		redirectErr("userinfo_failed")
		return
	}

	user, err := h.provisionOAuthUser(c, p, glUser)
	if err != nil {
		redirectErr(err.Error())
		return
	}
	// Group-membership → workspace access, refreshed on every login.
	h.applyOrgMap(c, p, token, glUser, user)

	access, refresh, ok := h.mintTokens(c, user)
	if !ok {
		return // mintTokens already wrote a 500
	}
	if native {
		// No cookie to set on a custom-scheme handoff — the native client stores
		// the refresh token itself. Desktop is cross-origin against the API
		// (frontend/src/utils/serverBase.js), so a host-only cookie would never
		// reach it either way.
		frag := url.Values{"access_token": {access}, "refresh_token": {refresh}}
		if nonce != "" {
			frag.Set("state", nonce)
		}
		c.Redirect(http.StatusFound, oauthNativeScheme+"#"+frag.Encode())
		return
	}
	// Web: the refresh token goes into the httpOnly cookie instead of the
	// fragment. A fragment never reaches the server, but it does stay in browser
	// history and in window.location, where injected script can read it — which
	// would defeat the whole point of hiding the token from JavaScript.
	h.setRefreshCookie(c, refresh)
	h.setMediaCookie(c, user.ID)
	frag := url.Values{"access_token": {access}}
	c.Redirect(http.StatusFound, base+"/oauth/callback#"+frag.Encode())
}

// provisionOAuthUser finds or creates the Tessera account for a GitLab identity:
//  1. known identity → that user;
//  2. same email as an existing local user → link (GitLab vouches for the email);
//  3. otherwise auto-register (provider='gitlab', no password, email pre-verified).
func (h *AuthHandler) provisionOAuthUser(c *gin.Context, p db.OauthProvider, glUser gitlab.OAuthUserInfo) (db.User, error) {
	providerUserID := strconv.FormatInt(glUser.ID, 10)

	if id, err := h.q.GetOAuthIdentity(c, db.GetOAuthIdentityParams{Provider: "gitlab", ProviderUserID: providerUserID}); err == nil {
		user, uerr := h.q.GetUserByID(c, id.UserID)
		if uerr != nil {
			return db.User{}, errors.New("account_missing")
		}
		if !user.Active {
			return db.User{}, errors.New("account_disabled")
		}
		h.upsertIdentity(c, user.ID, providerUserID, glUser, p.GlBaseUrl)
		return user, nil
	}

	email := strings.ToLower(strings.TrimSpace(glUser.Email))
	if email != "" {
		if user, err := h.q.GetUserByEmail(c, email); err == nil {
			if !user.Active {
				return db.User{}, errors.New("account_disabled")
			}
			h.upsertIdentity(c, user.ID, providerUserID, glUser, p.GlBaseUrl)
			return user, nil
		}
	}

	// Auto-register. GitLab may hide the email (scope/privacy) — synthesise a stable
	// placeholder so the NOT NULL/unique email holds; the user can set a real one later.
	count, _ := h.q.CountUsers(c)
	name := glUser.Name
	if name == "" {
		name = glUser.Username
	}
	if email == "" {
		email = strings.ToLower(glUser.Username) + "@gitlab.local"
	}
	user, err := h.q.CreateOAuthUser(c, db.CreateOAuthUserParams{
		Email: email, Name: name, IsAdmin: count == 0, Provider: "gitlab",
	})
	if err != nil {
		return db.User{}, errors.New("create_failed")
	}
	if ws, werr := h.q.CreateWorkspace(c, db.CreateWorkspaceParams{
		Name: personalWorkspaceName, NameKey: ptr(personalWorkspaceKey), OwnerID: user.ID,
	}); werr == nil {
		_, _ = h.q.CreateMembership(c, db.CreateMembershipParams{
			WorkspaceID: ws.ID, UserID: user.ID, Role: "owner",
		})
	}
	h.acceptPendingInvitations(c, user)
	h.upsertIdentity(c, user.ID, providerUserID, glUser, p.GlBaseUrl)
	return user, nil
}

func (h *AuthHandler) upsertIdentity(c *gin.Context, userID uuid.UUID, providerUserID string, glUser gitlab.OAuthUserInfo, baseURL string) {
	_, _ = h.q.UpsertOAuthIdentity(c, db.UpsertOAuthIdentityParams{
		UserID:           userID,
		Provider:         "gitlab",
		ProviderUserID:   providerUserID,
		ProviderUsername: glUser.Username,
		ProviderEmail:    strings.ToLower(strings.TrimSpace(glUser.Email)),
		GlBaseUrl:        strings.TrimRight(baseURL, "/"),
	})
}

// orgMapEntry is one AWX-style mapping: a GitLab group full-path → a Tessera
// workspace with an explicit admin list and an all-members flag.
type orgMapEntry struct {
	WorkspaceID string   `json:"workspace_id"`
	Admins      []string `json:"admins"`
	Users       bool     `json:"users"`
}

// applyOrgMap grants workspace membership from the user's GitLab group membership.
// Additive and idempotent; never downgrades a workspace owner. Best-effort — a
// GitLab/groups failure must not block login.
func (h *AuthHandler) applyOrgMap(c *gin.Context, p db.OauthProvider, token string, glUser gitlab.OAuthUserInfo, user db.User) {
	raw := strings.TrimSpace(string(p.OrgMap))
	if raw == "" || raw == "{}" || raw == "null" {
		return
	}
	var m map[string]orgMapEntry
	if err := json.Unmarshal(p.OrgMap, &m); err != nil || len(m) == 0 {
		return
	}
	groups, err := gitlab.OAuthUserGroupPaths(c, p.GlBaseUrl, token)
	if err != nil {
		return
	}
	for groupPath, cfg := range m {
		if !groups[groupPath] {
			continue
		}
		wsID, perr := uuid.Parse(cfg.WorkspaceID)
		if perr != nil {
			continue
		}
		role := ""
		for _, a := range cfg.Admins {
			if strings.EqualFold(strings.TrimSpace(a), glUser.Username) {
				role = "admin"
				break
			}
		}
		if role == "" && cfg.Users {
			role = "member"
		}
		if role == "" {
			continue
		}
		if existing, gerr := h.q.GetMembershipRole(c, db.GetMembershipRoleParams{WorkspaceID: wsID, UserID: user.ID}); gerr == nil && existing == "owner" {
			continue // never downgrade an owner
		}
		_, _ = h.q.CreateMembership(c, db.CreateMembershipParams{WorkspaceID: wsID, UserID: user.ID, Role: role})
	}
}
