package handlers

import (
	"strings"
	"testing"
)

// The handoff target decides where the session (or the error) is delivered: the web SPA
// on the server origin, or the custom-scheme deep link a native client registers. Getting
// this wrong is not a cosmetic bug — the desktop app would be sent to a web page it can't
// receive tokens on, which is exactly the bug #2696 is about.
func TestOAuthHandoffAuthorize(t *testing.T) {
	cases := []struct {
		platform   string
		wantNative bool
		wantPrefix string
	}{
		{"", false, ""},
		{"android", true, oauthMobileState},
		{"desktop", true, oauthDesktopState},
		{"ios", false, ""},
		{"DESKTOP", false, ""}, // exact match only — no case-folding surprises
	}
	for _, c := range cases {
		t.Run("platform="+c.platform, func(t *testing.T) {
			native, prefix := oauthHandoff(c.platform, "")
			if native != c.wantNative || prefix != c.wantPrefix {
				t.Errorf("oauthHandoff(%q, \"\") = (%v, %q), want (%v, %q)",
					c.platform, native, prefix, c.wantNative, c.wantPrefix)
			}
		})
	}
}

func TestOAuthHandoffCallback(t *testing.T) {
	cases := []struct {
		title      string
		state      string
		wantNative bool
		wantPrefix string
	}{
		{"web state", "9f86d081884c7d65", false, ""},
		// The Android marker must keep working after the desktop branch lands: a session
		// that started on the previous binary comes back to the new one and still has to
		// reach the app rather than the web page.
		{"mobile marker", oauthMobileState + "9f86d081884c7d65", true, oauthMobileState},
		{"desktop marker", oauthDesktopState + "9f86d081884c7d65", true, oauthDesktopState},
		{"empty state", "", false, ""},
		// A forged prefix only changes where an *error* is delivered — the CSRF check
		// against the cookie still rejects the state itself.
		{"marker without payload", oauthDesktopState, true, oauthDesktopState},
		{"marker mid-string is not a marker", "9f86d.081", false, ""},
	}
	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			native, prefix := oauthHandoff("", c.state)
			if native != c.wantNative || prefix != c.wantPrefix {
				t.Errorf("oauthHandoff(\"\", %q) = (%v, %q), want (%v, %q)",
					c.state, native, prefix, c.wantNative, c.wantPrefix)
			}
		})
	}
}

// Both markers must stay distinct and non-hex: the web state is hex, so a plain web state
// can never accidentally look like a native one.
func TestOAuthStateMarkersDoNotCollide(t *testing.T) {
	if oauthMobileState == oauthDesktopState {
		t.Fatal("mobile and desktop markers are identical")
	}
	notHex := func(r rune) bool {
		return (r < '0' || r > '9') && (r < 'a' || r > 'f')
	}
	for _, m := range []string{oauthMobileState, oauthDesktopState} {
		if strings.IndexFunc(m, notHex) < 0 {
			t.Errorf("marker %q is all-hex and could collide with a web state", m)
		}
	}
}
