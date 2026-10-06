// A changed avatar has to be visible at once (#2932).
//
// The avatar lives at a URL that never changes — /api/users/<id>/avatar — and
// UserAvatar.vue builds that URL itself from the user id, so there is nowhere to
// hang a version. Served with a plain `max-age=300` and no validator, the
// browser is entitled to keep showing the previous picture for five minutes: the
// Settings preview updates (it appends its own ?t= buster) while the sidebar,
// the cards, the comments and the admin table keep the old one, and a reload
// does not help. Deleting an avatar has the worse shape of the same bug — the
// server already answers 404 and the browser still paints the image.
//
// The other media endpoints cache far harder (immutable, a year) and are right
// to: their URLs carry a per-upload UUID, so new content means a new URL. The
// avatar is the one whose URL outlives its content, which is exactly the case
// that needs a validator instead of a lifetime.
package main

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

// gifBytes builds a distinguishable payload that http.DetectContentType accepts
// as an image (the handler rejects anything else).
func gifBytes(marker byte) []byte {
	b := []byte("GIF89a")
	return append(b, bytes.Repeat([]byte{marker}, 32)...)
}

// putAvatar uploads an avatar for the client (the route is PUT, so the shared
// uploadFile helper — which posts — does not fit).
func putAvatar(t *testing.T, c *client, content []byte) resp {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, err := w.CreateFormFile("avatar", "avatar.gif")
	if err != nil {
		t.Fatalf("multipart: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("multipart write: %v", err)
	}
	_ = w.Close()
	req, err := http.NewRequest(http.MethodPut, testServer.URL+"/api/users/me/avatar", &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("upload avatar: %v", err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	return resp{Status: res.StatusCode, Body: data, Header: res.Header}
}

// getAvatar fetches a user's avatar, optionally conditionally. The endpoint is
// unauthenticated (it is loaded straight into <img>), so no token is sent.
func getAvatar(t *testing.T, userID, ifNoneMatch string) resp {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, testServer.URL+"/api/users/"+userID+"/avatar", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if ifNoneMatch != "" {
		req.Header.Set("If-None-Match", ifNoneMatch)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get avatar: %v", err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	return resp{Status: res.StatusCode, Body: data, Header: res.Header}
}

// TestAvatarCarriesValidator is the premise: a cached copy must be checkable.
// Without a validator the browser has nothing to ask about and falls back to the
// lifetime, which is what makes the stale picture survive a reload.
func TestAvatarCarriesValidator(t *testing.T) {
	t.Parallel()
	u := signup(t)
	if r := putAvatar(t, u, gifBytes('a')); r.Status != http.StatusOK {
		t.Fatalf("upload avatar: status %d\n%s", r.Status, r.Body)
	}

	r := getAvatar(t, u.UserID, "")
	if r.Status != http.StatusOK {
		t.Fatalf("get avatar: status %d\n%s", r.Status, r.Body)
	}
	etag := r.Header.Get("ETag")
	if etag == "" {
		t.Fatalf("avatar served without an ETag; Cache-Control=%q", r.Header.Get("Cache-Control"))
	}
	// A lifetime without "no-cache"/"must-revalidate"/max-age=0 lets the browser
	// reuse the old bytes without ever asking, ETag or not.
	cc := r.Header.Get("Cache-Control")
	if !strings.Contains(cc, "no-cache") && !strings.Contains(cc, "must-revalidate") {
		t.Fatalf("Cache-Control %q lets a stale avatar be reused without revalidating", cc)
	}
	// The cheap half of the deal: an unchanged avatar must still answer 304, or
	// dropping the lifetime would mean re-sending the bytes on every page.
	if rr := getAvatar(t, u.UserID, etag); rr.Status != http.StatusNotModified {
		t.Fatalf("conditional GET of an unchanged avatar: status %d, want 304", rr.Status)
	}
}

// TestAvatarReuploadIsVisibleImmediately is the reported symptom: the picture
// changes and everything outside the Settings preview keeps the old one.
func TestAvatarReuploadIsVisibleImmediately(t *testing.T) {
	t.Parallel()
	u := signup(t)
	first, second := gifBytes('a'), gifBytes('b')
	if r := putAvatar(t, u, first); r.Status != http.StatusOK {
		t.Fatalf("upload first avatar: status %d\n%s", r.Status, r.Body)
	}
	etag := getAvatar(t, u.UserID, "").Header.Get("ETag")

	if r := putAvatar(t, u, second); r.Status != http.StatusOK {
		t.Fatalf("upload second avatar: status %d\n%s", r.Status, r.Body)
	}
	r := getAvatar(t, u.UserID, etag)
	if r.Status == http.StatusNotModified {
		t.Fatalf("re-uploaded avatar still answers 304 to the old validator")
	}
	if r.Status != http.StatusOK {
		t.Fatalf("get avatar after re-upload: status %d\n%s", r.Status, r.Body)
	}
	if !bytes.Equal(r.Body, second) {
		t.Fatalf("avatar bytes did not change after re-upload")
	}
	if got := r.Header.Get("ETag"); got == etag {
		t.Fatalf("ETag %q unchanged across a re-upload", got)
	}
}

// TestAvatarDeletionIsVisibleImmediately covers the worse half: the row is gone,
// the server says 404, and a client holding the old validator must not be told
// its copy is still good.
func TestAvatarDeletionIsVisibleImmediately(t *testing.T) {
	t.Parallel()
	u := signup(t)
	if r := putAvatar(t, u, gifBytes('a')); r.Status != http.StatusOK {
		t.Fatalf("upload avatar: status %d\n%s", r.Status, r.Body)
	}
	etag := getAvatar(t, u.UserID, "").Header.Get("ETag")

	if r := u.del("/users/me/avatar"); r.Status != http.StatusNoContent {
		t.Fatalf("delete avatar: status %d\n%s", r.Status, r.Body)
	}
	if r := getAvatar(t, u.UserID, etag); r.Status != http.StatusNotFound {
		t.Fatalf("deleted avatar: status %d, want 404", r.Status)
	}
}
