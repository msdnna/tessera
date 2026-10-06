package handlers

import (
	"strings"
	"testing"
)

// The nonce is what separates a handoff the app asked for from one a web page fired at
// the tessera:// scheme (#2932). Anything that could forge the state layout — a dot, URL
// syntax, an over-long value — has to be dropped at the door rather than carried into the
// redirect the app is about to trust.
func TestOAuthAppState(t *testing.T) {
	cases := []struct {
		title string
		raw   string
		want  string
	}{
		{"hex nonce", "9f86d081884c7d659f86d081884c7d65", "9f86d081884c7d659f86d081884c7d65"},
		{"alphanumeric is fine", "Abc123", "Abc123"},
		{"empty", "", ""},
		// A dot would split the state into a different nonce/random pair on the way back.
		{"dot is rejected", "ab.cd", ""},
		{"url syntax is rejected", "a&access_token=evil", ""},
		{"fragment syntax is rejected", "a#b", ""},
		{"path syntax is rejected", "a/../b", ""},
		{"too long", strings.Repeat("a", oauthAppStateMax+1), ""},
		{"exactly at the limit", strings.Repeat("a", oauthAppStateMax), strings.Repeat("a", oauthAppStateMax)},
	}
	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			if got := oauthAppState(c.raw); got != c.want {
				t.Errorf("oauthAppState(%q) = %q, want %q", c.raw, got, c.want)
			}
		})
	}
}

// The callback has to recover the nonce from the state GitLab echoed — and must report
// "no nonce" for the shapes that predate #2932, or Android and older desktop builds would
// start receiving a handoff field that means nothing to them.
func TestOAuthStateNonce(t *testing.T) {
	const random = "9f86d081884c7d65"
	cases := []struct {
		title  string
		prefix string
		state  string
		want   string
	}{
		{"desktop with nonce", oauthDesktopState, oauthDesktopState + "abc123." + random, "abc123"},
		{"mobile with nonce", oauthMobileState, oauthMobileState + "abc123." + random, "abc123"},
		{"desktop without nonce", oauthDesktopState, oauthDesktopState + random, ""},
		{"web state", "", random, ""},
		{"empty state", "", "", ""},
		// The separator with nothing in front of it is not a nonce.
		{"empty nonce field", oauthDesktopState, oauthDesktopState + "." + random, ""},
		// Whatever survives the split still goes through the charset check.
		{"garbage nonce is dropped", oauthDesktopState, oauthDesktopState + "a b." + random, ""},
	}
	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			if got := oauthStateNonce(c.prefix, c.state); got != c.want {
				t.Errorf("oauthStateNonce(%q, %q) = %q, want %q", c.prefix, c.state, got, c.want)
			}
		})
	}
}

// Round-trip through the exact pair the handlers use: what authorize builds must come
// back out of the callback's parser as the same nonce, with the native marker still
// readable. If the two halves drift apart the app rejects every real login instead of
// every forged one — a failure that only shows up against a live GitLab.
func TestOAuthStateRoundTrip(t *testing.T) {
	const random = "9f86d081884c7d65"
	cases := []struct {
		title  string
		prefix string
		nonce  string
	}{
		{"desktop with nonce", oauthDesktopState, "0123456789abcdef0123456789abcdef"},
		{"android without nonce", oauthMobileState, ""},
		{"web", "", ""},
	}
	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			state := oauthState(c.prefix, c.nonce, random)
			native, prefix := oauthHandoff("", state)
			if native != (c.prefix != "") || prefix != c.prefix {
				t.Fatalf("oauthHandoff(%q) = (%v, %q), want prefix %q", state, native, prefix, c.prefix)
			}
			if got := oauthStateNonce(prefix, state); got != c.nonce {
				t.Errorf("oauthStateNonce(%q) = %q, want %q", state, got, c.nonce)
			}
			if !strings.HasSuffix(state, random) {
				t.Errorf("state %q dropped the random half", state)
			}
		})
	}
}
