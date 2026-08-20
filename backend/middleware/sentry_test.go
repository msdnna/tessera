package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-gonic/gin"
)

// mockTransport captures events in-memory so we can assert on what SentryReport
// sends without a real Sentry server.
type mockTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (t *mockTransport) Configure(sentry.ClientOptions) {}
func (t *mockTransport) SendEvent(e *sentry.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, e)
}
func (t *mockTransport) Flush(time.Duration) bool              { return true }
func (t *mockTransport) FlushWithContext(context.Context) bool { return true }
func (t *mockTransport) Close()                                {}
func (t *mockTransport) all() []*sentry.Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]*sentry.Event(nil), t.events...)
}

// bindSentry installs a client backed by an in-memory transport and unbinds it
// afterwards — the hub is global, so leaking it would make sibling tests in this
// package start reporting.
func bindSentry(t *testing.T) *mockTransport {
	t.Helper()
	transport := &mockTransport{}
	if err := sentry.Init(sentry.ClientOptions{Dsn: "http://key@localhost/1", Transport: transport}); err != nil {
		t.Fatalf("sentry init: %v", err)
	}
	t.Cleanup(func() { sentry.CurrentHub().BindClient(nil) })
	return transport
}

func newSentryRouter() *gin.Engine {
	r := gin.New()
	r.Use(RequestID())
	r.Use(sentrygin.New(sentrygin.Options{}))
	r.Use(SentryReport())
	r.GET("/ok", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	r.GET("/boom", func(c *gin.Context) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "kaboom"})
	})
	// Mirrors handlers.fail(): a generic 500 body plus the real cause attached
	// for the middleware chain.
	r.GET("/fail/:id", func(c *gin.Context) {
		_ = c.Error(errors.New("pool exhausted"))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
	})
	return r
}

func TestSentryReport_Captures5xx(t *testing.T) {
	transport := bindSentry(t)
	r := newSentryRouter()

	// 2xx must not produce an event.
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/ok", nil))
	if n := len(transport.all()); n != 0 {
		t.Fatalf("200 produced %d events, want 0", n)
	}

	// 5xx must produce exactly one error event with the route context.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/boom", nil))

	events := transport.all()
	if len(events) != 1 {
		t.Fatalf("500 produced %d events, want 1", len(events))
	}
	e := events[0]
	if e.Level != sentry.LevelError {
		t.Errorf("level = %v, want %v", e.Level, sentry.LevelError)
	}
	if e.Tags["http.status_code"] != "500" {
		t.Errorf("http.status_code tag = %q, want 500", e.Tags["http.status_code"])
	}
	if e.Tags["http.route"] != "/boom" {
		t.Errorf("http.route tag = %q, want /boom", e.Tags["http.route"])
	}
	if e.Tags["request_id"] == "" {
		t.Error("request_id tag missing — the event can't be tied back to the access log")
	}
	if ctx, ok := e.Contexts["tessera"]; !ok || ctx["response_body"] == nil {
		t.Errorf("response_body not captured in contexts: %+v", e.Contexts)
	}
}

// handlers.fail() hides the cause from the client, so without c.Error() the
// event would carry nothing but "HTTP 500". Assert the real error survives.
func TestSentryReport_CapturesAttachedError(t *testing.T) {
	transport := bindSentry(t)
	r := newSentryRouter()

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/fail/abc", nil))

	events := transport.all()
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	e := events[0]
	if len(e.Exception) == 0 {
		t.Fatalf("event carries no exception, only: %+v", e.Message)
	}
	if e.Exception[0].Value != "pool exhausted" {
		t.Errorf("exception value = %q, want %q", e.Exception[0].Value, "pool exhausted")
	}
}

// IDs in the path must not fan one broken endpoint out into thousands of
// issues — the fingerprint is built from the route pattern, not the URL.
func TestSentryReport_FingerprintUsesRoutePattern(t *testing.T) {
	transport := bindSentry(t)
	r := newSentryRouter()

	for _, id := range []string{"abc", "def"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/fail/"+id, nil))
	}

	events := transport.all()
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	a, b := events[0].Fingerprint, events[1].Fingerprint
	if len(a) == 0 {
		t.Fatal("no fingerprint set")
	}
	if len(a) != len(b) {
		t.Fatalf("fingerprints differ in shape: %v vs %v", a, b)
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("fingerprint varies with the URL: %v vs %v", a, b)
		}
	}
	if a[len(a)-2] != "/fail/:id" {
		t.Errorf("fingerprint route = %q, want the route pattern", a[len(a)-2])
	}
}

func TestSentryReport_DisabledIsNoop(t *testing.T) {
	// No client bound → middleware must short-circuit and never panic.
	sentry.CurrentHub().BindClient(nil)

	r := newSentryRouter()
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/boom", nil))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (handler must still run)", w.Code)
	}
}
