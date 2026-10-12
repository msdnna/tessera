package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"tessera/internal/db"
	"tessera/internal/gitlab"
	"tessera/middleware"
)

// GitLab connections v2 (#3065, migration 0097): several projects on one board,
// an OAuth connection instead of a PAT, the token's expiry and a paused sync. All
// of it stays on the GitLab side of the seam — gitlab_credentials and
// gitlab_integrations only.

// ── Several projects on one board (#3082) ──────────────────

// errChooseIntegration answers a task action that has to pick one binding of a
// board several GitLab projects mirror into, when the caller named none.
const errChooseIntegration = "choose_integration"

// bindingChoice is one binding a caller may pick in that answer.
type bindingChoice struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	ProjectPath string    `json:"project_path"`
}

// boardIntegration resolves the binding a task-scoped GitLab action runs under.
// A linked task belongs to its link's binding — a card belongs to exactly one —
// so that wins. Otherwise the board's bindings: the one the caller named (it must
// be among them), the only one, or — when there are several and none was named —
// a 409 listing them, so the client can ask which project. Writes the error
// response and returns ok=false when it can't resolve.
func (h *API) boardIntegration(c *gin.Context, taskID, boardID uuid.UUID, want *uuid.UUID) (db.GitlabIntegration, bool) {
	if link, err := h.q.GetGitlabLinkByTask(c, taskID); err == nil {
		integ, ierr := h.q.GetGitlabIntegrationByID(c, link.IntegrationID)
		if ierr == nil {
			return integ, true
		}
	}
	all, err := h.q.ListGitlabIntegrationsByBoard(c, boardID)
	if err != nil {
		fail(c, err)
		return db.GitlabIntegration{}, false
	}
	if len(all) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task is not on a GitLab integration board"})
		return db.GitlabIntegration{}, false
	}
	if want != nil {
		for _, integ := range all {
			if integ.ID == *want {
				return integ, true
			}
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "that GitLab binding does not mirror into this board"})
		return db.GitlabIntegration{}, false
	}
	if len(all) == 1 {
		return all[0], true
	}
	choices := make([]bindingChoice, 0, len(all))
	for _, integ := range all {
		choices = append(choices, bindingChoice{ID: integ.ID, Name: integ.Name, ProjectPath: integ.ProjectPath})
	}
	c.JSON(http.StatusConflict, gin.H{
		"error":        "several GitLab projects are bound to this board — choose one",
		"code":         errChooseIntegration,
		"integrations": choices,
	})
	return db.GitlabIntegration{}, false
}

// ── Paused sync (#3084) ─────────────────────────────────────

// pauseReasonUnauthorized: GitLab refused the credential (expired, revoked).
const pauseReasonUnauthorized = "unauthorized"

// noteSyncOutcome keeps the binding's pause in step with how its last sync went:
// a refused credential parks it (the unattended sync stops knocking every minute
// with a dead token), any sync that got through lifts the pause.
func (h *API) noteSyncOutcome(ctx context.Context, integ db.GitlabIntegration, err error) {
	ctx = context.WithoutCancel(ctx)
	switch {
	case err == nil:
		if integ.PausedAt != nil {
			soft(ctx, "ResumeGitlabIntegration", h.q.ResumeGitlabIntegration(ctx, integ.ID))
		}
	case gitlab.IsUnauthorized(err):
		soft(ctx, "PauseGitlabIntegration", h.q.PauseGitlabIntegration(ctx, db.PauseGitlabIntegrationParams{
			ID: integ.ID, PauseReason: pauseReasonUnauthorized,
		}))
		if integ.PausedAt == nil {
			h.broadcast(integ.WorkspaceID, "integration.paused", gin.H{
				"provider": "gitlab", "integration_id": integ.ID, "reason": pauseReasonUnauthorized,
			})
		}
	}
}

// resumeAfterReconnect lifts the pauses a fresh credential answers for: the
// bindings the user owns, or — for the instance service token — all of them.
func (h *API) resumeAfterReconnect(ctx context.Context, owner *uuid.UUID) {
	var err error
	if owner != nil {
		_, err = h.q.ResumeGitlabIntegrationsForOwner(ctx, owner)
	} else {
		_, err = h.q.ResumeAllGitlabIntegrations(ctx)
	}
	soft(ctx, "ResumeGitlabIntegrations", err)
}

// tokenExpiryDate reads a PAT's expiry from GitLab, best-effort: an instance too
// old for /personal_access_tokens/self, or any hiccup, reads as "unknown" (nil).
func tokenExpiryDate(ctx context.Context, client *gitlab.Client) *time.Time {
	exp, err := client.TokenExpiry(ctx)
	if err != nil {
		return nil
	}
	return exp
}

// ── OAuth connection (#3083) ────────────────────────────────

// oauthConnectState marks the `state` of a connect flow. It shares the login's
// callback (/api/auth/gitlab/callback — the redirect URI the GitLab application
// already has registered, so connecting needs no second one), and the marker is
// how the callback tells the two apart.
const oauthConnectState = "c."

// oauthConnectTTL bounds how long a started connect flow stays valid.
const oauthConnectTTL = 10 * time.Minute

// connectClaims is what a connect state carries: who started it, until when, and
// where to return. Signed with the server secret, so the callback — a public
// route with no session — knows whose account the grant belongs to.
type connectClaims struct {
	User     uuid.UUID `json:"u"`
	Exp      int64     `json:"e"`
	ReturnTo string    `json:"r"`
	Nonce    string    `json:"n"`
}

func signConnectState(secret string, cl connectClaims) (string, error) {
	raw, err := json.Marshal(cl)
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(raw)
	mac := hmac.New(sha256.New, []byte("gitlab-connect:"+secret))
	mac.Write([]byte(body))
	return oauthConnectState + body + "." + hex.EncodeToString(mac.Sum(nil)), nil
}

func verifyConnectState(secret, state string, now time.Time) (connectClaims, bool) {
	rest, ok := strings.CutPrefix(state, oauthConnectState)
	if !ok {
		return connectClaims{}, false
	}
	body, sig, ok := strings.Cut(rest, ".")
	if !ok {
		return connectClaims{}, false
	}
	mac := hmac.New(sha256.New, []byte("gitlab-connect:"+secret))
	mac.Write([]byte(body))
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(sig), []byte(want)) {
		return connectClaims{}, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return connectClaims{}, false
	}
	var cl connectClaims
	if json.Unmarshal(raw, &cl) != nil || cl.User == uuid.Nil || now.Unix() > cl.Exp {
		return connectClaims{}, false
	}
	return cl, true
}

// safeReturnTo keeps a return path on this origin: an absolute path, nothing that
// could become another host ("//evil", "/\evil").
func safeReturnTo(p string) string {
	p = strings.TrimSpace(p)
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.HasPrefix(p, "/\\") || strings.ContainsAny(p, "\r\n") {
		return "/"
	}
	return p
}

// withQuery appends one query parameter to a same-origin path.
func withQuery(p, key, value string) string {
	u, err := url.Parse(p)
	if err != nil {
		return "/?" + url.Values{key: {value}}.Encode()
	}
	q := u.Query()
	q.Set(key, value)
	u.RawQuery = q.Encode()
	return u.String()
}

// GitlabConnectStart begins connecting the caller's GitLab account through OAuth
// (#3083): it answers the GitLab authorize URL (scope `api` — the connection
// writes back) and mirrors the signed state in the same short cookie the login
// uses. The SPA then navigates there; a fetch cannot follow a cross-origin
// redirect into GitLab's consent page. POST /gitlab/connection/oauth.
func (h *AuthHandler) GitlabConnectStart(c *gin.Context) {
	var req struct {
		ReturnTo string `json:"return_to"`
	}
	_ = c.ShouldBindJSON(&req)
	p, err := h.q.GetOAuthProvider(c, "gitlab")
	if err != nil || !p.Enabled || p.ClientID == "" || p.GlBaseUrl == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "GitLab OAuth is not configured — ask an admin, or connect with a token", "code": "oauth_not_configured"})
		return
	}
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	state, err := signConnectState(h.secret, connectClaims{
		User: middleware.CurrentUser(c), Exp: time.Now().Add(oauthConnectTTL).Unix(),
		ReturnTo: safeReturnTo(req.ReturnTo), Nonce: hex.EncodeToString(buf),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
		return
	}
	secure := strings.HasPrefix(h.oauthBaseURL(c), "https")
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(oauthStateCookie, state, int(oauthConnectTTL.Seconds()), "/api/auth/gitlab", "", secure, true)
	q := url.Values{
		"client_id":     {p.ClientID},
		"redirect_uri":  {h.gitlabRedirectURI(c)},
		"response_type": {"code"},
		"state":         {state},
		"scope":         {"api"},
	}
	c.JSON(http.StatusOK, gin.H{"url": strings.TrimRight(p.GlBaseUrl, "/") + "/oauth/authorize?" + q.Encode()})
}

// gitlabConnectCallback finishes a connect flow on the shared callback: check the
// state (cookie mirror + signature + expiry), exchange the code, learn whose
// GitLab account it is, and store the grant encrypted like a PAT. Answers by
// redirecting back where the flow started, with ?gitlab_oauth=connected or the
// reason it failed.
func (h *AuthHandler) gitlabConnectCallback(c *gin.Context) {
	base := h.oauthBaseURL(c)
	state := c.Query("state")
	cookieState, _ := c.Cookie(oauthStateCookie)
	secure := strings.HasPrefix(base, "https")
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(oauthStateCookie, "", -1, "/api/auth/gitlab", "", secure, true)

	claims, ok := verifyConnectState(h.secret, state, time.Now())
	back := func(result string) {
		to := "/"
		if ok {
			to = claims.ReturnTo
		}
		c.Redirect(http.StatusFound, base+withQuery(to, "gitlab_oauth", result))
	}
	if !ok || cookieState == "" || cookieState != state {
		back("state_mismatch")
		return
	}
	code := c.Query("code")
	if code == "" {
		back(c.DefaultQuery("error", "no_code"))
		return
	}
	p, err := h.q.GetOAuthProvider(c, "gitlab")
	if err != nil || !p.Enabled {
		back("not_configured")
		return
	}
	secret, err := h.sealer.Decrypt(p.ClientSecretEnc)
	if err != nil {
		back("server_error")
		return
	}
	grant, err := gitlab.ExchangeOAuthGrant(c, p.GlBaseUrl, p.ClientID, secret, code, h.gitlabRedirectURI(c))
	if err != nil {
		log.Printf("gitlab connect: exchange: %v", err)
		back("exchange_failed")
		return
	}
	user, err := gitlab.New(p.GlBaseUrl, gitlab.MarkOAuthToken(grant.AccessToken)).CurrentUser(c)
	if err != nil {
		back("userinfo_failed")
		return
	}
	accessEnc, err := h.sealer.Encrypt(grant.AccessToken)
	if err != nil {
		back("server_error")
		return
	}
	refreshEnc := ""
	if grant.RefreshToken != "" {
		if refreshEnc, err = h.sealer.Encrypt(grant.RefreshToken); err != nil {
			back("server_error")
			return
		}
	}
	if _, err := h.q.UpsertGitlabCredential(c, db.UpsertGitlabCredentialParams{
		UserID: claims.User, BaseUrl: strings.TrimRight(p.GlBaseUrl, "/"), TokenEnc: accessEnc,
		GlUserID: user.ID, GlUsername: user.Username,
		AuthKind: "oauth", RefreshTokenEnc: refreshEnc, AccessExpiresAt: grant.ExpiresAt,
	}); err != nil {
		back("server_error")
		return
	}
	owner := claims.User
	_, rerr := h.q.ResumeGitlabIntegrationsForOwner(c, &owner)
	soft(c, "ResumeGitlabIntegrationsForOwner", rerr)
	back("connected")
}

// oauthRefreshLeeway renews an OAuth access token this long before it expires,
// so a sync never starts on a token that dies mid-run.
const oauthRefreshLeeway = 2 * time.Minute

// oauthRefreshLocks serialises the refresh of one user's grant: GitLab spends a
// refresh token on use, so two concurrent refreshes would leave one of them
// holding a dead token and pause the binding for nothing.
var oauthRefreshLocks sync.Map // user id → *sync.Mutex

// credentialToken returns the token a stored credential authenticates with — a
// PAT as is, an OAuth grant refreshed when due and marked for gitlab.New.
func (h *API) credentialToken(ctx context.Context, cred db.GitlabCredential) (string, error) {
	if cred.AuthKind != "oauth" {
		return h.sealer.Decrypt(cred.TokenEnc)
	}
	if cred.AccessExpiresAt == nil || time.Until(*cred.AccessExpiresAt) > oauthRefreshLeeway {
		tok, err := h.sealer.Decrypt(cred.TokenEnc)
		return gitlab.MarkOAuthToken(tok), err
	}
	mu, _ := oauthRefreshLocks.LoadOrStore(cred.UserID, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	defer mu.(*sync.Mutex).Unlock()
	// Somebody else may have refreshed while we waited.
	if fresh, err := h.q.GetGitlabCredential(ctx, cred.UserID); err == nil {
		cred = fresh
	}
	if cred.AccessExpiresAt != nil && time.Until(*cred.AccessExpiresAt) > oauthRefreshLeeway {
		tok, err := h.sealer.Decrypt(cred.TokenEnc)
		return gitlab.MarkOAuthToken(tok), err
	}
	grant, err := h.refreshGrant(ctx, cred)
	if err != nil {
		return "", err
	}
	return gitlab.MarkOAuthToken(grant.AccessToken), nil
}

// refreshGrant renews a stored OAuth grant and writes the new pair back.
func (h *API) refreshGrant(ctx context.Context, cred db.GitlabCredential) (gitlab.OAuthGrant, error) {
	if cred.RefreshTokenEnc == "" {
		return gitlab.OAuthGrant{}, gitlab.ErrUnauthorized
	}
	p, err := h.q.GetOAuthProvider(ctx, "gitlab")
	if err != nil {
		return gitlab.OAuthGrant{}, err
	}
	secret, err := h.sealer.Decrypt(p.ClientSecretEnc)
	if err != nil {
		return gitlab.OAuthGrant{}, err
	}
	refresh, err := h.sealer.Decrypt(cred.RefreshTokenEnc)
	if err != nil {
		return gitlab.OAuthGrant{}, err
	}
	grant, err := gitlab.RefreshOAuthGrant(ctx, cred.BaseUrl, p.ClientID, secret, refresh, h.gitlabOAuthRedirectURI())
	if err != nil {
		return gitlab.OAuthGrant{}, err
	}
	accessEnc, err := h.sealer.Encrypt(grant.AccessToken)
	if err != nil {
		return gitlab.OAuthGrant{}, err
	}
	refreshEnc := cred.RefreshTokenEnc
	if grant.RefreshToken != "" {
		if refreshEnc, err = h.sealer.Encrypt(grant.RefreshToken); err != nil {
			return gitlab.OAuthGrant{}, err
		}
	}
	if err := h.q.SetGitlabCredentialTokens(ctx, db.SetGitlabCredentialTokensParams{
		UserID: cred.UserID, TokenEnc: accessEnc, RefreshTokenEnc: refreshEnc, AccessExpiresAt: grant.ExpiresAt,
	}); err != nil {
		return gitlab.OAuthGrant{}, err
	}
	return grant, nil
}

// gitlabOAuthRedirectURI is the callback the grant was issued for. GitLab checks
// it on refresh too; with no request at hand it is built from PUBLIC_URL — the
// same origin the authorize step used whenever PUBLIC_URL is set (and it must be,
// in production: a proxy that drops the port would mint a wrong one, #2602).
func (h *API) gitlabOAuthRedirectURI() string {
	return strings.TrimRight(h.publicURL, "/") + "/api/auth/gitlab/callback"
}

// credentialView is the connection as the client sees it — never the token.
func credentialView(cred db.GitlabCredential) gin.H {
	return gin.H{
		"connected": true, "base_url": cred.BaseUrl, "gl_username": cred.GlUsername,
		"auth_kind": cred.AuthKind, "token_expires_at": dateOnly(cred.TokenExpiresAt),
	}
}

// dateOnly renders an optional date column as YYYY-MM-DD (nil stays null).
func dateOnly(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format("2006-01-02")
	return &s
}

// integrationCredential describes the credential a binding syncs under, for the
// connections view: the instance service token when there is one, else the
// owner's own connection.
func (h *API) integrationCredential(ctx context.Context, integ db.GitlabIntegration) (kind string, expires *string) {
	if p, err := h.q.GetOAuthProvider(ctx, "gitlab"); err == nil && p.ServiceTokenEnc != "" && p.GlBaseUrl != "" {
		return "service", dateOnly(p.ServiceTokenExpiresAt)
	}
	if integ.OwnerUserID == nil {
		return "", nil
	}
	cred, err := h.q.GetGitlabCredential(ctx, *integ.OwnerUserID)
	if errors.Is(err, pgx.ErrNoRows) || err != nil {
		return "", nil
	}
	return cred.AuthKind, dateOnly(cred.TokenExpiresAt)
}
