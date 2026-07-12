package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/db"
	"tessera/internal/gitlab"
)

// GitLab uploads referenced in synced descriptions/comments are project-relative
// ("/uploads/<secret>/<file>") and need the integration's credentials to fetch.
// We rewrite them at sync time to a signed, same-origin proxy URL; the proxy
// (public, but HMAC-signed so only Tessera-minted links work) streams the file
// from GitLab using the integration owner's token. No expiry — the unguessable
// signature is the capability, like Tessera's own public uploads.

var uploadRe = regexp.MustCompile(`/uploads/[^\s)"'<>]+`)

// firstOwnerCred returns the credential of the first workspace binding that has an
// owner set. The asset/avatar proxies only need any valid token for the GitLab
// instance (all bindings of a user share one self-hosted instance), so any owner's
// credential works.
func (h *API) firstOwnerCred(c *gin.Context, wsID uuid.UUID) (db.GitlabCredential, bool) {
	rows, err := h.q.ListGitlabIntegrationsByWorkspace(c, wsID)
	if err != nil {
		return db.GitlabCredential{}, false
	}
	for _, integ := range rows {
		if integ.OwnerUserID == nil {
			continue
		}
		if cred, cerr := h.q.GetGitlabCredential(c, *integ.OwnerUserID); cerr == nil {
			return cred, true
		}
	}
	return db.GitlabCredential{}, false
}

// signAsset is the HMAC over (workspace, relative path).
func (h *API) signAsset(wsID uuid.UUID, p string) string {
	m := hmac.New(sha256.New, h.assetKey)
	m.Write([]byte(wsID.String() + "\n" + p))
	return hex.EncodeToString(m.Sum(nil))
}

// assetProxyURL builds the same-origin signed proxy URL for a relative path.
func (h *API) assetProxyURL(wsID uuid.UUID, relPath string) string {
	return "/api/gitlab/asset?ws=" + wsID.String() +
		"&p=" + base64.RawURLEncoding.EncodeToString([]byte(relPath)) +
		"&sig=" + h.signAsset(wsID, relPath)
}

// avatarProxyURL builds a same-origin signed proxy URL for an absolute GitLab
// avatar URL. Clients with no direct GitLab access (e.g. the mobile app) load
// the avatar from Tessera, which fetches it server-side. "" stays "".
func (h *API) avatarProxyURL(wsID uuid.UUID, absURL string) string {
	if absURL == "" {
		return ""
	}
	return "/api/gitlab/avatar?ws=" + wsID.String() +
		"&u=" + base64.RawURLEncoding.EncodeToString([]byte(absURL)) +
		"&sig=" + h.signAsset(wsID, absURL)
}

// rewriteAssets rewrites GitLab project-relative "/uploads/…" links in markdown
// (or inline HTML) to signed proxy URLs so they resolve in Tessera.
func (h *API) rewriteAssets(body string, wsID uuid.UUID) string {
	if body == "" || !strings.Contains(body, "/uploads/") {
		return body
	}
	return uploadRe.ReplaceAllStringFunc(body, func(p string) string {
		return h.assetProxyURL(wsID, p)
	})
}

// GitlabAsset proxies a signed GitLab upload: it verifies the HMAC, then streams
// the file from the workspace integration's GitLab using the owner's token.
// Public (an <img> can't send auth) but only serves Tessera-signed paths.
func (h *API) GitlabAsset(c *gin.Context) {
	wsID, err := uuid.Parse(c.Query("ws"))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	pb, err := base64.RawURLEncoding.DecodeString(c.Query("p"))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	relPath := string(pb)
	// Only serve upload paths, and only with a valid signature.
	if !strings.HasPrefix(relPath, "/uploads/") ||
		!hmac.Equal([]byte(c.Query("sig")), []byte(h.signAsset(wsID, relPath))) {
		c.Status(http.StatusForbidden)
		return
	}
	cred, ok := h.firstOwnerCred(c, wsID)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
	token, err := h.sealer.Decrypt(cred.TokenEnc)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	base := strings.TrimRight(cred.BaseUrl, "/")
	rest := strings.TrimPrefix(relPath, "/uploads/")

	// The signed URL doesn't carry the GitLab project (an upload secret is unique
	// per instance, and the same signature scheme predates multi-binding). Try each
	// of the workspace's bindings' projects — all share one GitLab instance/owner
	// token — and stream from the first that resolves. The set is tiny.
	projectPaths := []string{}
	if rows, rerr := h.q.ListGitlabIntegrationsByWorkspace(c, wsID); rerr == nil {
		for _, integ := range rows {
			if p := strings.Trim(integ.ProjectPath, "/"); p != "" {
				projectPaths = append(projectPaths, p)
			}
		}
	}

	var lastWebURL string
	for _, projectPath := range projectPaths {
		// GitLab 17.4+ exposes a PAT-authenticated uploads API; try it first and
		// stream the bytes. relPath "/uploads/<secret>/<file>" → ".../uploads/<secret>/<file>".
		apiURL := base + "/api/v4/projects/" + url.QueryEscape(projectPath) + "/uploads/" + rest
		if req, rerr := http.NewRequestWithContext(c, http.MethodGet, apiURL, nil); rerr == nil {
			req.Header.Set("PRIVATE-TOKEN", token)
			if resp, derr := gitlab.NewHTTPClient().Do(req); derr == nil {
				if resp.StatusCode == http.StatusOK {
					defer func() { _ = resp.Body.Close() }()
					ct := resp.Header.Get("Content-Type")
					if ct == "" {
						ct = "application/octet-stream"
					}
					c.Header("Cache-Control", "private, max-age=3600")
					c.DataFromReader(http.StatusOK, resp.ContentLength, ct, resp.Body, nil)
					return
				}
				_ = resp.Body.Close()
			}
		}

		// Older GitLab (< 17.4) has no uploads API. Stream the web /uploads/ route
		// server-side (best-effort PRIVATE-TOKEN; works for public projects) instead
		// of redirecting — a redirect to the GitLab host only resolves for a browser
		// with a GitLab session, never for the mobile app.
		webURL := base + "/" + projectPath + relPath
		lastWebURL = webURL
		if req, rerr := http.NewRequestWithContext(c, http.MethodGet, webURL, nil); rerr == nil {
			req.Header.Set("PRIVATE-TOKEN", token)
			if resp, derr := gitlab.NewHTTPClient().Do(req); derr == nil {
				ct := resp.Header.Get("Content-Type")
				// A sign-in redirect serves HTML — only stream real binary content.
				if resp.StatusCode == http.StatusOK && !strings.HasPrefix(ct, "text/html") {
					defer func() { _ = resp.Body.Close() }()
					if ct == "" {
						ct = "application/octet-stream"
					}
					c.Header("Cache-Control", "private, max-age=3600")
					c.DataFromReader(http.StatusOK, resp.ContentLength, ct, resp.Body, nil)
					return
				}
				_ = resp.Body.Close()
			}
		}
	}
	// Couldn't stream the file server-side (no uploads API, session-gated web
	// route, egress failure). Bounce the client to the last GitLab web URL — a
	// browser with a GitLab session loads it directly, mirroring the avatar proxy's
	// fallback. (The mobile app can't follow this, but it had no access either.)
	if lastWebURL == "" {
		c.Status(http.StatusNotFound)
		return
	}
	c.Redirect(http.StatusFound, lastWebURL)
}

// GitlabAvatar proxies a signed absolute GitLab avatar URL: it verifies the HMAC,
// then streams the image so clients without direct GitLab access (the mobile app)
// can show it. The integration owner's token is sent ONLY when the URL host is
// the GitLab instance — never leaked to gravatar/external avatar hosts.
func (h *API) GitlabAvatar(c *gin.Context) {
	wsID, err := uuid.Parse(c.Query("ws"))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	ub, err := base64.RawURLEncoding.DecodeString(c.Query("u"))
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}
	rawURL := string(ub)
	if !hmac.Equal([]byte(c.Query("sig")), []byte(h.signAsset(wsID, rawURL))) {
		c.Status(http.StatusForbidden)
		return
	}
	target, err := url.Parse(rawURL)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") {
		c.Status(http.StatusBadRequest)
		return
	}

	// The owner's token, only when the URL host is the GitLab instance (never
	// leaked to gravatar/external hosts). Best-effort — public avatars need none.
	var token string
	if cred, ok := h.firstOwnerCred(c, wsID); ok {
		if gb, gerr := url.Parse(cred.BaseUrl); gerr == nil && gb.Host == target.Host {
			if t, derr := h.sealer.Decrypt(cred.TokenEnc); derr == nil {
				token = t
			}
		}
	}

	if h.streamGitlabImage(c, rawURL, target.Host, token) {
		return
	}
	// Couldn't fetch a usable image server-side — let the client load the original
	// URL directly (works in a browser that can reach GitLab; other clients fall
	// back to initials). No regression vs. the pre-proxy behaviour.
	c.Redirect(http.StatusFound, rawURL)
}

// imgAttempt is one server-side fetch of the avatar with a specific credential
// placement. GitLab authenticates differently across its endpoints, so we try the
// PAT as a Bearer header (what the API/GraphQL accept), a PRIVATE-TOKEN header, and
// a `?private_token=` query param (older web/upload routes) before giving up.
type imgAttempt struct {
	url    string
	header string // header name, or "" for none
	value  string
}

// streamGitlabImage fetches an image server-side and streams it to the client,
// returning true on success. It follows redirects (a GitLab avatar URL often 302s
// to the actual file), stripping the credential whenever the redirect leaves the
// original host so it's never leaked. On total failure it logs the last GitLab
// response (status + content-type + a short body snippet) so a session-only route
// (private instance, no PAT support on /uploads/-/system/user/avatar) is diagnosable.
func (h *API) streamGitlabImage(c *gin.Context, rawURL, ghost, token string) bool {
	var attempts []imgAttempt
	if token != "" {
		sep := "?"
		if strings.Contains(rawURL, "?") {
			sep = "&"
		}
		attempts = []imgAttempt{
			{rawURL, "Authorization", "Bearer " + token},
			{rawURL, "PRIVATE-TOKEN", token},
			{rawURL + sep + "private_token=" + url.QueryEscape(token), "", ""},
		}
	} else {
		// External host (e.g. gravatar) — no credential to send.
		attempts = []imgAttempt{{rawURL, "", ""}}
	}

	client := *gitlab.NewHTTPClient()
	client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return http.ErrUseLastResponse
		}
		if r.URL.Host != ghost {
			r.Header.Del("PRIVATE-TOKEN")
			r.Header.Del("Authorization")
		}
		return nil
	}

	var lastStatus int
	var lastCT, lastBody string
	for _, a := range attempts {
		req, rerr := http.NewRequestWithContext(c, http.MethodGet, a.url, nil)
		if rerr != nil {
			continue
		}
		if a.header != "" {
			req.Header.Set(a.header, a.value)
		}
		resp, derr := client.Do(req)
		if derr != nil {
			lastStatus, lastCT, lastBody = 0, "", derr.Error()
			continue
		}
		ct := resp.Header.Get("Content-Type")
		// Accept a real image only (200, non-text). A sign-in HTML page or a
		// remaining 3xx means this attempt didn't authenticate — try the next.
		if resp.StatusCode == http.StatusOK && ct != "" && !strings.HasPrefix(ct, "text/") {
			c.Header("Cache-Control", "private, max-age=3600")
			c.DataFromReader(http.StatusOK, resp.ContentLength, ct, resp.Body, nil)
			_ = resp.Body.Close()
			return true
		}
		snippet := make([]byte, 120)
		n, _ := io.ReadFull(io.LimitReader(resp.Body, 120), snippet)
		lastStatus, lastCT, lastBody = resp.StatusCode, ct, strings.TrimSpace(string(snippet[:n]))
		_ = resp.Body.Close()
	}
	log.Printf("gitlab avatar proxy: could not fetch %q server-side (last status=%d ct=%q body=%q) — "+
		"the instance's avatar web route likely needs a session cookie the PAT can't provide",
		rawURL, lastStatus, lastCT, lastBody)
	return false
}
