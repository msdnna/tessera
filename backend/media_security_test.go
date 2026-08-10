// Integration tests for #2621: the public /uploads route is same-origin, so
// anything it renders runs with the app's session. These pin both halves of the
// fix — what may be stored, and how what's already stored is handed out.
package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	pngBody  = "\x89PNG\r\n\x1a\nfake image body"
	svgBody  = `<svg xmlns="http://www.w3.org/2000/svg"><script>alert(document.cookie)</script></svg>`
	htmlBody = "<!DOCTYPE html><html><body><script>alert(1)</script></body></html>"
)

// Uploads are gated on the leading bytes, so neither the declared Content-Type
// nor the filename can smuggle scriptable content in.
func TestMediaUploadRejectsScriptableContent(t *testing.T) {
	t.Parallel()
	c := signup(t)

	for _, tc := range []struct {
		name, filename, body string
	}{
		{"svg by name", "payload.svg", svgBody},
		{"svg disguised as png", "payload.png", svgBody},
		{"html disguised as png", "payload.png", htmlBody},
		{"html disguised as jpg", "payload.jpg", htmlBody},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := uploadFile(t, c, "/uploads", "file", tc.filename, []byte(tc.body))
			if r.Status != http.StatusBadRequest {
				t.Fatalf("upload %s: status %d, want 400\n%s", tc.filename, r.Status, r.Body)
			}
		})
	}
}

// The stored extension comes from the sniff, not from the client — a real PNG
// named .svg is kept, but as a .png.
func TestMediaUploadExtensionFollowsBytes(t *testing.T) {
	t.Parallel()
	c := signup(t)

	m := c.expect(t, uploadFile(t, c, "/uploads", "file", "actually-a-png.svg", []byte(pngBody)), http.StatusCreated)
	url, _ := m["url"].(string)
	if !strings.HasSuffix(url, ".png") {
		t.Fatalf("stored url %q, want a .png suffix", url)
	}
}

// A legitimate image still renders inline, but with the sniffing and active
// content escapes closed off.
func TestMediaServeHardening(t *testing.T) {
	t.Parallel()
	c := signup(t)

	m := c.expect(t, uploadFile(t, c, "/uploads", "file", "картинка.png", []byte(pngBody)), http.StatusCreated)
	url, _ := m["url"].(string)

	res, err := http.Get(testServer.URL + url)
	if err != nil {
		t.Fatalf("public GET: %v", err)
	}
	defer res.Body.Close()

	if got := res.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options: %q", got)
	}
	if got := res.Header.Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'none'") {
		t.Fatalf("Content-Security-Policy: %q", got)
	}
	if got := res.Header.Get("Content-Type"); !strings.HasPrefix(got, "image/png") {
		t.Fatalf("Content-Type: %q, want image/png", got)
	}
	if got := res.Header.Get("Content-Disposition"); got != "" {
		t.Fatalf("Content-Disposition: %q, want images to stay inline", got)
	}
}

// SVGs uploaded before the fix are still on disk. They must come back as opaque
// downloads rather than as documents the browser will execute.
func TestLegacySvgServedInert(t *testing.T) {
	t.Parallel()
	if testUploadDir == "" {
		t.Skip("no upload dir")
	}
	dir := filepath.Join(testUploadDir, "media")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	name := "legacy-payload.svg"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(svgBody), 0o600); err != nil {
		t.Fatalf("plant legacy svg: %v", err)
	}

	res, err := http.Get(testServer.URL + "/api/uploads/" + name)
	if err != nil {
		t.Fatalf("public GET: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("legacy svg: status %d", res.StatusCode)
	}
	if got := res.Header.Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("Content-Type: %q, want application/octet-stream", got)
	}
	if got := res.Header.Get("Content-Disposition"); got != "attachment" {
		t.Fatalf("Content-Disposition: %q, want attachment", got)
	}
	if got := res.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options: %q", got)
	}
}

// Task attachments accept any file by design; the download route is what keeps
// them from rendering.
func TestAttachmentDownloadIsInert(t *testing.T) {
	t.Parallel()
	c := signup(t)
	s := mkStack(t, c)
	task := mkTask(t, c, s.Board, s.col(t, 0), "С вложением")
	id := task["id"].(string)

	c.expect(t, uploadFile(t, c, "/tasks/"+id+"/attachments", "file", "payload.html", []byte(htmlBody)),
		http.StatusCreated)
	att := c.get("/tasks/" + id + "/attachments").listBody(t)
	if len(att) != 1 {
		t.Fatalf("attachments: %v", att)
	}

	dl := c.get("/attachments/" + att[0]["id"].(string) + "/download")
	if dl.Status != http.StatusOK {
		t.Fatalf("download: %d", dl.Status)
	}
	if got := dl.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options: %q", got)
	}
	if got := dl.Header.Get("Content-Disposition"); !strings.HasPrefix(got, "attachment") {
		t.Fatalf("Content-Disposition: %q", got)
	}
}
