package observability

import (
	"testing"

	"github.com/getsentry/sentry-go"
)

// The whole safety story of this branch is that an install without a DSN — which
// is every install until the operator deploys Sentry — never initialises the SDK
// at all. Assert it rather than trust it: a client bound here would make the gin
// middleware start buffering response bodies on every request.
func TestInitSentry_BlankDSNBindsNoClient(t *testing.T) {
	t.Cleanup(func() { sentry.CurrentHub().BindClient(nil) })

	flush := InitSentry("", "development", "tessera-backend@test", 1.0)
	if flush == nil {
		t.Fatal("flush func is nil — main() defers it unconditionally")
	}
	if c := sentry.CurrentHub().Client(); c != nil {
		t.Errorf("blank DSN still bound a client: %v", c)
	}
	flush() // must not panic
}

// A DSN the SDK rejects must not take the API down with it: telemetry is the
// least important thing the process does.
func TestInitSentry_BadDSNIsNotFatal(t *testing.T) {
	t.Cleanup(func() { sentry.CurrentHub().BindClient(nil) })

	flush := InitSentry("nonsense://not-a-dsn", "development", "tessera-backend@test", 1.0)
	if flush == nil {
		t.Fatal("flush func is nil after a bad DSN")
	}
	flush()
}

func TestInitSentry_ValidDSNEnablesClient(t *testing.T) {
	t.Cleanup(func() { sentry.CurrentHub().BindClient(nil) })

	flush := InitSentry("http://key@localhost:9100/1", "staging", "tessera-backend@1.2.3", 0.25)
	defer flush()

	client := sentry.CurrentHub().Client()
	if client == nil {
		t.Fatal("valid DSN did not bind a client")
	}
	opts := client.Options()
	if opts.Environment != "staging" {
		t.Errorf("environment = %q, want staging", opts.Environment)
	}
	if opts.Release != "tessera-backend@1.2.3" {
		t.Errorf("release = %q", opts.Release)
	}
	if opts.TracesSampleRate != 0.25 {
		t.Errorf("traces rate = %v, want 0.25", opts.TracesSampleRate)
	}
}

// Rate 0 means "no tracing"; leaving EnableTracing on with a zero rate is the
// kind of contradiction the SDK resolves silently.
func TestInitSentry_ZeroRateDisablesTracing(t *testing.T) {
	t.Cleanup(func() { sentry.CurrentHub().BindClient(nil) })

	flush := InitSentry("http://key@localhost:9100/1", "development", "tessera-backend@test", 0)
	defer flush()

	client := sentry.CurrentHub().Client()
	if client == nil {
		t.Fatal("valid DSN did not bind a client")
	}
	if client.Options().EnableTracing {
		t.Error("tracing enabled despite a 0 sample rate")
	}
}
