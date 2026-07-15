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

// oauthMobileScheme is the custom-scheme deep link the Android app registers. When the
// flow is initiated from the mobile client (?platform=android), the callback hands the
// session (or error) back through this URI instead of the web SPA route — Chrome Custom
// Tab redirects to it, MainActivity picks up the fragment. The confidential-app secret
// never leaves the server, so the mobile client stays a pure public consumer of tokens.
const oauthMobileScheme = "tessera://oauth/callback"

// oauthMobileState marks a `state` value as originating from the mobile client. The
// marker rides through GitLab and comes back in the callback query, so the callback
// knows which handoff target to use. It is inside the CSRF-checked state (mirrored in
// the cookie), so it can't be forged independently of the state itself.
const oauthMobileState = "m."

// oauthBaseURL is the externally-reachable origin used to build redirect URIs and
// the post-login handoff. Prefers the configured PublicURL; falls back to the
// request's scheme+host (dev).
func (h *AuthHandler) oauthBaseURL(c *gin.Context) string {
	if h.publicURL != "" {
		return strings.TrimRight(h.publicURL, "/")
	}
	scheme := "http"
	if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
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
	mobile := c.Query("platform") == "android"
	p, err := h.q.GetOAuthProvider(c, "gitlab")
	if err != nil || !p.Enabled || p.ClientID == "" || p.GlBaseUrl == "" {
		if mobile {
			c.Redirect(http.StatusFound, oauthMobileScheme+"#"+url.Values{"oauth_error": {"not_configured"}}.Encode())
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
	state := hex.EncodeToString(buf)
	if mobile {
		state = oauthMobileState + state
	}
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
	// The mobile marker lives inside the state value, which is echoed back untouched by
	// GitLab — safe to read here for choosing the handoff target even before the CSRF
	// check (a forged prefix only changes where an *error* is delivered).
	mobile := strings.HasPrefix(c.Query("state"), oauthMobileState)
	redirectErr := func(reason string) {
		if mobile {
			c.Redirect(http.StatusFound, oauthMobileScheme+"#"+url.Values{"oauth_error": {reason}}.Encode())
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
	frag := url.Values{"access_token": {access}, "refresh_token": {refresh}}
	if mobile {
		c.Redirect(http.StatusFound, oauthMobileScheme+"#"+frag.Encode())
		return
	}
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
		Name: "Личное пространство", OwnerID: user.ID,
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
