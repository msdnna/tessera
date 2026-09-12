// The signed GitLab proxies stream somebody else's bytes from Tessera's own
// origin, to a caller who never logged in. What the HMAC bounds is *which file*
// may be fetched — it says nothing about what the file turns out to be.
//
// The bytes are not ours and their declared type is not ours either: the proxy
// reprinted the Content-Type of the GitLab response verbatim, so an attachment
// on a synced issue decided how the browser would treat it. An .svg (or, on the
// uploads-API branch, an .html) came back as active content on the app's origin,
// where the session lives in localStorage — and the link into it was planted by
// the sync itself, straight out of the issue body, for every Tessera user who
// opens the task.
//
// The two sibling routes that serve user files (ServeUpload, DocumentAsset)
// already answer this with nosniff + a sandbox CSP + an octet-stream fallback
// for anything outside the image allowlist. This is the same guard on the two
// proxies that reach outside.
package main

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// svgUploadPath is an attachment anyone able to comment on the synced issue can
// put on the instance. The extension is inert here — what decides the browser's
// behaviour is the type GitLab declares, which is why the fake declares it.
const svgUploadPath = "/uploads/aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa/pwn.svg"

// pngUploadPath is the control: a real image has to keep rendering inline, or
// the guard has broken every synced screenshot to fix the hole.
const pngUploadPath = "/uploads/bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb/shot.png"

// svgPayload scripts on whatever origin renders it.
const svgPayload = `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(document.domain)</script></svg>`

// getAnonResp fetches a path with no credential at all and returns the response
// with its body already read — the headers are the subject here.
func getAnonResp(t *testing.T, path string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Get(testServer.URL + path)
	if err != nil {
		t.Fatalf("anonymous fetch %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	return resp, string(body)
}

// mintedAssetLink finds the proxy URL the sync wrote for a given upload path.
func mintedAssetLink(t *testing.T, desc, want string) string {
	t.Helper()
	for _, link := range proxyLinkRe.FindAllString(desc, -1) {
		q, err := url.ParseQuery(strings.TrimPrefix(link, "/api/gitlab/asset?"))
		if err != nil {
			t.Fatalf("minted link %q: %v", link, err)
		}
		p, err := base64.RawURLEncoding.DecodeString(q.Get("p"))
		if err != nil {
			t.Fatalf("minted link %q: bad p: %v", link, err)
		}
		if string(p) == want {
			return link
		}
	}
	t.Fatalf("sync minted no proxy link for %q\ndesc: %s", want, desc)
	return ""
}

func TestGitlabAssetProxyNeverServesActiveContentInline(t *testing.T) {
	t.Parallel()
	c := signup(t)
	makeAdmin(t, c)
	s := mkStack(t, c)
	f := newFakeGitlab(t, "gl-inline-user", "grp-inline")

	f.addUpload(svgUploadPath, "image/svg+xml", svgPayload)
	f.addUpload(pngUploadPath, "image/png", "\x89PNG\r\n\x1a\nfake")
	f.addIssue(glIssue{
		IID: 1, Title: "Issue with attachments",
		Description: "скрин ![shot](" + pngUploadPath + ") и ![x](" + svgUploadPath + ")",
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

	// The attack link is not forged: the sync minted it, into the description
	// every member of the workspace reads.
	resp, body := getAnonResp(t, mintedAssetLink(t, desc, svgUploadPath))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("proxy fetch of the attachment: status %d, want 200 (the file must still be\n"+
			"reachable — this is about *how* it is served)\n%s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "svg") {
		t.Errorf("the public proxy served the attachment as %q on the app's own origin —\n"+
			"an SVG rendered there scripts against the session in localStorage", ct)
	}
	if got := resp.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want \"nosniff\"", got)
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "sandbox") {
		t.Errorf("Content-Security-Policy = %q, want a sandbox (as on ServeUpload)", csp)
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.HasPrefix(cd, "attachment") {
		t.Errorf("Content-Disposition = %q, want an attachment for a non-image type", cd)
	}
	// The bytes themselves may still be handed over — it is a file the task links
	// to. Only its execution on our origin is refused.
	if !strings.Contains(body, "<script>") {
		t.Logf("note: proxy did not return the attachment bytes (%q)", body)
	}

	// Control: the screenshot next to it still renders inline as a real image.
	resp, body = getAnonResp(t, mintedAssetLink(t, desc, pngUploadPath))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("proxy fetch of the screenshot: status %d, want 200\n%s", resp.StatusCode, body)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "image/png") {
		t.Errorf("screenshot served as %q, want image/png — the guard must not break real images", ct)
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != "" {
		t.Errorf("screenshot forced to download (%q) — inline images are the whole point of the proxy", cd)
	}
}
