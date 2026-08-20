// Package observability wires optional runtime telemetry (Sentry) into the
// backend. Everything here is a no-op when SENTRY_DSN is unset, so the binary
// behaves identically in environments where Sentry isn't configured — which is
// every install until the operator deploys one.
package observability

import (
	"log"
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
