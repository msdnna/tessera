package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"tessera/internal/gitlab"
)

// GitLab uploads referenced in synced descriptions/comments are project-relative
// ("/uploads/<secret>/<file>") and need the integration's credentials to fetch.
// We rewrite them at sync time to a signed, same-origin proxy URL; the proxy
// (public, but HMAC-signed so only Tessera-minted links work) streams the file
// from GitLab using the integration owner's token. No expiry — the unguessable
// signature is the capability, like Tessera's own public uploads.

var uploadRe = regexp.MustCompile(`/uploads/[^\s)"'<>]+`)

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
	integ, err := h.q.GetGitlabIntegrationByWorkspace(c, wsID)
	if err != nil || integ.OwnerUserID == nil {
		c.Status(http.StatusNotFound)
		return
	}
	cred, err := h.q.GetGitlabCredential(c, *integ.OwnerUserID)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	token, err := h.sealer.Decrypt(cred.TokenEnc)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	base := strings.TrimRight(cred.BaseUrl, "/")
	projectPath := strings.Trim(integ.ProjectPath, "/")

	// GitLab 17.4+ exposes a PAT-authenticated uploads API; try it first and
	// stream the bytes. relPath "/uploads/<secret>/<file>" → ".../uploads/<secret>/<file>".
	rest := strings.TrimPrefix(relPath, "/uploads/")
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
	// with a GitLab session, never for the mobile app. On failure fall through to
	// 404 so the client shows its own fallback rather than a broken redirect.
	webURL := base + "/" + projectPath + relPath
	if req, rerr := http.NewRequestWithContext(c, http.MethodGet, webURL, nil); rerr == nil {
		req.Header.Set("PRIVATE-TOKEN", token)
		if resp, derr := gitlab.NewHTTPClient().Do(req); derr == nil {
			defer func() { _ = resp.Body.Close() }()
			ct := resp.Header.Get("Content-Type")
			// A sign-in redirect serves HTML — only stream real binary content.
			if resp.StatusCode == http.StatusOK && !strings.HasPrefix(ct, "text/html") {
				if ct == "" {
					ct = "application/octet-stream"
				}
				c.Header("Cache-Control", "private, max-age=3600")
				c.DataFromReader(http.StatusOK, resp.ContentLength, ct, resp.Body, nil)
				return
			}
		}
	}
	c.Status(http.StatusNotFound)
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

	req, rerr := http.NewRequestWithContext(c, http.MethodGet, rawURL, nil)
	if rerr != nil {
		c.Redirect(http.StatusFound, rawURL)
		return
	}
	// Attach the owner's token only when fetching from the GitLab host itself
	// (never leak it to gravatar/external hosts). Best-effort — public avatars
	// need no token.
	if integ, ierr := h.q.GetGitlabIntegrationByWorkspace(c, wsID); ierr == nil && integ.OwnerUserID != nil {
		if cred, cerr := h.q.GetGitlabCredential(c, *integ.OwnerUserID); cerr == nil {
			if gb, gerr := url.Parse(cred.BaseUrl); gerr == nil && gb.Host == target.Host {
				if token, derr := h.sealer.Decrypt(cred.TokenEnc); derr == nil {
					req.Header.Set("PRIVATE-TOKEN", token)
				}
			}
		}
	}
	// Don't auto-follow redirects: a redirecting GitLab/gravatar URL would both
	// leak the token cross-host and serve a sign-in HTML page; we handle non-image
	// responses by bouncing the client to the original URL instead (below).
	client := *gitlab.NewHTTPClient()
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, derr := client.Do(req)
	if derr == nil {
		defer func() { _ = resp.Body.Close() }()
		ct := resp.Header.Get("Content-Type")
		// Stream only a real image fetched directly (200, non-HTML). Anything else
		// (3xx, sign-in HTML, egress failure) falls through to the redirect.
		if resp.StatusCode == http.StatusOK && ct != "" && !strings.HasPrefix(ct, "text/") {
			c.Header("Cache-Control", "private, max-age=3600")
			c.DataFromReader(http.StatusOK, resp.ContentLength, ct, resp.Body, nil)
			return
		}
	}
	// Couldn't fetch a usable image server-side — let the client load the original
	// URL directly (works in a browser that can reach GitLab; the mobile app falls
	// back to initials). This guarantees no regression vs. the pre-proxy behaviour.
	c.Redirect(http.StatusFound, rawURL)
}
