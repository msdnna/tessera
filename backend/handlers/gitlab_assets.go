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

	// Fetch via the project uploads API (authenticates with the PAT) rather than
	// the web /uploads/ route (which needs a session and serves the login page).
	// relPath is "/uploads/<secret>/<filename>" → API ".../uploads/<secret>/<filename>".
	rest := strings.TrimPrefix(relPath, "/uploads/")
	target := strings.TrimRight(cred.BaseUrl, "/") +
		"/api/v4/projects/" + url.QueryEscape(strings.Trim(integ.ProjectPath, "/")) +
		"/uploads/" + rest
	req, err := http.NewRequestWithContext(c, http.MethodGet, target, nil)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}
	req.Header.Set("PRIVATE-TOKEN", token)
	resp, err := gitlab.NewHTTPClient().Do(req)
	if err != nil {
		c.Status(http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		c.Status(resp.StatusCode) // forward upstream status (e.g. 401/404) for debugging
		return
	}
	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "application/octet-stream"
	}
	c.Header("Cache-Control", "private, max-age=3600")
	c.DataFromReader(http.StatusOK, resp.ContentLength, ct, resp.Body, nil)
}
