package notify

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// webhookChannel builds a webhook channel config around a target URL + auth.
func webhookChannel(target, authHeader string) Channel {
	return Channel{
		Type:   "webhook",
		Config: map[string]any{"url": target},
		Secret: map[string]string{"auth_header": authHeader},
	}
}

// TestWebhookSendDelivers is the happy path: with private targets allowed, the
// envelope is POSTed (with the Authorization header) to the target and a 2xx
// answer resolves nil. The loopback test server is reached via the injected
// client; the per-call ValidateURL is let through by NOTIFY_ALLOW_PRIVATE_URLS.
func TestWebhookSendDelivers(t *testing.T) {
	var hits int32
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("NOTIFY_ALLOW_PRIVATE_URLS", "true") // srv.URL is loopback — let validation through
	s := WebhookSender{Client: srv.Client()}

	err := s.Send(context.Background(),
		webhookChannel(srv.URL, "Bearer sekret"),
		Message{Kind: "assigned", Title: "T", Body: "B", Link: "https://app/x"})

	if err != nil {
		t.Fatalf("Send = %v, want nil", err)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("hits = %d, want 1", hits)
	}
	if gotAuth != "Bearer sekret" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotBody == "" || !contains(gotBody, `"title":"T"`) || !contains(gotBody, `"link":"https://app/x"`) {
		t.Fatalf("payload = %q", gotBody)
	}
}

// TestWebhookSendRejectsPrivateTarget: with the default policy (private
// disallowed), a loopback target is rejected as a permanent error before any
// HTTP call is made — a misconfigured private webhook must not retry forever.
func TestWebhookSendRejectsPrivateTarget(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("NOTIFY_ALLOW_PRIVATE_URLS", "false") // the safe default
	s := WebhookSender{Client: srv.Client()}

	err := s.Send(context.Background(), webhookChannel(srv.URL, ""), Message{Body: "x"})

	if !IsPermanent(err) {
		t.Fatalf("Send = %v, want a permanent error", err)
	}
	if atomic.LoadInt32(&hits) != 0 {
		t.Fatalf("hits = %d, want 0 (no request should have been made)", hits)
	}
}

// TestWebhookSendRejectsBadScheme: a non-http(s) URL is rejected regardless of
// the private-target policy.
func TestWebhookSendRejectsBadScheme(t *testing.T) {
	t.Setenv("NOTIFY_ALLOW_PRIVATE_URLS", "true") // scheme check is independent of this
	s := WebhookSender{Client: &http.Client{}}

	for _, raw := range []string{"file:///etc/passwd", "javascript:alert(1)", "gopher://x/"} {
		if err := s.Send(context.Background(), webhookChannel(raw, ""), Message{Body: "x"}); !IsPermanent(err) {
			t.Fatalf("Send(%q) = %v, want permanent", raw, err)
		}
	}
}

// TestWebhookSendMissingURL keeps the pre-existing contract: an empty URL is a
// permanent error.
func TestWebhookSendMissingURL(t *testing.T) {
	s := WebhookSender{Client: &http.Client{}}
	if err := s.Send(context.Background(), webhookChannel("", ""), Message{Body: "x"}); !IsPermanent(err) {
		t.Fatalf("Send with no url = %v, want permanent", err)
	}
}

// contains is a tiny strings.Contains without importing strings.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}
