package observability

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getsentry/sentry-go"
)

func TestRedactQuery(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"code=abc123&state=xyz", "code=***&state=***"},
		{"token=secret", "token=***"},
		{"sig=deadbeef&p=cGF0aA", "sig=***&p=cGF0aA"},
		{"page=2&limit=50", "page=2&limit=50"},
		{"code", "code"}, // value-less key keeps its form
		{"q=code&code=real", "q=code&code=***"},
		// The invitation token. It is mailed as /invite?token=… and the web app
		// then hands the invitee to /register?invite=<the same token>, so the one
		// credential travels under two names.
		{"invite=inv-abc", "invite=***"},
	}
	for _, c := range cases {
		if got := RedactQuery(c.in); got != c.want {
			t.Errorf("RedactQuery(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRedactURLs(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"empty", "", ""},
		{
			// The shape that started this: a *url.Error from the avatar proxy.
			"url error from an http client",
			`Get "http://gl.example/uploads/-/system/user/avatar/7/a.png?private_token=glpat-live": dial tcp: i/o timeout`,
			`Get "http://gl.example/uploads/-/system/user/avatar/7/a.png?private_token=***": dial tcp: i/o timeout`,
		},
		{
			"every parameter on one url, unrelated ones kept",
			"https://h/x?page=2&token=t&code=c&sig=s&access_token=a&refresh_token=r&secret=e&state=n&limit=50",
			"https://h/x?page=2&token=***&code=***&sig=***&access_token=***&refresh_token=***&secret=***&state=***&limit=50",
		},
		{
			// A short name must not eat a longer one that ends with it: the
			// value has to disappear, not the "access_" prefix of the key.
			"longer parameter name wins",
			"https://h/x?access_token=live",
			"https://h/x?access_token=***",
		},
		{
			// Only a real parameter, not the word appearing in prose or path.
			"no false positives",
			"fetching /api/token for user code=? nothing to redact",
			"fetching /api/token for user code=? nothing to redact",
		},
	}
	for _, c := range cases {
		if got := RedactURLs(c.in); got != c.want {
			t.Errorf("%s: RedactURLs(%q) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// The regression this file exists for: sentry-go copies r.URL.RawQuery into the
// event verbatim (interfaces.go newRequest), so an OAuth callback that 500s —
// or, with tracing on, any sampled request at all — used to hand the live
// authorization code to the Sentry project.
func TestScrubEvent_RedactsRequestQueryString(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/auth/oauth/gitlab/callback?code=live-auth-code&state=nonce&keep=1", nil)
	event := sentry.NewEvent()
	event.Request = sentry.NewRequest(r)

	got := scrubEvent(event).Request.QueryString
	if strings.Contains(got, "live-auth-code") || strings.Contains(got, "nonce") {
		t.Fatalf("secrets survived scrubbing: %q", got)
	}
	if want := "code=***&state=***&keep=1"; got != want {
		t.Errorf("QueryString = %q, want %q", got, want)
	}
}

// The unit test above proves the function; this one proves it is actually wired
// into the options InitSentry ships — including the transaction path, which is
// the one that fires on *every* request at the default sample rate of 1.0.
func TestClientOptions_ScrubsOnTheWire(t *testing.T) {
	rt := &recordTransport{}
	opts := clientOptions("http://key@localhost:9100/1", "test", "tessera-backend@test", 1.0)
	opts.Transport = rt
	client, err := sentry.NewClient(opts)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	sentry.CurrentHub().BindClient(client)
	t.Cleanup(func() { sentry.CurrentHub().BindClient(nil) })

	req := httptest.NewRequest("GET", "/api/auth/oauth/gitlab/callback?code=live-auth-code&state=nonce", nil)
	for _, ev := range []*sentry.Event{sentry.NewEvent(), sentry.NewEvent()} {
		ev.Request = sentry.NewRequest(req)
		ev.Message = "boom"
		ev.Type = "" // second one is flipped to a transaction below
		sentry.CurrentHub().CaptureEvent(ev)
	}
	// A transaction takes the other hook; captured the same way, typed differently.
	tx := sentry.NewEvent()
	tx.Type = "transaction"
	tx.Transaction = "GET /api/auth/oauth/:provider/callback"
	tx.Request = sentry.NewRequest(req)
	sentry.CurrentHub().CaptureEvent(tx)

	sent := rt.captured()
	if len(sent) == 0 {
		t.Fatal("nothing reached the transport")
	}
	for _, e := range sent {
		if e.Request == nil {
			continue
		}
		if strings.Contains(e.Request.QueryString, "live-auth-code") {
			t.Errorf("type=%q shipped the authorization code: %q", e.Type, e.Request.QueryString)
		}
	}
}

func TestScrubEvent_NilSafe(t *testing.T) {
	if scrubEvent(nil) != nil {
		t.Error("scrubEvent(nil) must stay nil — the SDK reads it back as 'drop the event'")
	}
	event := sentry.NewEvent() // no Request (background capture)
	if scrubEvent(event) != event {
		t.Error("scrubEvent dropped an event with no Request")
	}
}
