// Package observability wires optional runtime telemetry (Sentry) into the
// backend. Everything here is a no-op when SENTRY_DSN is unset, so the binary
// behaves identically in environments where Sentry isn't configured — which is
// every install until the operator deploys one.
package observability

import (
	"fmt"
	"log"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/getsentry/sentry-go"
)

// InitSentry configures the global Sentry hub from the supplied options and
// returns a flush function that the caller MUST defer — it drains buffered
// events on shutdown so the last error/transaction isn't lost.
//
// A blank dsn disables Sentry entirely: the SDK is never initialised, the
// returned closure is a cheap no-op, and the rest of the app (sentrygin
// middleware, sentry.CaptureException, …) degrades to harmless no-ops.
func InitSentry(dsn, environment, release string, tracesSampleRate float64) func() {
	if dsn == "" {
		log.Println("Sentry disabled (SENTRY_DSN not set)")
		return func() {}
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:         dsn,
		Environment: environment,
		Release:     release,
		// Performance / distributed tracing. sentrygin starts one transaction
		// per HTTP request, sampled at this rate; 0 disables tracing entirely.
		EnableTracing:    tracesSampleRate > 0,
		TracesSampleRate: tracesSampleRate,
		// Attach a stack trace to messages captured without an error value,
		// so log-style captures are still actionable.
		AttachStacktrace: true,
	})
	if err != nil {
		// Treat a bad DSN as non-fatal — telemetry must never take the API down.
		log.Printf("Sentry init failed, continuing without telemetry: %v", err)
		return func() {}
	}

	log.Printf("Sentry enabled (env=%s, release=%s, traces=%.2f)", environment, release, tracesSampleRate)
	return func() { sentry.Flush(2 * time.Second) }
}

// CapturePanic reports a recovered panic to Sentry and logs it with its stack.
// It fills the gap the HTTP middleware can't reach: a panic in a background
// goroutine is invisible to gin's recovery and to middleware.SentryReport, and
// left alone it takes the whole process down. component tags the event
// ("worker:notify_delivery", "gitlab-sync", …) so background panics group per
// origin instead of collapsing into one issue.
//
// Safe when Sentry is disabled (no client → the report is skipped, the slog line
// still prints). It flushes briefly because a panic often precedes process death
// and the event must not be lost in the async transport's buffer.
func CapturePanic(component string, recovered any) {
	slog.Error("recovered panic in background goroutine",
		"component", component, "panic", fmt.Sprint(recovered), "stack", string(debug.Stack()))
	hub := sentry.CurrentHub()
	if hub.Client() == nil {
		return
	}
	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(sentry.LevelFatal)
		scope.SetTag("component", component)
		scope.SetTag("origin", "background")
		hub.Recover(recovered)
	})
	sentry.Flush(2 * time.Second)
}

// CaptureError reports a handled background error to Sentry, tagged with the
// component so it groups per origin. Unlike CapturePanic it does not flush — the
// async transport drains it — because the worker keeps running after a handled
// error. No-op when err is nil or Sentry is disabled. Use it for a discrete
// background job that failed terminally (a sync run, a delivery batch), not for
// per-tick best-effort retries, which would be noise.
func CaptureError(component string, err error) {
	if err == nil {
		return
	}
	hub := sentry.CurrentHub()
	if hub.Client() == nil {
		return
	}
	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetLevel(sentry.LevelError)
		scope.SetTag("component", component)
		scope.SetTag("origin", "background")
		hub.CaptureException(err)
	})
}

// Recover is a deferred panic guard for a fire-and-forget goroutine that has no
// supervisor loop: defer it as the goroutine's first statement and a panic is
// reported through CapturePanic instead of crashing the process.
//
//	go func() {
//	    defer observability.Recover("gitlab-sync")
//	    …
//	}()
func Recover(component string) {
	if r := recover(); r != nil {
		CapturePanic(component, r)
	}
}
