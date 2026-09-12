package notify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// The "shoutrrr" channel type is the escape hatch: its secret IS a full service
// URL, and shoutrrr's generic service will POST to whatever host that URL names
// — through its own http.DefaultClient, which our guarded dialer never touches.
// So the SSRF guard the webhook channel gets from netguard has to be applied to
// this path explicitly, or a workspace member can aim the server at the internal
// network (a sibling container, the cloud metadata endpoint) by picking a
// channel type instead of a URL scheme.

// shoutrrrChannel builds a generic-service channel pointed at target.
func shoutrrrChannel(target string) Channel {
	return Channel{Type: "shoutrrr", Secret: map[string]string{"url": target}}
}

// genericURL rewrites an http test-server URL into shoutrrr's generic DSL.
func genericURL(srvURL string) string {
	return "generic+" + srvURL
}

// TestShoutrrrSendRejectsPrivateTarget is the finding: under the default policy
// a shoutrrr channel aimed at loopback must not reach it.
func TestShoutrrrSendRejectsPrivateTarget(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("NOTIFY_ALLOW_PRIVATE_URLS", "false") // the safe default

	err := ShoutrrrSender{}.Send(context.Background(), shoutrrrChannel(genericURL(srv.URL)), Message{Body: "x"})

	if !IsPermanent(err) {
		t.Fatalf("Send = %v, want a permanent error", err)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("hits = %d, want 0 — the internal server was reached", hits)
	}
}

// TestShoutrrrSendRejectsPrivateHostname covers the same target named by a
// hostname rather than a literal IP: "localhost" resolves to loopback, which a
// string-only check would wave through.
func TestShoutrrrSendRejectsPrivateHostname(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	_, port, ok := strings.Cut(strings.TrimPrefix(srv.URL, "http://"), ":")
	if !ok {
		t.Fatalf("unexpected test server URL %q", srv.URL)
	}

	t.Setenv("NOTIFY_ALLOW_PRIVATE_URLS", "false")

	err := ShoutrrrSender{}.Send(context.Background(),
		shoutrrrChannel("generic+http://localhost:"+port+"/hook"), Message{Body: "x"})

	if !IsPermanent(err) {
		t.Fatalf("Send = %v, want a permanent error", err)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("hits = %d, want 0 — the internal server was reached via a hostname", hits)
	}
}

// TestShoutrrrSendAllowsPrivateWhenOptedIn is the negative control: an operator
// running ntfy/gotify next to Tessera flips NOTIFY_ALLOW_PRIVATE_URLS and the
// same delivery goes through. Without this the guard could pass by blocking
// everything.
func TestShoutrrrSendAllowsPrivateWhenOptedIn(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("NOTIFY_ALLOW_PRIVATE_URLS", "true")

	err := ShoutrrrSender{}.Send(context.Background(), shoutrrrChannel(genericURL(srv.URL)), Message{Body: "x"})
	if err != nil {
		t.Fatalf("Send = %v, want nil", err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("hits = %d, want 1", hits)
	}
}

// TestShoutrrrGuardKeepsPublicTarget pins that the guard only rejects an
// internal target: a public host passes, and so does a DSL whose host is a
// placeholder the service never dials (telegram's own endpoint is fixed) —
// that arm is netguard.TestValidateHostAllowsUnresolvable, which can shorten
// the resolver timeout; here a cancelled context stands in for it.
func TestShoutrrrGuardKeepsPublicTarget(t *testing.T) {
	t.Setenv("NOTIFY_ALLOW_PRIVATE_URLS", "false")

	if err := guardShoutrrrTarget(context.Background(), "generic+https://hooks.example.com/x"); err != nil {
		t.Fatalf("public target = %v, want nil", err)
	}

	dead, cancel := context.WithCancel(context.Background())
	cancel() // no resolution possible — the placeholder host must still pass
	if err := guardShoutrrrTarget(dead, "telegram://123:abc@telegram/?chats=42"); err != nil {
		t.Fatalf("telegram DSL = %v, want nil", err)
	}
}

// TestShoutrrrGuardBlocksMetadataEndpoint pins the classic SSRF target: the
// cloud metadata address is link-local, so the guard rejects it.
func TestShoutrrrGuardBlocksMetadataEndpoint(t *testing.T) {
	t.Setenv("NOTIFY_ALLOW_PRIVATE_URLS", "false")

	for _, raw := range []string{
		"generic+http://169.254.169.254/latest/meta-data/",
		"generic+http://10.0.0.5:8090/api/admin/users",
		"generic+http://[::1]:9000/x",
		"generic+http://100.64.0.1/x",
	} {
		if err := guardShoutrrrTarget(context.Background(), raw); err == nil {
			t.Fatalf("guardShoutrrrTarget(%q) = nil, want blocked", raw)
		}
	}
}
