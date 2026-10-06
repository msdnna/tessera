package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"tessera-mcp/internal/client"
)

// pngBytes is a one-pixel PNG header — enough for the fetch to look like an image.
var pngBytes = []byte("\x89PNG\r\n\x1a\n")

// authRecorder is an HTTP server that serves a PNG for any path and remembers the
// Authorization header it was asked with.
type authRecorder struct {
	*httptest.Server
	hits int
	auth string
}

func newAuthRecorder(t *testing.T) *authRecorder {
	t.Helper()
	r := &authRecorder{}
	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.hits++
		r.auth = req.Header.Get("Authorization")
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	}))
	t.Cleanup(r.Close)
	return r
}

// taskWithDescription serves GET /api/tasks/t1 with the given description and an
// image at /api/uploads/up.png, recording the Authorization header of the latter.
func taskWithDescription(t *testing.T, description string) (*client.Client, *authRecorder) {
	t.Helper()
	own := &authRecorder{}
	own.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/api/tasks/t1" {
			d := detail("t1", "b1")
			d.Description = description
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(d)
			return
		}
		if req.URL.Path == "/api/uploads/up.png" {
			own.hits++
			own.auth = req.Header.Get("Authorization")
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(pngBytes)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(own.Close)
	return client.New(own.URL+"/api", "tsra_secret"), own
}

// A task's description is not ours: any member writes it, and the GitLab sync
// mirrors an issue body into it verbatim. So an absolute image URL in there points
// wherever its author chose, and view_image fetches every image it finds — the one
// credential that must not travel to that address is the token itself.
func TestViewImageDoesNotSendTokenOffHost(t *testing.T) {
	foreign := newAuthRecorder(t)
	c, _ := taskWithDescription(t, "Смотри скриншот: ![скрин]("+foreign.URL+"/p.png)")

	_, out, err := viewImage(c)(context.Background(), nil, viewImageInput{TaskID: "t1"})
	if err != nil {
		t.Fatalf("viewImage: %v", err)
	}
	if out.Count != 1 {
		t.Fatalf("expected the foreign image to be fetched, got count=%d refs=%v", out.Count, out.Refs)
	}
	if foreign.hits == 0 {
		t.Fatal("the foreign host was never asked — the test proves nothing")
	}
	if foreign.auth != "" {
		t.Fatalf("утечка: personal access token уехал на сторонний хост %s в заголовке %q",
			foreign.URL, foreign.auth)
	}
}

// The counterweight: an image on our own backend still has to be fetched with the
// token, or every attachment and every non-public upload stops loading.
func TestViewImageStillAuthenticatesOwnHost(t *testing.T) {
	c, own := taskWithDescription(t, "![up](/api/uploads/up.png)")

	_, out, err := viewImage(c)(context.Background(), nil, viewImageInput{TaskID: "t1"})
	if err != nil || out.Count != 1 {
		t.Fatalf("viewImage: %+v %v", out, err)
	}
	if own.auth != "Bearer tsra_secret" {
		t.Fatalf("own host got %q, want the bearer token", own.auth)
	}
}

// An attachment ref is built from the API base, not from task text, and must keep
// its bearer whichever branch of FetchImage builds it.
func TestFetchImageAttachmentKeepsToken(t *testing.T) {
	rec := newAuthRecorder(t)
	c := client.New(rec.URL+"/api", "tsra_secret")
	if _, _, err := c.FetchImage(context.Background(), "attachment:a1"); err != nil {
		t.Fatalf("FetchImage: %v", err)
	}
	if rec.auth != "Bearer tsra_secret" {
		t.Fatalf("attachment download got %q, want the bearer token", rec.auth)
	}
}

// A relative ref cannot name another host, so it stays authenticated too.
func TestFetchImageRelativeRefKeepsToken(t *testing.T) {
	rec := newAuthRecorder(t)
	c := client.New(rec.URL+"/api", "tsra_secret")
	for _, ref := range []string{"/api/uploads/up.png", "/uploads/up.png"} {
		rec.auth = ""
		if _, _, err := c.FetchImage(context.Background(), ref); err != nil {
			t.Fatalf("FetchImage(%q): %v", ref, err)
		}
		if rec.auth != "Bearer tsra_secret" {
			t.Fatalf("FetchImage(%q) got %q, want the bearer token", ref, rec.auth)
		}
	}
}

// Our own host may answer a fetch with a redirect elsewhere, and the token must
// not follow — otherwise one 302 walks around the guard above. net/http drops
// Authorization by itself only when the hop crosses to another *domain*; here both
// ends are 127.0.0.1 on different ports, which is what an install sharing a
// machine with other services looks like, and that hop kept the header until the
// client re-applied its own rule per hop.
func TestFetchImageDropsTokenOnForeignRedirect(t *testing.T) {
	foreign := newAuthRecorder(t)
	var ownURL string
	own := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, foreign.URL+"/p.png", http.StatusFound)
	}))
	t.Cleanup(own.Close)
	ownURL = own.URL

	c := client.New(ownURL+"/api", "tsra_secret")
	if _, _, err := c.FetchImage(context.Background(), "/api/uploads/up.png"); err != nil {
		t.Fatalf("FetchImage: %v", err)
	}
	if foreign.hits == 0 {
		t.Fatal("the redirect was never followed — the test proves nothing")
	}
	if foreign.auth != "" {
		t.Fatalf("утечка через редирект: токен уехал на %s в заголовке %q", foreign.URL, foreign.auth)
	}
}

// An absolute URL that happens to name our own backend is the shape a task
// carries after a copy-paste out of the browser's address bar — same host, so the
// token belongs on it.
func TestFetchImageAbsoluteOwnHostKeepsToken(t *testing.T) {
	rec := newAuthRecorder(t)
	c := client.New(rec.URL+"/api", "tsra_secret")
	if _, _, err := c.FetchImage(context.Background(), rec.URL+"/api/uploads/up.png"); err != nil {
		t.Fatalf("FetchImage: %v", err)
	}
	if rec.auth != "Bearer tsra_secret" {
		t.Fatalf("absolute own-host ref got %q, want the bearer token", rec.auth)
	}
}
