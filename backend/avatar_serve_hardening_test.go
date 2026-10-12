// The avatar route is the fifth byte-serving route, and the last one without the
// inline-safety headers the other four share (#2932).
//
// /api/users/<id>/avatar is unauthenticated, same-origin, and hands back bytes a
// user uploaded — the same three properties that made /uploads worth hardening in
// #2621. What it was missing is the pair that route settled on: `nosniff` to pin
// the declared type, and the sandbox CSP to neuter anything that still ends up
// rendered. ServeUpload, DocumentAsset, streamProxied (both GitLab proxies) and
// the conference-attachment download all set them; this one set neither.
//
// This is the second line, not the first: UploadMyAvatar gates on
// http.DetectContentType and stores the sniffed verdict, accepting only
// png/jpeg/gif/webp, and it has done so since the feature landed (de753f2b) — so
// there are no rows holding a client-declared type, and no payload is known to
// get through today. The headers are here because the stored type is trusted on
// the way out by a public route, and that trust should not be the only thing
// standing between an upload gate and the app's own origin.
package main

import (
	"net/http"
	"strings"
	"testing"
)

// A real avatar still renders inline — the headers must not turn pictures into
// downloads — but with sniffing and active content closed off.
func TestAvatarServedWithInlineSafetyHeaders(t *testing.T) {
	t.Parallel()
	u := signup(t)
	if r := putAvatar(t, u, gifBytes('a')); r.Status != http.StatusOK {
		t.Fatalf("upload avatar: status %d\n%s", r.Status, r.Body)
	}

	r := getAvatar(t, u.UserID, "")
	if r.Status != http.StatusOK {
		t.Fatalf("get avatar: status %d\n%s", r.Status, r.Body)
	}
	if got := r.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options: %q, want nosniff", got)
	}
	if got := r.Header.Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'none'") ||
		!strings.Contains(got, "sandbox") {
		t.Fatalf("Content-Security-Policy: %q, want the sandbox policy the other media routes serve", got)
	}
	// The point of the route is that <img> renders it. A Content-Disposition or a
	// rewritten type on a legitimate image would be a regression, not hardening.
	if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "image/gif") {
		t.Fatalf("Content-Type: %q, want image/gif", got)
	}
	if got := r.Header.Get("Content-Disposition"); got != "" {
		t.Fatalf("Content-Disposition: %q, want avatars to stay inline", got)
	}
}

// The upload gate is the first line and stays the load-bearing one: nothing the
// browser would treat as active content may reach the blob in the first place,
// whatever the filename or the declared type says.
func TestAvatarUploadRejectsScriptableContent(t *testing.T) {
	t.Parallel()
	u := signup(t)

	for _, tc := range []struct{ name, body string }{
		{"svg", svgBody},
		{"html", htmlBody},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if r := putAvatar(t, u, []byte(tc.body)); r.Status != http.StatusUnsupportedMediaType {
				t.Fatalf("upload %s avatar: status %d, want 415\n%s", tc.name, r.Status, r.Body)
			}
		})
	}
}
