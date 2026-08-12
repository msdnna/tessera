package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recordingCloser stands in for the realtime hub so the ordering can be checked
// without a WebSocket stack.
type recordingCloser struct {
	closed atomic.Bool
	at     atomic.Int64 // sequence number, assigned from a shared counter
	seq    *atomic.Int64
}

func (r *recordingCloser) Close() {
	r.closed.Store(true)
	r.at.Store(r.seq.Add(1))
}

// serve starts srv on an ephemeral port and returns its base URL.
func serve(t *testing.T, srv *http.Server) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("serve: %v", err)
		}
	}()
	return "http://" + ln.Addr().String()
}

// The point of graceful shutdown: a request already being served gets its
// answer, and the port stops accepting new work. Without it every deploy cuts
// whatever was mid-flight.
func TestDrainFinishesInFlightRequest(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("done"))
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	base := serve(t, srv)

	type result struct {
		code int
		err  error
	}
	got := make(chan result, 1)
	go func() {
		resp, err := http.Get(base + "/slow") //nolint:noctx // deliberate: no client-side deadline
		if err != nil {
			got <- result{err: err}
			return
		}
		defer resp.Body.Close()
		got <- result{code: resp.StatusCode}
	}()
	<-started // the request is inside the handler when shutdown begins

	var workers, hubRun sync.WaitGroup
	seq := &atomic.Int64{}
	hub := &recordingCloser{seq: seq}

	drained := make(chan struct{})
	go func() {
		drain(srv, &workers, hub, &hubRun, 5*time.Second)
		close(drained)
	}()

	// Shutdown must not return while the handler is still running.
	select {
	case <-drained:
		t.Fatal("drain returned before the in-flight request finished")
	case <-time.After(100 * time.Millisecond):
	}

	close(release)
	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("drain did not finish after the request completed")
	}

	r := <-got
	if r.err != nil {
		t.Fatalf("in-flight request failed instead of being served: %v", r.err)
	}
	if r.code != http.StatusOK {
		t.Fatalf("in-flight request status = %d, want 200", r.code)
	}
	if !hub.closed.Load() {
		t.Fatal("hub was not closed")
	}

	// The listener is gone, so a new request is refused.
	if _, err := http.Get(base + "/slow"); err == nil { //nolint:noctx // expect a connection error
		t.Fatal("server still accepted a request after drain")
	}
}

// Workers must be waited on before the hub closes and before main's deferred
// pool.Close runs — a worker mid-sync on a closed pool is exactly the corruption
// this ordering exists to prevent.
func TestDrainWaitsForWorkersBeforeClosingHub(t *testing.T) {
	srv := &http.Server{Handler: http.NewServeMux(), ReadHeaderTimeout: 5 * time.Second}
	serve(t, srv)

	seq := &atomic.Int64{}
	hub := &recordingCloser{seq: seq}
	workerFinishedAt := &atomic.Int64{}

	var workers, hubRun sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		time.Sleep(150 * time.Millisecond) // a tick still in progress
		workerFinishedAt.Store(seq.Add(1))
	}()

	drain(srv, &workers, hub, &hubRun, 5*time.Second)

	if workerFinishedAt.Load() == 0 {
		t.Fatal("drain returned before the worker finished")
	}
	if workerFinishedAt.Load() > hub.at.Load() {
		t.Fatalf("hub closed before workers drained (worker=%d hub=%d)",
			workerFinishedAt.Load(), hub.at.Load())
	}
}

// A stuck worker must not hold the process forever: past the budget, shutdown
// proceeds anyway. An orchestrator would SIGKILL us at that point regardless.
func TestDrainGivesUpOnStuckWorker(t *testing.T) {
	srv := &http.Server{Handler: http.NewServeMux(), ReadHeaderTimeout: 5 * time.Second}
	serve(t, srv)

	stuck := make(chan struct{})
	defer close(stuck)

	var workers, hubRun sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		<-stuck
	}()

	seq := &atomic.Int64{}
	hub := &recordingCloser{seq: seq}

	start := time.Now()
	done := make(chan struct{})
	go func() {
		drain(srv, &workers, hub, &hubRun, 200*time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("drain hung on a stuck worker instead of timing out")
	}
	if elapsed := time.Since(start); elapsed < 200*time.Millisecond {
		t.Fatalf("drain gave up after %s, before the budget elapsed", elapsed)
	}
	if !hub.closed.Load() {
		t.Fatal("hub was not closed after the worker timeout")
	}
}

// The workers all hang off the signal context, so cancelling it is what stops
// them; this pins that a ctx-aware tick loop actually returns.
func TestWorkersStopOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	cancel()
	done := make(chan struct{})
	go func() {
		workers.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not return on context cancellation")
	}
}
