package handlers

import (
	"context"
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
	"tessera/internal/observability"
)

// GitLab uploads referenced in synced descriptions/comments are project-relative
// ("/uploads/<secret>/<file>") and need the integration's credentials to fetch.
// We rewrite them at sync time to a signed, same-origin proxy URL; the proxy
// (public, but HMAC-signed so only Tessera-minted links work) streams the file
// from GitLab using the integration owner's token. No expiry — the unguessable
// signature is the capability, like Tessera's own public uploads.

// uploadRe matches a GitLab project-relative upload path. The first alternative is
// not a target but a guard: Tessera's own "/api/uploads/<name>" ends in the literal
// "/uploads/…", so without claiming the longer form first the rewrite would fire on
// the tail and mangle a Tessera URL into "/api/" + a proxy link. RE2 has no
// lookbehind, so the alternation does the work and rewriteInbound skips the match.
var uploadRe = regexp.MustCompile(`/api/uploads/[A-Za-z0-9_-]+\.[A-Za-z0-9]+|/uploads/[^\s)"'<>]+`)

// safeUploadPath reports whether a proxy path is a plain GitLab upload
// reference rather than one that walks out of it.
//
// The "/uploads/" prefix alone bounds nothing. The path is lifted out of
// GitLab-authored text by uploadRe, which allows dots and slashes, and the
// proxy resolves it against the project's URL before fetching it with the
// integration owner's token. One ".." segment therefore turns a signed
// attachment link into a signed link to any endpoint of the instance — and the
// proxy has to be public (an <img> cannot authenticate), so the answer goes to
// whoever asks. Percent-escapes are decoded first: the reverse proxy in front
// of GitLab decodes before it resolves the path, so "%2e%2e" is a ".." by the
// time anything acts on it.
func safeUploadPath(p string) bool {
	if !strings.HasPrefix(p, "/uploads/") {
		return false
	}
	dec, err := url.PathUnescape(p)
	if err != nil {
		return false
	}
	if strings.Contains(dec, `\`) {
		return false
	}
	for _, seg := range strings.Split(dec, "/") {
		if seg == "." || seg == ".." {
			return false
		}
	}
	return true
}

// assetGitlabConn resolves the connection for the asset/avatar proxies: the
// instance service token first, else any workspace binding owner's PAT. Returns the
// (trimmed) base URL + decrypted token.
func (h *API) assetGitlabConn(c *gin.Context, wsID uuid.UUID) (string, string, bool) {
	if b, t, ok := h.serviceGitlabConn(c); ok {
		return b, t, true
	}
	if cred, ok := h.firstOwnerCred(c, wsID); ok {
		if t, err := h.sealer.Decrypt(cred.TokenEnc); err == nil {
			return strings.TrimRight(cred.BaseUrl, "/"), t, true
		}
	}
	return "", "", false
}

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
//
// One exception, and it is what keeps outbound mirroring from fighting the conflict
// detector (task #2713): a link Tessera itself uploaded into GitLab is mapped back
// to the "/api/uploads/<name>" URL it came from, not to a proxy URL. Otherwise a
// description that went out with a mirrored image would come back differently shaped
// than the one `tasks.description` holds, every title_desc comparison would read as
// diverged, and each edit would re-upload the whole image set.
func (h *API) rewriteAssets(ctx context.Context, body string, wsID uuid.UUID) string {
	if body == "" || !strings.Contains(body, "/uploads/") {
		return body
	}
	return rewriteInbound(body,
		func(p string) (string, bool) {
			src, err := h.q.GetGitlabUploadSourceByURL(ctx, db.GetGitlabUploadSourceByURLParams{
				WorkspaceID: wsID, GlUrl: p,
			})
			return src, err == nil && strings.HasPrefix(src, "/api/uploads/")
		},
		func(p string) string { return h.assetProxyURL(wsID, p) },
	)
}

// rewriteInbound is the pure half of rewriteAssets: ours reports whether a GitLab
// upload URL is one Tessera itself mirrored (and with which source URL), proxy wraps
// everything else. Split out so the round-trip against rewriteOutbound is testable
// without a database.
//
// Code spans are skipped, mirroring the outbound rewrite. Beyond not corrupting a
// snippet, the symmetry is load-bearing: if only one direction skipped code, a body
// with an upload path inside a fence would come back differently shaped than it went
// out and title_desc would read as permanently diverged.
func rewriteInbound(body string, ours func(string) (string, bool), proxy func(string) string) string {
	return replaceOutsideCode(body, codeRanges(body), uploadRe, func(m []string) string {
		p := m[0]
		if strings.HasPrefix(p, "/api/uploads/") {
			return p // a Tessera URL matched only by the guard alternative — leave it
		}
		if src, ok := ours(p); ok {
			return src
		}
		// Not a shape we can bound — leave it as the text GitLab sent. Signing it
		// would mint a capability for a path nobody chose (see safeUploadPath), and
		// the round trip stays symmetric because rewriteOutbound has nothing to
		// match on a path that was never turned into a proxy URL.
		if !safeUploadPath(p) {
			return p
		}
		return proxy(p)
	})
}

// streamProxied hands a proxied GitLab response to the caller on our own origin.
//
// The HMAC bounds which file may be fetched; it says nothing about what the file
// is. Both proxies are public and same-origin, so whatever they return renders
// with the app's session — and the bytes, along with the Content-Type GitLab
// declares for them, come from an attachment somebody put on a synced issue. An
// SVG (or, on the uploads-API branch, anything at all) would therefore decide
// for itself that it is active content on the origin that holds the session in
// localStorage, via a link the sync itself wrote into the task description.
//
// So: the declared type is honoured only for the image types Tessera serves
// inline anywhere else (inlineSafeTypes, shared with ServeUpload and
// DocumentAsset), and everything else is handed over as an opaque download.
// nosniff pins the verdict and the sandbox CSP neuters anything that still gets
// rendered — both inert for the real screenshots this proxy exists to show.
func streamProxied(c *gin.Context, ct string, length int64, body io.Reader) {
	c.Header("Cache-Control", "private, max-age=3600")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "default-src 'none'; sandbox")
	base, _, _ := strings.Cut(ct, ";")
	if !inlineSafeTypes[strings.ToLower(strings.TrimSpace(base))] {
		c.Header("Content-Disposition", "attachment")
		ct = "application/octet-stream"
	}
	c.DataFromReader(http.StatusOK, length, ct, body, nil)
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
	// Only serve real upload paths, and only with a valid signature. The shape
	// check is not redundant with the HMAC: it is what keeps the HMAC meaningful,
	// since an older link (or a future signing path) could carry anything.
	if !safeUploadPath(relPath) ||
		!hmac.Equal([]byte(c.Query("sig")), []byte(h.signAsset(wsID, relPath))) {
		c.Status(http.StatusForbidden)
		return
	}
	base, token, ok := h.assetGitlabConn(c, wsID)
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
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
					streamProxied(c, resp.Header.Get("Content-Type"), resp.ContentLength, resp.Body)
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
					streamProxied(c, ct, resp.ContentLength, resp.Body)
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
	if base, t, ok := h.assetGitlabConn(c, wsID); ok {
		if gb, gerr := url.Parse(base); gerr == nil && gb.Host == target.Host {
			token = t
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
			// Not derr.Error() as-is: a transport failure yields a *url.Error
			// that quotes the whole request URL, and the third attempt has the
			// PAT in it. That attempt is also the one that overwrites whatever
			// the first two recorded, so the credential would be what the
			// diagnostic below prints.
			lastStatus, lastCT, lastBody = 0, "", observability.RedactURLs(derr.Error())
			continue
		}
		ct := resp.Header.Get("Content-Type")
		// Accept a real image only (200, non-text). A sign-in HTML page or a
		// remaining 3xx means this attempt didn't authenticate — try the next.
		if resp.StatusCode == http.StatusOK && ct != "" && !strings.HasPrefix(ct, "text/") {
			streamProxied(c, ct, resp.ContentLength, resp.Body)
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
