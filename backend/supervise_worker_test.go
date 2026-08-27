package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
)

// superviseWorker must turn a panicking tick into a restart, not a process death,
// and must return promptly once ctx is cancelled. Sentry is left unbound so the
// recovered panic is a pure no-op (no 2s flush) and the test stays fast.
func TestSuperviseWorker_RestartsAfterPanicAndStopsOnCtx(t *testing.T) {
	sentry.CurrentHub().BindClient(nil)

	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int64
	done := make(chan struct{})
	go func() {
		superviseWorker(ctx, "test", func(ctx context.Context) {
			switch calls.Add(1) {
			case 1:
				panic("first run blew up") // recovered → restarted after the backoff
			default:
				<-ctx.Done() // second run behaves: block until shutdown, return cleanly
			}
		})
		close(done)
	}()

	// The restart waits one backoff second; give it generous head-room.
	deadline := time.After(5 * time.Second)
	for calls.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("worker did not restart after panic (calls=%d)", calls.Load())
		case <-time.After(20 * time.Millisecond):
		}
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("superviseWorker did not return after ctx cancel")
	}
	if got := calls.Load(); got != 2 {
		t.Errorf("worker ran %d times, want exactly 2 (panic + one blocking restart)", got)
	}
}

// A clean return from fn on a live ctx is an early exit, not shutdown: it must be
// restarted too, so a worker that mistakenly falls out of its loop keeps running.
func TestSuperviseWorker_RestartsOnEarlyReturn(t *testing.T) {
	sentry.CurrentHub().BindClient(nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int64
	done := make(chan struct{})
	go func() {
		superviseWorker(ctx, "test", func(ctx context.Context) {
			if calls.Add(1) == 1 {
				return // early exit on a live ctx → restart after backoff
			}
			<-ctx.Done()
		})
		close(done)
	}()

	deadline := time.After(5 * time.Second)
	for calls.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("worker did not restart after early return (calls=%d)", calls.Load())
		case <-time.After(20 * time.Millisecond):
		}
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("superviseWorker did not return after ctx cancel")
	}
}
