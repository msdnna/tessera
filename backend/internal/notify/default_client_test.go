package notify

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/containrrr/shoutrrr"

	"tessera/internal/netguard"
)

// swapDefaultClient installs the hardened client for one test and restores the
// stock one afterwards, so the rest of the package keeps its httptest freedom.
func swapDefaultClient(t *testing.T) {
	t.Helper()
	prev := http.DefaultClient
	t.Cleanup(func() { http.DefaultClient = prev })
	HardenDefaultClient()
}

// The stock http.DefaultClient will happily connect to loopback; after
// hardening the dial is refused by the guard rather than by the peer.
func TestHardenedDefaultClientBlocksPrivateTarget(t *testing.T) {
	t.Setenv("NOTIFY_ALLOW_PRIVATE_URLS", "false")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	swapDefaultClient(t)

	resp, err := http.Get(srv.URL) // #nosec G107 -- httptest URL, loopback by construction
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("http.Get to a loopback server succeeded, want it blocked")
	}
	if !errors.Is(err, netguard.ErrBlocked) {
		t.Fatalf("error = %v, want netguard.ErrBlocked", err)
	}
}

// The operator opt-in still reaches a private target — the guard must not turn
// a self-hosted ntfy on the same host into a permanent failure.
func TestHardenedDefaultClientHonoursPrivateOptIn(t *testing.T) {
	t.Setenv("NOTIFY_ALLOW_PRIVATE_URLS", "true")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	swapDefaultClient(t)

	resp, err := http.Get(srv.URL) // #nosec G107 -- httptest URL, loopback by construction
	if err != nil {
		t.Fatalf("http.Get with the opt-in on: %v", err)
	}
	_ = resp.Body.Close()
}

// The point of the fix: shoutrrr follows redirects through http.DefaultClient,
// so a first hop that passes guardShoutrrrTarget can still hand the server an
// internal destination. The policy is read per dial, so the test lets the first
// hop through and flips the policy shut from inside the redirecting handler —
// standing in for a public host that answers 302 → 169.254.169.254. Before the
// hardening the second hop was dialled with no check at all and the delivery
// reached `secret`.
func TestShoutrrrRedirectToPrivateIsBlocked(t *testing.T) {
	t.Setenv("NOTIFY_ALLOW_PRIVATE_URLS", "true")

	var reached bool
	secret := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	defer secret.Close()

	hop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// From here on the destination counts as internal. Plain os.Setenv
		// because this runs on the server's goroutine; the t.Setenv above
		// already registered the restore of the original value.
		_ = os.Setenv("NOTIFY_ALLOW_PRIVATE_URLS", "false")
		http.Redirect(w, r, secret.URL, http.StatusFound)
	}))
	defer hop.Close()

	swapDefaultClient(t)

	err := shoutrrr.Send("generic+"+hop.URL, "hello")
	if reached {
		t.Fatal("the redirect target was reached — the hop after the first is unguarded")
	}
	if err == nil {
		t.Fatal("shoutrrr.Send succeeded, want the redirect dial blocked")
	}
	if !strings.Contains(err.Error(), netguard.ErrBlocked.Error()) {
		t.Fatalf("error = %v, want it to carry netguard.ErrBlocked", err)
	}
}
