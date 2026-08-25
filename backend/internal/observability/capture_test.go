package observability

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

// recordTransport captures events in memory instead of shipping them, so a test
// can assert what CapturePanic/CaptureError/Recover hand to Sentry.
type recordTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (t *recordTransport) Configure(sentry.ClientOptions) {}
func (t *recordTransport) SendEvent(e *sentry.Event) {
	t.mu.Lock()
	t.events = append(t.events, e)
	t.mu.Unlock()
}
func (t *recordTransport) Flush(time.Duration) bool              { return true }
func (t *recordTransport) FlushWithContext(context.Context) bool { return true }
func (t *recordTransport) Close()                                {}

func (t *recordTransport) captured() []*sentry.Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]*sentry.Event(nil), t.events...)
}

// bindRecorder installs a client whose transport records events and returns it,
// restoring no-client state on cleanup so tests don't leak a hub into each other.
func bindRecorder(t *testing.T) *recordTransport {
	t.Helper()
	rt := &recordTransport{}
	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:       "http://key@localhost:9100/1",
		Transport: rt,
	})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	sentry.CurrentHub().BindClient(client)
	t.Cleanup(func() { sentry.CurrentHub().BindClient(nil) })
	return rt
}

// The whole point of these helpers is to be safe on every install, including the
// overwhelming majority that never configure a DSN. None may panic or send when
// no client is bound.
func TestCaptureHelpers_NoClientNoOp(_ *testing.T) {
	sentry.CurrentHub().BindClient(nil)
	CaptureError("worker:test", errors.New("boom")) // must not panic
	CapturePanic("worker:test", "kaboom")           // must not panic
	func() {
		defer Recover("worker:test")
		panic("recovered without a client")
	}()
}

func TestCaptureError_SendsTaggedException(t *testing.T) {
	rt := bindRecorder(t)

	CaptureError("gitlab-sync", nil) // nil is a no-op
	if got := len(rt.captured()); got != 0 {
		t.Fatalf("nil error sent %d events, want 0", got)
	}

	CaptureError("gitlab-sync", errors.New("sync failed"))
	evs := rt.captured()
	if len(evs) != 1 {
		t.Fatalf("captured %d events, want 1", len(evs))
	}
	e := evs[0]
	if e.Level != sentry.LevelError {
		t.Errorf("level = %q, want error", e.Level)
	}
	if e.Tags["component"] != "gitlab-sync" || e.Tags["origin"] != "background" {
		t.Errorf("tags = %v, want component=gitlab-sync origin=background", e.Tags)
	}
	if len(e.Exception) == 0 {
		t.Error("no exception attached to the event")
	}
}

func TestCapturePanic_SendsFatalEvent(t *testing.T) {
	rt := bindRecorder(t)

	CapturePanic("worker:notify_delivery", "nil map write")
	evs := rt.captured()
	if len(evs) != 1 {
		t.Fatalf("captured %d events, want 1", len(evs))
	}
	if evs[0].Level != sentry.LevelFatal {
		t.Errorf("level = %q, want fatal", evs[0].Level)
	}
	if evs[0].Tags["component"] != "worker:notify_delivery" {
		t.Errorf("component tag = %q", evs[0].Tags["component"])
	}
}

// Recover both stops the panic (the goroutine returns normally) and reports it.
func TestRecover_CatchesAndReports(t *testing.T) {
	rt := bindRecorder(t)

	returned := false
	func() {
		defer func() { returned = true }()
		defer Recover("doc-ws-read")
		panic("frame decode blew up")
	}()

	if !returned {
		t.Fatal("Recover did not stop the panic — the goroutine would have crashed")
	}
	if got := len(rt.captured()); got != 1 {
		t.Fatalf("captured %d events, want 1", got)
	}
}
