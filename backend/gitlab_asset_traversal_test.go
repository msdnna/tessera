// The signed GitLab asset proxy is public by design — an <img> cannot send an
// Authorization header, so the HMAC is the whole capability. What the HMAC is
// supposed to bound is *which file* a link may fetch: the signature covers the
// upload path, so only paths Tessera itself minted can be asked for.
//
// The path Tessera mints, though, comes out of GitLab-authored text (an issue
// description or note picked over by uploadRe at sync time). If a path with
// ".." in it can be signed, the signature stops bounding anything: the proxy
// resolves it against the project URL, fetches it with the integration owner's
// PAT, and streams the answer to a caller who never logged in.
//
// Two arms, tested apart: sync must not mint such a link, and the proxy must
// not serve one it is handed.
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// proxyLinkRe pulls minted proxy URLs back out of a synced description.
var proxyLinkRe = regexp.MustCompile(`/api/gitlab/asset\?[^\s)"'<>]+`)

// escapePath walks out of the project the proxy resolves it against and lands
// on an instance-wide endpoint. It survives uploadRe — no space, quote, paren
// or angle bracket in it — and so would be signed like any real attachment.
const escapePath = "/uploads/../../../../../api/v4/personal_access_tokens"

// realUploadPath is what an actual GitLab attachment looks like. It rides along
// in the same description so the guard has to tell the two apart: a fix that
// stops minting links for real attachments would break every synced image.
const realUploadPath = "/uploads/0123456789abcdef0123456789abcdef/shot.png"

// signAssetLike mints the proxy URL the backend would mint, using the harness's
// configured encryption key (see TestMain) and the same derivation as NewAPI.
func signAssetLike(t *testing.T, wsID, relPath string) string {
	t.Helper()
	ak := sha256.Sum256([]byte("integration-test-encryption-key:gitlab-asset"))
	m := hmac.New(sha256.New, ak[:])
	m.Write([]byte(wsID + "\n" + relPath))
	return "/api/gitlab/asset?ws=" + wsID +
		"&p=" + base64.RawURLEncoding.EncodeToString([]byte(relPath)) +
		"&sig=" + hex.EncodeToString(m.Sum(nil))
}

// getAnon fetches a path with no token and no cookie — the access the public
// proxy grants by construction.
func getAnon(t *testing.T, path string) (int, string) {
	t.Helper()
	resp, err := http.Get(testServer.URL + path)
	if err != nil {
		t.Fatalf("anonymous fetch %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, string(body)
}

func TestGitlabAssetProxyRefusesTraversalPath(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-traversal-user", "grp-traversal")

	f.addIssue(glIssue{
		IID: 1, Title: "Innocent looking issue",
		Description: "скрин ![shot](" + realUploadPath + ") и ![x](" + escapePath + ")",
	})

	connectGitlab(t, c, f)
	integ := createIntegration(t, c, s.WS, s.Board, f, nil)
	triggerSync(t, c, s.WS, integ["id"].(string))
	waitSyncRuns(t, c, s.WS, 1)

	tasks := c.get("/boards/" + s.Board + "/tasks").listBody(t)
	if len(tasks) != 1 {
		t.Fatalf("synced tasks = %d, want 1", len(tasks))
	}
	desc, _ := c.expect(t, c.get("/tasks/"+tasks[0]["id"].(string)), http.StatusOK)["description"].(string)

	// Arm one: what sync was willing to sign. The real attachment gets a proxy
	// link; the traversal path is left as the inert text GitLab sent.
	var minted []string
	for _, link := range proxyLinkRe.FindAllString(desc, -1) {
		q, err := url.ParseQuery(strings.TrimPrefix(link, "/api/gitlab/asset?"))
		if err != nil {
			t.Fatalf("minted link %q: %v", link, err)
		}
		p, err := base64.RawURLEncoding.DecodeString(q.Get("p"))
		if err != nil {
			t.Fatalf("minted link %q: bad p: %v", link, err)
		}
		minted = append(minted, string(p))
	}
	if len(minted) != 1 || minted[0] != realUploadPath {
		t.Fatalf("sync signed %q, want exactly [%q] — a traversal path must never be\n"+
			"signed, and a real attachment must still be proxied\ndesc: %s", minted, realUploadPath, desc)
	}

	// Arm two: the proxy handed a correctly signed traversal path anyway. The
	// signature verifies; only the shape check stands between an anonymous
	// caller and the instance-wide endpoint.
	status, body := getAnon(t, signAssetLike(t, s.WS, escapePath))
	if f.sawPath("/api/v4/personal_access_tokens") {
		t.Fatalf("the signed proxy fetched /api/v4/personal_access_tokens from GitLab with the " +
			"owner's PAT — the HMAC bounds nothing once a signed path may contain \"..\"")
	}
	if strings.Contains(body, f.token) {
		t.Fatalf("anonymous caller was handed the integration owner's PAT:\n%s", body)
	}
	if status != http.StatusForbidden {
		t.Fatalf("traversal link: status %d, want 403\n%s", status, body)
	}

	// Control: the same signing, on the real attachment path, is still served —
	// the proxy answers it (404 here, since the fake has no uploads route) rather
	// than refusing it outright.
	if status, _ := getAnon(t, signAssetLike(t, s.WS, realUploadPath)); status == http.StatusForbidden {
		t.Fatalf("a real attachment link was refused as unsafe — the guard is too tight")
	}
}
