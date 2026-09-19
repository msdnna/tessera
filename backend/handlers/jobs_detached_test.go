// The jobs panel's "run now" starts a background sweep and answers before it
// finishes, so the work outlives the request. main's shutdown ordering — HTTP,
// then the workers at a tick boundary, then the pool — is written on the
// assumption that nothing is left writing once the workers are drained, and a
// manual run that belongs to no scope breaks it: the pool closes mid-sweep, and
// settleDelivery, which reads ctx.Err() to tell a deploy stopping mid-send from
// a verdict on the delivery, is told the process is fine (#2932).
package handlers

import (
	"context"
	"sync"
	"testing"
	"time"
)

const settle = 50 * time.Millisecond

// TestRunDetachedJoinsTheShutdownScope pins both halves of the contract: the run
// is told when the process is going away, and shutdown waits for it to return.
func TestRunDetachedJoinsTheShutdownScope(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	h := &API{}
	h.WireBackgroundScope(ctx, &wg)

	started := make(chan struct{})
	noticed := make(chan struct{})
	finish := make(chan struct{})
	if !h.runDetached("test", func(runCtx context.Context) {
		close(started)
		<-runCtx.Done()
		close(noticed)
		// Hold on past the cancellation the way a sweep holds on to settle the
		// row it is on — the point of the WaitGroup is that this is waited for.
		<-finish
	}) {
		t.Fatal("runDetached refused to start on a live scope")
	}
	<-started

	drained := make(chan struct{})
	go func() { wg.Wait(); close(drained) }()
	select {
	case <-drained:
		t.Fatal("shutdown drained while the run was still going: it was never registered")
	case <-time.After(settle):
	}

	cancel()
	select {
	case <-noticed:
	case <-time.After(2 * time.Second):
		t.Fatal("the detached run never saw the shutdown: it is running on a context of its own")
	}
	select {
	case <-drained:
		t.Fatal("shutdown drained before the run returned — the pool would close under it")
	case <-time.After(settle):
	}

	close(finish)
	select {
	case <-drained:
	case <-time.After(2 * time.Second):
		t.Fatal("shutdown never drained after the run returned")
	}
}

// TestRunDetachedRefusesOnceShutdownStarted: a run that would be cancelled
// before its first query must not be reported as started, and must not raise the
// counter shutdown is already waiting on.
func TestRunDetachedRefusesOnceShutdownStarted(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var wg sync.WaitGroup
	h := &API{}
	h.WireBackgroundScope(ctx, &wg)

	ran := make(chan struct{})
	if h.runDetached("test", func(context.Context) { close(ran) }) {
		t.Fatal("runDetached reported a start while the process was shutting down")
	}
	select {
	case <-ran:
		t.Fatal("runDetached ran the work it said it had not started")
	case <-time.After(settle):
	}

	drained := make(chan struct{})
	go func() { wg.Wait(); close(drained) }()
	select {
	case <-drained:
	case <-time.After(2 * time.Second):
		t.Fatal("a refused run still raised the shutdown counter")
	}
}

// TestRunDetachedWithoutAScopeStillRuns: the scope is optional wiring, and an API
// built without one (the GitLab and job unit tests) must still do the work rather
// than refuse every request.
func TestRunDetachedWithoutAScopeStillRuns(t *testing.T) {
	h := &API{}
	live := make(chan struct{})
	if !h.runDetached("test", func(runCtx context.Context) {
		if runCtx.Err() == nil {
			close(live)
		}
	}) {
		t.Fatal("runDetached refused without a scope wired")
	}
	select {
	case <-live:
	case <-time.After(2 * time.Second):
		t.Fatal("the run never happened, or got a context that was already done")
	}
}
